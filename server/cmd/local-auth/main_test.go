package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func writeDeploymentConfig(t *testing.T, payload any) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "deployment.json")
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal deployment config: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write deployment config: %v", err)
	}
	return path
}

func TestDatabaseURLFromDeploymentConfig(t *testing.T) {
	// The password deliberately carries characters that must be URL-escaped:
	// a raw interpolation would produce an unparseable DSN.
	path := writeDeploymentConfig(t, map[string]any{
		"Database":         "multica_verify",
		"DatabaseUser":     "multica",
		"DatabasePassword": "p@ss:word/?#%and spaces",
		"DatabasePort":     16442,
		// Fields the tool must ignore rather than print:
		"Root":     "D:\\UWorkers\\verify",
		"JWT_SECRET": "should-not-appear",
	})

	dsn, err := databaseURLFromDeploymentConfig(path)
	if err != nil {
		t.Fatalf("databaseURLFromDeploymentConfig: %v", err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("derived DSN is not a valid URL: %v", err)
	}
	if parsed.Scheme != "postgresql" {
		t.Fatalf("scheme = %q, want postgresql", parsed.Scheme)
	}
	if parsed.Host != "127.0.0.1:16442" {
		t.Fatalf("host = %q, want 127.0.0.1:16442 (the tool always connects over loopback)", parsed.Host)
	}
	if parsed.Path != "/multica_verify" {
		t.Fatalf("path = %q, want /multica_verify", parsed.Path)
	}
	password, _ := parsed.User.Password()
	if password != "p@ss:word/?#%and spaces" {
		t.Fatalf("password round-trip = %q, want the original value", password)
	}
	if parsed.Query().Get("sslmode") != "disable" {
		t.Fatalf("sslmode = %q, want disable for the loopback native database", parsed.Query().Get("sslmode"))
	}
	if strings.Contains(dsn, "should-not-appear") {
		t.Fatal("derived DSN carries unrelated deployment fields")
	}
}

func TestDatabaseURLFromDeploymentConfigRejectsBadInput(t *testing.T) {
	missing := writeDeploymentConfig(t, map[string]any{"DatabasePort": 16442})
	emptyPassword := writeDeploymentConfig(t, map[string]any{
		"Database": "db", "DatabaseUser": "multica", "DatabasePassword": "", "DatabasePort": 16442,
	})
	invalid := filepath.Join(t.TempDir(), "deployment.json")
	if err := os.WriteFile(invalid, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}

	cases := []struct {
		name string
		path string
	}{
		{"missing_fields", missing},
		{"empty_password_is_allowed_but_fields_required", emptyPassword},
		{"invalid_json", invalid},
		{"missing_file", filepath.Join(t.TempDir(), "nope.json")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "empty_password_is_allowed_but_fields_required" {
				// A passwordless local role is legal; the fields that matter are
				// database name, user and port.
				if _, err := databaseURLFromDeploymentConfig(tc.path); err != nil {
					t.Fatalf("unexpected error for a complete config with an empty password: %v", err)
				}
				return
			}
			_, err := databaseURLFromDeploymentConfig(tc.path)
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), "postgresql://") || strings.Contains(strings.ToLower(err.Error()), "password") {
				t.Fatalf("error %q leaks connection details", err)
			}
		})
	}
}

func TestResolveDatabaseURLPrecedence(t *testing.T) {
	configPath := writeDeploymentConfig(t, map[string]any{
		"Database": "from_config", "DatabaseUser": "multica", "DatabasePassword": "secret", "DatabasePort": 16442,
	})

	t.Run("flag_wins", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://from:env@localhost:5432/from_env?sslmode=disable")
		got, err := resolveDatabaseURL("postgres://from:flag@localhost:5432/from_flag?sslmode=disable", configPath)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if !strings.Contains(got, "from_flag") {
			t.Fatalf("resolved %q, want the explicit flag", got)
		}
	})
	t.Run("env_beats_deployment_config", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://from:env@localhost:5432/from_env?sslmode=disable")
		got, err := resolveDatabaseURL("", configPath)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if !strings.Contains(got, "from_env") {
			t.Fatalf("resolved %q, want DATABASE_URL", got)
		}
	})
	t.Run("deployment_config_fallback", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "")
		got, err := resolveDatabaseURL("", configPath)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if !strings.Contains(got, "from_config") {
			t.Fatalf("resolved %q, want the deployment config database", got)
		}
	})
	t.Run("nothing_configured", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "")
		if _, err := resolveDatabaseURL("", ""); err == nil {
			t.Fatal("expected an error when no connection source is configured")
		}
	})
}

// --- database-backed lifecycle ---------------------------------------------

func testPoolOrSkip(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		t.Skip("local-auth lifecycle test requires DATABASE_URL")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("database not reachable: %v", err)
	}
	return pool
}

func insertTestUser(t *testing.T, pool *pgxpool.Pool, email string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO "user" (name, email) VALUES ($1, $2) RETURNING id`, "local-auth test", email).Scan(&id); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM local_password_credential WHERE user_id = $1`, id)
		_, _ = pool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, id)
	})
	return id
}

// withPipedStdin replaces os.Stdin with a pipe carrying value, so the
// --password-stdin path can be exercised without an interactive terminal.
func withPipedStdin(t *testing.T, value string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}
	if _, err := io.WriteString(w, value+"\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = w.Close()
	original := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = original
		_ = r.Close()
	})
}

func TestLocalAuthCredentialLifecycle(t *testing.T) {
	pool := testPoolOrSkip(t)
	queries := db.New(pool)
	ctx := context.Background()
	email := "local-auth-lifecycle@example.com"
	username := "lifecycle.user"
	userID := insertTestUser(t, pool, email)
	var stdout, stderr bytes.Buffer

	// Create.
	withPipedStdin(t, "first password value")
	if err := runCreate(ctx, queries, email, "", username, true, &stdout, &stderr); err != nil {
		t.Fatalf("create credential: %v", err)
	}
	credential, err := queries.GetLocalPasswordCredentialByUsername(ctx, username)
	if err != nil {
		t.Fatalf("created credential not found: %v", err)
	}
	if util.UUIDToString(credential.UserID) != userID {
		t.Fatalf("credential user = %s, want %s", util.UUIDToString(credential.UserID), userID)
	}
	if !auth.VerifyLocalPassword(credential.PasswordHash, "first password value") {
		t.Fatal("stored hash does not verify the provisioned password")
	}
	if strings.Contains(stdout.String(), "first password value") {
		t.Fatal("create output echoes the password")
	}
	if strings.Contains(credential.PasswordHash, "first password value") {
		t.Fatal("stored hash contains the plaintext password")
	}

	// Duplicate username is refused.
	otherID := insertTestUser(t, pool, "local-auth-lifecycle-other@example.com")
	withPipedStdin(t, "another password value")
	if err := runCreate(ctx, queries, "", util.UUIDToString(mustUUID(t, otherID)), username, true, &stdout, &stderr); err == nil {
		t.Fatal("creating a second credential with the same username succeeded")
	}

	// A second credential for the same user is refused (one credential per user).
	withPipedStdin(t, "another password value")
	if err := runCreate(ctx, queries, email, "", "lifecycle.second", true, &stdout, &stderr); err == nil {
		t.Fatal("creating a second credential for the same user succeeded")
	}

	// Reset: new password works, old one does not.
	withPipedStdin(t, "second password value")
	if err := runReset(ctx, queries, username, true, &stdout, &stderr); err != nil {
		t.Fatalf("reset credential: %v", err)
	}
	credential, err = queries.GetLocalPasswordCredentialByUsername(ctx, username)
	if err != nil {
		t.Fatalf("credential after reset: %v", err)
	}
	if !auth.VerifyLocalPassword(credential.PasswordHash, "second password value") {
		t.Fatal("reset did not install the new password")
	}
	if auth.VerifyLocalPassword(credential.PasswordHash, "first password value") {
		t.Fatal("the old password still works after a reset")
	}

	// List shows the mapping without hashes.
	stdout.Reset()
	if err := runList(ctx, queries, &stdout); err != nil {
		t.Fatalf("list credentials: %v", err)
	}
	if !strings.Contains(stdout.String(), username) || !strings.Contains(stdout.String(), email) {
		t.Fatalf("list output missing the mapping: %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "$2a$") || strings.Contains(stdout.String(), credential.PasswordHash) {
		t.Fatal("list output exposes the password hash")
	}

	// Delete.
	if err := runDelete(ctx, queries, username, &stdout); err != nil {
		t.Fatalf("delete credential: %v", err)
	}
	if _, err := queries.GetLocalPasswordCredentialByUsername(ctx, username); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("credential still present after delete: %v", err)
	}
	if err := runDelete(ctx, queries, username, &stdout); err == nil {
		t.Fatal("deleting a missing credential succeeded")
	}

	// The user itself survives a credential delete.
	if _, err := queries.GetUser(ctx, mustUUID(t, userID)); err != nil {
		t.Fatalf("user row was removed with the credential: %v", err)
	}
}

func TestLocalAuthCreateRejectsUnknownUserAndUsernameCase(t *testing.T) {
	pool := testPoolOrSkip(t)
	queries := db.New(pool)
	ctx := context.Background()
	email := "local-auth-case@example.com"
	userID := insertTestUser(t, pool, email)
	_ = userID
	var stdout, stderr bytes.Buffer

	withPipedStdin(t, "first password value")
	if err := runCreate(ctx, queries, "missing-user@example.com", "", "case.user", true, &stdout, &stderr); err == nil {
		t.Fatal("creating a credential for an unknown user succeeded")
	}

	// Upper-case input is normalized, not created as a second username.
	withPipedStdin(t, "first password value")
	if err := runCreate(ctx, queries, email, "", "Case.User", true, &stdout, &stderr); err != nil {
		t.Fatalf("create credential: %v", err)
	}
	if _, err := queries.GetLocalPasswordCredentialByUsername(ctx, "case.user"); err != nil {
		t.Fatalf("username was not normalized to lower case: %v", err)
	}
}

func mustUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	uuid, err := util.ParseUUID(raw)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", raw, err)
	}
	return uuid
}
