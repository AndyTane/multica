package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/server/internal/util"
)

// P5-R2: local_password_credential and registration_invite have no foreign key,
// so a CLI write that resolves an account before the delete transaction takes
// its lock must re-check the live row after taking the same lock. Otherwise the
// insert lands after the delete's final scan and survives as an orphan.
//
// The test holds the account key, lets both writes resolve the account and
// queue on the lock, deletes the account without taking that key (a plain
// DELETE, the outcome the handler commits), then releases the lock. Whichever
// side wins the race, no orphan row may remain.
func TestAccountWritesCannotFollowADeletedAccount(t *testing.T) {
	pool := testPoolOrSkip(t)
	ctx := context.Background()
	email := "p5r2-cli-deleted@example.com"
	userID := insertTestUser(t, pool, email)
	userUUID := mustUUID(t, userID)

	holder, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire lock connection: %v", err)
	}
	defer holder.Release()
	lockKey := "user:" + util.UUIDToString(userUUID)
	if _, err := holder.Exec(ctx, "SELECT pg_advisory_lock(hashtextextended($1, 0))", lockKey); err != nil {
		t.Fatalf("hold account lock: %v", err)
	}
	defer func() {
		_, _ = holder.Exec(context.Background(), "SELECT pg_advisory_unlock(hashtextextended($1, 0))", lockKey)
	}()

	withPipedStdin(t, "a sufficiently long password")
	credentialErr := make(chan error, 1)
	go func() {
		var stdout, stderr bytes.Buffer
		credentialErr <- runCreate(ctx, pool, email, "", "p5r2.deleted.race", true, &stdout, &stderr)
	}()
	inviteErr := make(chan error, 1)
	go func() {
		var stdout bytes.Buffer
		inviteErr <- runInvite(ctx, pool, email, "", 15, true, &stdout)
	}()

	// Let both commands resolve the account and block on the account key.
	time.Sleep(300 * time.Millisecond)
	if _, err := pool.Exec(ctx, `DELETE FROM "user" WHERE id = $1`, userID); err != nil {
		t.Fatalf("delete account: %v", err)
	}
	if _, err := holder.Exec(ctx, "SELECT pg_advisory_unlock(hashtextextended($1, 0))", lockKey); err != nil {
		t.Fatalf("release account lock: %v", err)
	}

	if err := <-credentialErr; err == nil {
		t.Fatal("credential write succeeded for a deleted account")
	}
	if err := <-inviteErr; err == nil {
		t.Fatal("invitation write succeeded for a deleted account")
	}
	if got := countForUser(t, pool, "local_password_credential", userID); got != 0 {
		t.Fatalf("%d orphan credentials remain", got)
	}
	if got := countForUser(t, pool, "registration_invite", userID); got != 0 {
		t.Fatalf("%d orphan invitations remain", got)
	}
}

func countForUser(t *testing.T, pool *pgxpool.Pool, table, userID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM "+table+" WHERE user_id = $1", mustUUID(t, userID)).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}
