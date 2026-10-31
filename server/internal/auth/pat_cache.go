package auth

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// AuthCacheTTL bounds how long a token-hash lookup stays cached before
// the auth middleware goes back to Postgres. Shared by PATCache and
// DaemonTokenCache so both kinds of token follow the same revocation
// latency contract. Short enough that revocation lag from a missed
// invalidation is bounded; long enough that a high-frequency client
// (CLI, daemon) collapses from one DB round-trip per request to one
// per TTL window.
const AuthCacheTTL = 10 * time.Minute

// patCachePrefix namespaces auth-cache keys away from the realtime relay
// (ws:*) and local-skill (mul:local_skill:*) keys.
const patCachePrefix = "mul:auth:pat:"

// patDeletedPrefix marks an account as deleted so a PAT entry that resolved to
// it before the delete cannot keep authenticating afterwards. The marker
// outlives every possible PAT entry: entries live at most AuthCacheTTL, and the
// marker is written for twice that, so no entry can survive the marker.
const patDeletedPrefix = "mul:auth:deleted:"

const patDeletedMarkerTTL = 2 * AuthCacheTTL

func patDeletedKey(userID string) string { return patDeletedPrefix + userID }

// PATCache caches resolved PAT lookups in Redis. A nil *PATCache is safe
// to use — every method becomes a no-op or reports a cache miss, and the
// auth middleware degrades to direct DB lookups.
type PATCache struct {
	rdb redis.UniversalClient
}

// NewPATCache returns a cache backed by rdb. Pass nil to disable caching;
// the returned *PATCache is safe to call but never hits Redis.
func NewPATCache(rdb redis.UniversalClient) *PATCache {
	if rdb == nil {
		return nil
	}
	return &PATCache{rdb: rdb}
}

func patCacheKey(hash string) string { return patCachePrefix + hash }

// Get returns the cached user_id for a token hash. ok=false on cache miss
// or any Redis error — a dead Redis must not take down auth.
func (c *PATCache) Get(ctx context.Context, hash string) (userID string, ok bool) {
	if c == nil {
		return "", false
	}
	v, err := c.rdb.Get(ctx, patCacheKey(hash)).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			slog.Warn("pat_cache: get failed; falling back to DB", "error", err)
		}
		return "", false
	}
	// A cached token is only as good as the account it resolved to. Account
	// deletion marks the user id before it commits; a hit for a marked id is a
	// stale entry that must never authenticate. The failed marker read fails
	// closed (miss) rather than trusting the entry: the DB lookup that follows
	// sees the deleted token row and rejects the request.
	deleted, err := c.rdb.Exists(ctx, patDeletedKey(v)).Result()
	if err != nil {
		slog.Warn("pat_cache: deleted-marker check failed; treating entry as revoked", "error", err)
		return "", false
	}
	if deleted > 0 {
		if err := c.rdb.Del(ctx, patCacheKey(hash)).Err(); err != nil {
			slog.Warn("pat_cache: drop stale entry after account deletion failed", "error", err)
		}
		return "", false
	}
	return v, true
}

// MarkUserDeleted records that the account no longer exists. Account deletion
// calls this BEFORE it commits: from then on every cache hit that resolves to
// the account is rejected, even if the best-effort key sweep below cannot run.
// An error means the marker could not be written, which must abort the delete
// rather than leave a path that keeps honouring a deleted user's tokens.
func (c *PATCache) MarkUserDeleted(ctx context.Context, userID string) error {
	if c == nil || userID == "" {
		return nil
	}
	if err := c.rdb.Set(ctx, patDeletedKey(userID), "1", patDeletedMarkerTTL).Err(); err != nil {
		slog.Warn("pat_cache: account deletion marker write failed", "error", err)
		return err
	}
	return nil
}

// ClearUserDeleted removes the marker after a failed delete rolled back, so a
// transient database error does not lock a live account out of its tokens.
func (c *PATCache) ClearUserDeleted(ctx context.Context, userID string) {
	if c == nil || userID == "" {
		return
	}
	if err := c.rdb.Del(ctx, patDeletedKey(userID)).Err(); err != nil {
		slog.Warn("pat_cache: account deletion marker cleanup failed; tokens stay revoked until it expires", "error", err)
	}
}

// Set populates the cache with the given TTL. Callers MUST pass a TTL no
// longer than the token's remaining lifetime — otherwise an entry could
// outlive the PAT's expires_at and let an expired token pass auth on
// cache hit. Use TTLForExpiry to compute it from a token's expires_at.
//
// Errors are logged and swallowed — a cache write failure is not a
// request failure.
func (c *PATCache) Set(ctx context.Context, hash, userID string, ttl time.Duration) {
	if c == nil || ttl <= 0 {
		return
	}
	if err := c.rdb.Set(ctx, patCacheKey(hash), userID, ttl).Err(); err != nil {
		slog.Warn("pat_cache: set failed", "error", err)
	}
}

// TTLForExpiry returns the cache TTL for a token given its expires_at.
//   - Zero expiresAt (token never expires) → full AuthCacheTTL.
//   - expiresAt in the future → min(AuthCacheTTL, time until expiry).
//   - expiresAt at or before now → 0 (caller should skip caching; the
//     middleware shouldn't reach here because the SELECT already
//     filters expired tokens, but a TOCTOU between SELECT and Set is
//     possible).
//
// Pass time.Time{} when the token has no expiry (pgtype.Timestamptz with
// Valid=false maps to a zero Time).
func TTLForExpiry(now, expiresAt time.Time) time.Duration {
	if expiresAt.IsZero() {
		return AuthCacheTTL
	}
	remaining := expiresAt.Sub(now)
	if remaining <= 0 {
		return 0
	}
	if remaining < AuthCacheTTL {
		return remaining
	}
	return AuthCacheTTL
}

// Invalidate removes the entry for hash. Called on PAT revocation so the
// revoke takes effect immediately rather than waiting for the TTL.
func (c *PATCache) Invalidate(ctx context.Context, hash string) {
	if c == nil {
		return
	}
	if err := c.rdb.Del(ctx, patCacheKey(hash)).Err(); err != nil {
		slog.Warn("pat_cache: invalidate failed; entry will expire on TTL", "error", err)
	}
}

// InvalidateAllForUser drops every cached PAT entry that resolves to userID.
// Account deletion revokes all of that user's tokens in the database; without
// this sweep a Redis-backed deployment could keep honouring a deleted user's
// PAT until the cache TTL expired. The scan runs only on an explicit admin
// action, so its cost is bounded by how often an operator deletes an account.
func (c *PATCache) InvalidateAllForUser(userID string) {
	if c == nil || userID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	iter := c.rdb.Scan(ctx, 0, patCachePrefix+"*", 200).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		value, err := c.rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		if value == userID {
			if err := c.rdb.Del(ctx, key).Err(); err != nil {
				slog.Warn("pat_cache: delete during user sweep failed", "error", err)
				return
			}
		}
	}
	if err := iter.Err(); err != nil {
		slog.Warn("pat_cache: scan during user sweep failed", "error", err)
	}
}
