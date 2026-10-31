package auth

import (
	"context"
	"errors"
	"testing"

	redismock "github.com/go-redis/redismock/v9"
)

// P5-R1: a PAT entry cached before account deletion must not keep
// authenticating afterwards. The deleted-user marker is checked on every cache
// hit, so even a sweep that never runs cannot resurrect the account.

func TestPATCacheDeletedUserMarkerBlocksCacheHits(t *testing.T) {
	ctx := context.Background()
	rdb, mock := redismock.NewClientMock()
	cache := NewPATCache(rdb)
	const hash = "pat-hash"
	const userID = "user-id"

	mock.ExpectGet(patCacheKey(hash)).SetVal(userID)
	mock.ExpectExists(patDeletedKey(userID)).SetVal(0)
	got, ok := cache.Get(ctx, hash)
	if !ok || got != userID {
		t.Fatalf("live account cache hit = (%q, %v), want (%q, true)", got, ok, userID)
	}

	// Account deletion writes the marker before it commits.
	mock.ExpectSet(patDeletedKey(userID), "1", patDeletedMarkerTTL).SetVal("OK")
	if err := cache.MarkUserDeleted(ctx, userID); err != nil {
		t.Fatalf("mark deleted: %v", err)
	}

	// A hit that resolves to the marked account is a miss, and the stale entry
	// is dropped.
	mock.ExpectGet(patCacheKey(hash)).SetVal(userID)
	mock.ExpectExists(patDeletedKey(userID)).SetVal(1)
	mock.ExpectDel(patCacheKey(hash)).SetVal(1)
	got, ok = cache.Get(ctx, hash)
	if ok || got != "" {
		t.Fatalf("deleted account cache hit = (%q, %v), want a miss", got, ok)
	}

	// A rolled-back delete clears the marker so a live account is not locked out.
	mock.ExpectDel(patDeletedKey(userID)).SetVal(1)
	cache.ClearUserDeleted(ctx, userID)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected redis interaction: %v", err)
	}
}

func TestPATCacheFailsClosedWhenTheMarkerCannotBeRead(t *testing.T) {
	ctx := context.Background()
	rdb, mock := redismock.NewClientMock()
	cache := NewPATCache(rdb)
	const hash = "pat-hash"
	const userID = "user-id"

	mock.ExpectGet(patCacheKey(hash)).SetVal(userID)
	mock.ExpectExists(patDeletedKey(userID)).SetErr(errors.New("redis unavailable"))
	if got, ok := cache.Get(ctx, hash); ok || got != "" {
		t.Fatalf("cache hit = (%q, %v) while the revocation marker was unreadable, want a miss", got, ok)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected redis interaction: %v", err)
	}
}

func TestPATCacheMarkUserDeletedPropagatesFailure(t *testing.T) {
	ctx := context.Background()
	rdb, mock := redismock.NewClientMock()
	cache := NewPATCache(rdb)
	mock.ExpectSet(patDeletedKey("user-id"), "1", patDeletedMarkerTTL).SetErr(errors.New("redis unavailable"))
	if err := cache.MarkUserDeleted(ctx, "user-id"); err == nil {
		t.Fatal("MarkUserDeleted reported success while the marker could not be written")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected redis interaction: %v", err)
	}
}
