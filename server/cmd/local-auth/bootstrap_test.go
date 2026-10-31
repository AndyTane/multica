package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// P5-R1: the one-time host bootstrap. It provisions exactly one pinned
// identity, refuses to replace an existing password, opens the setup page
// privately, and never prints the invitation secret.

// TestCommandFlagsParseWithoutPanic drives the real CLI entry point for every
// command. Registering a flag twice panics the FlagSet before any work runs, and
// that is exactly how `bootstrap-host` — the only host entry that can give the
// pinned operator a first password — failed on the deployed host. Parsing stops
// at --help, so this needs no database and cannot touch one.
func TestCommandFlagsParseWithoutPanic(t *testing.T) {
	for _, command := range []string{"create", "reset", "delete", "list", "invite", "grant-admin", "bootstrap-host"} {
		command := command
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run([]string{command, "--help"}, &stdout, &stderr)
			if !errors.Is(err, flag.ErrHelp) {
				t.Fatalf("%s --help: got %v, want flag.ErrHelp", command, err)
			}
		})
	}
}

func bootstrapEmailDigest(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:])
}

func seedBootstrapUser(t *testing.T, email string) (string, db.User) {
	t.Helper()
	pool := testPoolOrSkip(t)
	ctx := context.Background()
	var id string
	if err := pool.QueryRow(ctx, `DELETE FROM "user" WHERE email = $1`, email).Scan(); err != nil {
		_ = err
	}
	var user db.User
	row := pool.QueryRow(ctx, `INSERT INTO "user" (name, email) VALUES ('Bootstrap Operator', $1) RETURNING id`, email)
	if err := row.Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	queries := db.New(pool)
	uuid := mustUUID(t, id)
	user, err := queries.GetUser(ctx, uuid)
	if err != nil {
		t.Fatalf("load seeded user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM registration_invite WHERE user_id = $1`, uuid)
		_, _ = pool.Exec(context.Background(), `DELETE FROM local_password_credential WHERE user_id = $1`, uuid)
		_, _ = pool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, uuid)
	})
	return id, user
}

func TestBootstrapHostRefusesAnIdentityThatIsNotPinned(t *testing.T) {
	pool := testPoolOrSkip(t)
	opened := ""
	previousOpen := openBrowser
	openBrowser = func(target string) error {
		opened = target
		return nil
	}
	t.Cleanup(func() { openBrowser = previousOpen })

	id, _ := seedBootstrapUser(t, "p5r1-bootstrap-other@example.com")
	var stdout bytes.Buffer
	err := runBootstrapHost(context.Background(), pool, id, "https://example.test", 15, &stdout)
	if err == nil {
		t.Fatal("bootstrap accepted an account that does not match the pinned digest")
	}
	if opened != "" {
		t.Fatalf("bootstrap opened %q for an unverified identity", opened)
	}
	if count := countInvitesFor(t, id); count != 0 {
		t.Fatalf("bootstrap minted %d invitations for an unverified identity", count)
	}
}

func TestBootstrapHostMintsShortLivedInvitationWithoutPrintingTheSecret(t *testing.T) {
	pool := testPoolOrSkip(t)
	email := "p5r1-bootstrap@example.com"
	previousDigest := pinnedOperatorEmailSHA256
	pinnedOperatorEmailSHA256 = bootstrapEmailDigest(email)
	t.Cleanup(func() { pinnedOperatorEmailSHA256 = previousDigest })

	opened := ""
	previousOpen := openBrowser
	openBrowser = func(target string) error {
		opened = target
		return nil
	}
	t.Cleanup(func() { openBrowser = previousOpen })

	id, _ := seedBootstrapUser(t, email)
	var stdout bytes.Buffer
	if err := runBootstrapHost(context.Background(), pool, id, "https://example.test", 15, &stdout); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if !strings.HasPrefix(opened, "https://example.test/register#token=") {
		t.Fatalf("opened %q, want the private setup page", opened)
	}
	raw := strings.TrimPrefix(opened, "https://example.test/register#token=")
	if len(raw) != 43 {
		t.Fatalf("invitation secret length = %d, want 43 (256-bit base64url)", len(raw))
	}
	if strings.Contains(stdout.String(), raw) {
		t.Fatal("bootstrap printed the invitation secret")
	}
	if !strings.Contains(stdout.String(), "expires at") {
		t.Fatalf("bootstrap output = %q", stdout.String())
	}
	queries := db.New(pool)
	invite, err := queries.GetRegistrationInviteByTokenHash(context.Background(), auth.RegistrationTokenHash(raw))
	if err != nil {
		t.Fatalf("stored invitation: %v", err)
	}
	if invite.UserID != mustUUID(t, id) {
		t.Fatal("invitation is bound to the wrong account")
	}
	remaining := time.Until(invite.ExpiresAt.Time)
	if remaining < 14*time.Minute || remaining > 16*time.Minute {
		t.Fatalf("invitation expires in %s, want about 15 minutes", remaining)
	}
}

func TestBootstrapHostNeverReplacesAnExistingPassword(t *testing.T) {
	pool := testPoolOrSkip(t)
	email := "p5r1-bootstrap-existing@example.com"
	previousDigest := pinnedOperatorEmailSHA256
	pinnedOperatorEmailSHA256 = bootstrapEmailDigest(email)
	t.Cleanup(func() { pinnedOperatorEmailSHA256 = previousDigest })

	opened := ""
	previousOpen := openBrowser
	openBrowser = func(target string) error {
		opened = target
		return nil
	}
	t.Cleanup(func() { openBrowser = previousOpen })

	id, _ := seedBootstrapUser(t, email)
	hash, err := auth.HashLocalPassword("an existing password 123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := db.New(pool).CreateLocalPasswordCredential(context.Background(), db.CreateLocalPasswordCredentialParams{
		UserID: mustUUID(t, id), UsernameNormalized: "p5r1.bootstrap.existing", PasswordHash: hash,
	}); err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	var stdout bytes.Buffer
	if err := runBootstrapHost(context.Background(), pool, id, "https://example.test", 15, &stdout); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if opened != "https://example.test/login" {
		t.Fatalf("opened %q, want the sign-in page when a password already exists", opened)
	}
	if strings.Contains(stdout.String(), "register#token") {
		t.Fatal("bootstrap reported a setup link for an activated account")
	}
	if count := countInvitesFor(t, id); count != 0 {
		t.Fatalf("bootstrap minted %d invitations for an activated account", count)
	}
}

func TestInviteCommandWithholdsTheSecretUnlessExplicitlyRequested(t *testing.T) {
	pool := testPoolOrSkip(t)
	email := "p5r1-invite-print@example.com"
	id, _ := seedBootstrapUser(t, email)

	var withheld bytes.Buffer
	if err := runInvite(context.Background(), pool, email, "", 15, false, &withheld); err != nil {
		t.Fatalf("invite: %v", err)
	}
	if strings.Contains(withheld.String(), "invite=") {
		t.Fatalf("invite printed the raw secret by default: %q", withheld.String())
	}
	if !strings.Contains(withheld.String(), "token_sha256=") {
		t.Fatalf("invite output = %q, want the digest reference line", withheld.String())
	}

	var printed bytes.Buffer
	if err := runInvite(context.Background(), pool, email, "", 15, true, &printed); err != nil {
		t.Fatalf("invite with --stdout-token: %v", err)
	}
	if !strings.Contains(printed.String(), "invite=") {
		t.Fatalf("--stdout-token output = %q", printed.String())
	}
	if active := countInvitesFor(t, id); active != 1 {
		t.Fatalf("active invitations = %d, want exactly one", active)
	}
}

func TestResolvePublicOriginRequiresHTTPS(t *testing.T) {
	configPath := writeDeploymentConfig(t, map[string]any{"PublicOrigin": "http://example.test"})
	if _, err := resolvePublicOrigin(configPath); err == nil {
		t.Fatal("plaintext public origin accepted")
	}
	configPath = writeDeploymentConfig(t, map[string]any{"PublicOrigin": "https://uworkers.example.test/"})
	origin, err := resolvePublicOrigin(configPath)
	if err != nil {
		t.Fatalf("https origin rejected: %v", err)
	}
	if origin != "https://uworkers.example.test" {
		t.Fatalf("origin = %q, want the normalized HTTPS origin", origin)
	}
	configPath = writeDeploymentConfig(t, map[string]any{"PublicOrigin": "http://localhost:13444"})
	if _, err := resolvePublicOrigin(configPath); err != nil {
		t.Fatalf("loopback http origin rejected: %v", err)
	}
}

func countInvitesFor(t *testing.T, userID string) int {
	t.Helper()
	pool := testPoolOrSkip(t)
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM registration_invite WHERE user_id = $1 AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at > now()`,
		mustUUID(t, userID),
	).Scan(&count); err != nil {
		t.Fatalf("count invitations: %v", err)
	}
	return count
}

var _ = util.UUIDToString
