// Command local-auth manages the local account/password credentials that back
// POST /auth/password on a self-hosted deployment.
//
// It is an operator tool, not a service: it must run on the host that owns the
// database, reads the password without echo (or from stdin with
// --password-stdin), and never prints a password, a hash or the database
// connection string. There is deliberately no web or API path that creates
// credentials; provisioning is a local administrative action.
//
// Usage:
//
//	local-auth create --user-email you@example.com [--username you]
//	local-auth reset  --username you
//	local-auth delete --username you
//	local-auth list
//
// The database connection comes from --database-url, DATABASE_URL, or the
// deployment.json of a native Windows deployment (--deployment-config).
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"golang.org/x/term"
)

const usageText = `Manage local account/password credentials for the login at POST /auth/password.

Usage:
  local-auth <command> [flags]

Commands:
  create   Create a credential for an existing user. Fails if the user already has one.
  reset    Replace the password of an existing credential. The old password stops working.
  delete   Remove a credential. The user keeps their account and other login methods.
  list     Show usernames and the user each one signs in as.
  invite    Mint a short-lived web registration invitation for an existing user
            without a password. The raw secret is NOT printed by default: stdout
            carries only the invitation id, expiry and the SHA-256 digest that is
            stored. Pass --stdout-token only from a host-local helper that
            consumes the value in memory; the raw secret must never be written
            to disk, logs or a terminal transcript.
  bootstrap-host  One-time host onboarding for the pinned operator identity.
            Requires --user-id and a deployment config with an HTTPS origin.
            Verifies the account's normalised email digest, then either opens
            the private sign-in page (the account already has a password) or
            mints a 15-minute first-time invitation and opens the private setup
            page with the secret in the URL fragment. The secret is never
            printed, logged or written to disk.
  grant-admin  Mark one existing user as an instance administrator. The host
            bootstrap helper is responsible for verifying the pinned identity
            before calling this.

Flags (all commands):
  --database-url <url>          PostgreSQL connection string. Falls back to DATABASE_URL.
  --deployment-config <path>    A native Windows deployment.json to read the connection
                                from. Only used when neither of the above is set.

Invite flags:
  --user-email <email> | --user-id <uuid>   The existing user the invitation binds to.
  --minutes <n>          Lifetime in minutes, default 15 (hard cap 24h).
  --stdout-token         Print the raw secret on stdout. Only for a host helper
                         that keeps it in memory; never for a logged terminal.

Bootstrap-host flags:
  --user-id <uuid>       The pinned operator account UUID. Required.
  --minutes <n>          Invitation lifetime, capped at 15 minutes.
  --deployment-config <path>  Read the database and PublicOrigin from a native
                         Windows deployment.json (the origin must be HTTPS).

Create flags:
  --user-email <email>   The existing user the credential signs in as.
  --user-id <uuid>       Alternative to --user-email.
  --username <name>      Account name. 3-64 characters: letters, digits, '.', '_', '-'.
                         Lowercased for storage and comparison. Prompted when omitted.

Reset/delete flags:
  --username <name>      The account name of the credential.

Password input:
  Interactive terminals are prompted twice with no echo. Scripts must pass
  --password-stdin and pipe the password on stdin; passwords are never accepted
  as command-line arguments.

Policy: passwords are at least 12 characters and at most 72 UTF-8 bytes, hashed
with bcrypt (cost 12). Usernames are case-insensitive and independent of email.
`

// stdin is the reader used for prompts; a variable so tests can drive it.
var stdin io.Reader = os.Stdin

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "local-auth:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return errors.New("a command is required")
	}
	command := args[0]
	if command == "help" || command == "-h" || command == "--help" {
		fmt.Fprint(stdout, usageText)
		return nil
	}
	if command != "create" && command != "reset" && command != "delete" && command != "list" && command != "invite" && command != "grant-admin" && command != "bootstrap-host" {
		fmt.Fprint(stderr, usageText)
		return fmt.Errorf("unknown command %q", command)
	}

	fs := flag.NewFlagSet("local-auth "+command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var databaseURL, deploymentConfig, userEmail, userID, username string
	var inviteMinutes int
	var passwordStdin, stdoutToken bool
	fs.StringVar(&databaseURL, "database-url", "", "PostgreSQL connection string (falls back to DATABASE_URL)")
	fs.StringVar(&deploymentConfig, "deployment-config", "", "native deployment.json to read the connection from")
	if command == "create" {
		fs.StringVar(&userEmail, "user-email", "", "existing user email the credential signs in as")
		fs.StringVar(&userID, "user-id", "", "existing user id the credential signs in as")
		fs.StringVar(&username, "username", "", "account name (prompted when omitted)")
		fs.BoolVar(&passwordStdin, "password-stdin", false, "read the password from stdin instead of prompting")
	}
	if command == "reset" {
		fs.StringVar(&username, "username", "", "account name of the credential")
		fs.BoolVar(&passwordStdin, "password-stdin", false, "read the password from stdin instead of prompting")
	}
	if command == "delete" {
		fs.StringVar(&username, "username", "", "account name of the credential")
	}
	if command == "invite" {
		fs.StringVar(&userEmail, "user-email", "", "existing user email the invitation binds to")
		fs.StringVar(&userID, "user-id", "", "existing user id the invitation binds to")
		fs.IntVar(&inviteMinutes, "minutes", 15, "invitation lifetime in minutes (max 24h)")
		fs.BoolVar(&stdoutToken, "stdout-token", false, "print the raw secret on stdout (host helper only)")
	}
	if command == "bootstrap-host" {
		// --deployment-config is already registered for every command above:
		// registering it again here panicked flag.FlagSet and made the entry
		// unusable on the host.
		fs.StringVar(&userID, "user-id", "", "pinned operator account UUID (required)")
		fs.IntVar(&inviteMinutes, "minutes", 15, "invitation lifetime in minutes (max 15)")
	}
	if command == "grant-admin" {
		fs.StringVar(&userEmail, "user-email", "", "existing user email to grant")
		fs.StringVar(&userID, "user-id", "", "existing user id to grant")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	dsn, err := resolveDatabaseURL(databaseURL, deploymentConfig)
	if err != nil {
		return err
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		// pgx embeds the connection string in some errors; report only that the
		// configuration could not be used.
		return errors.New("cannot parse the database connection configuration")
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return errors.New("cannot reach the database")
	}
	queries := db.New(pool)

	switch command {
	case "create":
		return runCreate(ctx, pool, userEmail, userID, username, passwordStdin, stdout, stderr)
	case "reset":
		return runReset(ctx, pool, username, passwordStdin, stdout, stderr)
	case "delete":
		return runDelete(ctx, queries, username, stdout)
	case "invite":
		return runInvite(ctx, pool, userEmail, userID, inviteMinutes, stdoutToken, stdout)
	case "bootstrap-host":
		origin, err := resolvePublicOrigin(deploymentConfig)
		if err != nil {
			return err
		}
		return runBootstrapHost(ctx, pool, userID, origin, inviteMinutes, stdout)
	case "grant-admin":
		return runGrantAdmin(ctx, queries, userEmail, userID, stdout)
	default:
		return runList(ctx, queries, stdout)
	}
}

// lockLiveAccount takes the account's advisory key inside the caller's
// transaction and re-reads the row under it. Account deletion takes the same
// key before its dependency scan, so every credential or invitation write in
// this tool either commits before that scan (and is deleted with the account)
// or observes the account is gone and writes nothing. local_password_credential
// and registration_invite have no foreign key, so this is the only protection
// against an orphan row.
func lockLiveAccount(ctx context.Context, q *db.Queries, userID pgtype.UUID) (db.User, error) {
	if err := q.LockAccountKey(ctx, "user:"+util.UUIDToString(userID)); err != nil {
		return db.User{}, errors.New("cannot serialize the account write")
	}
	user, err := q.GetUser(ctx, userID)
	if err != nil {
		if isNoRows(err) {
			return db.User{}, errors.New("the account no longer exists; nothing was written")
		}
		return db.User{}, errors.New("cannot read the account")
	}
	return user, nil
}

func runCreate(ctx context.Context, pool *pgxpool.Pool, userEmail, userID, username string, passwordStdin bool, stdout, stderr io.Writer) error {
	q := db.New(pool)
	target, err := resolveUser(ctx, q, userEmail, userID)
	if err != nil {
		return err
	}
	if username == "" {
		name, err := promptLine(stderr, "Username: ")
		if err != nil {
			return err
		}
		username = name
	}
	normalized, err := auth.NormalizeLocalUsername(username)
	if err != nil {
		return fmt.Errorf("invalid username: use %d-%d letters, digits, '.', '_' or '-'", auth.LocalUsernameMinRunes, auth.LocalUsernameMaxRunes)
	}
	if _, err := q.GetLocalPasswordCredentialByUserID(ctx, target.ID); err == nil {
		return fmt.Errorf("user %s already has a local password credential; use 'local-auth reset --username <name>' to change its password", target.Email)
	} else if !isNoRows(err) {
		return fmt.Errorf("check existing credential: %w", err)
	}
	if _, err := q.GetLocalPasswordCredentialByUsername(ctx, normalized); err == nil {
		return fmt.Errorf("username %q is already in use", normalized)
	} else if !isNoRows(err) {
		return fmt.Errorf("check username: %w", err)
	}

	password, err := readPassword(passwordStdin, stderr)
	if err != nil {
		return err
	}
	hash, err := auth.HashLocalPassword(password)
	if err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return errors.New("cannot start the credential transaction")
	}
	defer tx.Rollback(ctx)
	qtx := db.New(tx)
	live, err := lockLiveAccount(ctx, qtx, target.ID)
	if err != nil {
		return err
	}
	if _, err := qtx.GetLocalPasswordCredentialByUserID(ctx, live.ID); err == nil {
		return fmt.Errorf("user %s already has a local password credential; use 'local-auth reset --username <name>' to change its password", live.Email)
	} else if !isNoRows(err) {
		return fmt.Errorf("check existing credential: %w", err)
	}
	credential, err := qtx.CreateLocalPasswordCredential(ctx, db.CreateLocalPasswordCredentialParams{
		UserID:             live.ID,
		UsernameNormalized: normalized,
		PasswordHash:       hash,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return errors.New("that username or user was provisioned concurrently; run the command again")
		}
		return fmt.Errorf("create credential: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.New("cannot commit the credential")
	}
	fmt.Fprintf(stdout, "Created credential %q for %s (user %s).\n", credential.UsernameNormalized, live.Email, util.UUIDToString(live.ID))
	return nil
}

func runReset(ctx context.Context, pool *pgxpool.Pool, username string, passwordStdin bool, stdout, stderr io.Writer) error {
	q := db.New(pool)
	if username == "" {
		name, err := promptLine(stderr, "Username: ")
		if err != nil {
			return err
		}
		username = name
	}
	normalized, err := auth.NormalizeLocalUsername(username)
	if err != nil {
		return errors.New("invalid username")
	}
	credential, err := q.GetLocalPasswordCredentialByUsername(ctx, normalized)
	if err != nil {
		if isNoRows(err) {
			return fmt.Errorf("no credential found for username %q", normalized)
		}
		return fmt.Errorf("look up credential: %w", err)
	}
	if _, err := q.GetUser(ctx, credential.UserID); err != nil {
		if isNoRows(err) {
			return errors.New("the credential's user no longer exists; delete the credential and provision the intended user")
		}
		return fmt.Errorf("look up user: %w", err)
	}
	password, err := readPassword(passwordStdin, stderr)
	if err != nil {
		return err
	}
	hash, err := auth.HashLocalPassword(password)
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return errors.New("cannot start the credential transaction")
	}
	defer tx.Rollback(ctx)
	qtx := db.New(tx)
	live, err := lockLiveAccount(ctx, qtx, credential.UserID)
	if err != nil {
		return err
	}
	if _, err := qtx.UpdateLocalPasswordCredentialHash(ctx, db.UpdateLocalPasswordCredentialHashParams{
		UsernameNormalized: normalized,
		PasswordHash:       hash,
	}); err != nil {
		return fmt.Errorf("update credential: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.New("cannot commit the credential")
	}
	fmt.Fprintf(stdout, "Password reset for %q (%s). The previous password no longer works.\n", normalized, live.Email)
	return nil
}

func runDelete(ctx context.Context, q *db.Queries, username string, stdout io.Writer) error {
	if username == "" {
		return errors.New("--username is required")
	}
	normalized, err := auth.NormalizeLocalUsername(username)
	if err != nil {
		return errors.New("invalid username")
	}
	affected, err := q.DeleteLocalPasswordCredentialByUsername(ctx, normalized)
	if err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("no credential found for username %q", normalized)
	}
	fmt.Fprintf(stdout, "Deleted credential %q.\n", normalized)
	return nil
}

func runList(ctx context.Context, q *db.Queries, stdout io.Writer) error {
	rows, err := q.ListLocalPasswordCredentials(ctx)
	if err != nil {
		return fmt.Errorf("list credentials: %w", err)
	}
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "No local password credentials.")
		return nil
	}
	fmt.Fprintf(stdout, "%-24s %-32s %s\n", "USERNAME", "EMAIL", "UPDATED")
	for _, row := range rows {
		fmt.Fprintf(stdout, "%-24s %-32s %s\n", row.UsernameNormalized, row.Email, row.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z"))
	}
	return nil
}

func resolveUser(ctx context.Context, q *db.Queries, userEmail, userID string) (db.User, error) {
	switch {
	case userEmail != "" && userID != "":
		return db.User{}, errors.New("pass one of --user-email or --user-id, not both")
	case userEmail != "":
		user, err := q.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(userEmail)))
		if err != nil {
			if isNoRows(err) {
				return db.User{}, fmt.Errorf("no user with email %q", userEmail)
			}
			return db.User{}, fmt.Errorf("look up user: %w", err)
		}
		return user, nil
	case userID != "":
		id, err := util.ParseUUID(userID)
		if err != nil {
			return db.User{}, errors.New("--user-id is not a valid UUID")
		}
		user, err := q.GetUser(ctx, id)
		if err != nil {
			if isNoRows(err) {
				return db.User{}, fmt.Errorf("no user with id %q", userID)
			}
			return db.User{}, fmt.Errorf("look up user: %w", err)
		}
		return user, nil
	default:
		return db.User{}, errors.New("pass --user-email or --user-id")
	}
}

// readPassword reads the new password once from a hidden terminal prompt (twice,
// with confirmation) or from stdin when --password-stdin was given. Nothing is
// ever echoed, logged or accepted from argv.
func readPassword(fromStdin bool, stderr io.Writer) (string, error) {
	if fromStdin {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			return "", errors.New("--password-stdin needs the password piped on stdin; omit the flag to use the hidden prompt")
		}
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", errors.New("read password from stdin")
		}
		password := strings.TrimRight(line, "\r\n")
		if password == "" {
			return "", errors.New("empty password on stdin")
		}
		if err := auth.ValidateLocalPassword(password); err != nil {
			return "", err
		}
		return password, nil
	}

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("stdin is not a terminal: run this command from a terminal, or pass --password-stdin with the password piped in")
	}
	fmt.Fprintf(stderr, "New password (%s): ", auth.LocalPasswordPolicy)
	first, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(stderr)
	if err != nil {
		return "", errors.New("read password")
	}
	if err := auth.ValidateLocalPassword(string(first)); err != nil {
		return "", err
	}
	fmt.Fprint(stderr, "Confirm new password: ")
	second, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(stderr)
	if err != nil {
		return "", errors.New("read password confirmation")
	}
	if string(first) != string(second) {
		return "", errors.New("passwords do not match")
	}
	return string(first), nil
}

func promptLine(stderr io.Writer, prompt string) (string, error) {
	fmt.Fprint(stderr, prompt)
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", errors.New("read input")
	}
	value := strings.TrimSpace(line)
	if value == "" {
		return "", errors.New("a value is required")
	}
	return value, nil
}

// resolveDatabaseURL picks the connection source in priority order: explicit
// flag, DATABASE_URL, then a native deployment.json.
func resolveDatabaseURL(databaseURL, deploymentConfig string) (string, error) {
	if strings.TrimSpace(databaseURL) != "" {
		return databaseURL, nil
	}
	if env := strings.TrimSpace(os.Getenv("DATABASE_URL")); env != "" {
		return env, nil
	}
	if strings.TrimSpace(deploymentConfig) != "" {
		return databaseURLFromDeploymentConfig(deploymentConfig)
	}
	return "", errors.New("no database connection configured: pass --database-url, set DATABASE_URL, or pass --deployment-config")
}

// databaseURLFromDeploymentConfig derives the loopback DSN from a native
// Windows deployment.json. Only the connection fields are read; the password is
// URL-escaped into the DSN and never returned to the caller verbatim.
func databaseURLFromDeploymentConfig(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("cannot read the deployment config")
	}
	var config struct {
		Database         string `json:"Database"`
		DatabaseUser     string `json:"DatabaseUser"`
		DatabasePassword string `json:"DatabasePassword"`
		DatabasePort     int    `json:"DatabasePort"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return "", errors.New("deployment config is not valid JSON")
	}
	if config.Database == "" || config.DatabaseUser == "" || config.DatabasePort <= 0 {
		return "", errors.New("deployment config is missing the database name, user or port")
	}
	dsn := url.URL{
		Scheme:   "postgresql",
		User:     url.UserPassword(config.DatabaseUser, config.DatabasePassword),
		Host:     fmt.Sprintf("127.0.0.1:%d", config.DatabasePort),
		Path:     "/" + config.Database,
		RawQuery: "sslmode=disable",
	}
	return dsn.String(), nil
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.SQLState() == "23505"
}

// runInvite mints a registration invitation for an existing user that has no
// password yet. Revoking the previous invitation and minting the new one share
// one transaction and one account lock, so the account ends up with exactly one
// valid invitation even when two operators run this at the same time.
//
// The raw secret is withheld by default: stdout carries the invitation id, the
// expiry and the stored SHA-256 digest. Only --stdout-token emits the raw value,
// for a host-local helper that consumes it in memory. Existing credentials are
// refused: this command is for first-time activation only, never a reset.
func runInvite(ctx context.Context, pool *pgxpool.Pool, userEmail, userID string, minutes int, stdoutToken bool, stdout io.Writer) error {
	if minutes <= 0 {
		minutes = 15
	}
	if minutes > 24*60 {
		minutes = 24 * 60
	}
	queries := db.New(pool)
	target, err := resolveUser(ctx, queries, userEmail, userID)
	if err != nil {
		return err
	}
	raw, err := auth.NewRegistrationInviteToken()
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return errors.New("cannot start the invitation transaction")
	}
	defer tx.Rollback(ctx)
	qtx := queries.WithTx(tx)
	if err := qtx.LockAccountKey(ctx, "user:"+util.UUIDToString(target.ID)); err != nil {
		return errors.New("cannot serialize the invitation")
	}
	// Re-read under the lock: a concurrent account deletion takes the same key,
	// and registration_invite has no foreign key. Without this check the mint
	// could commit after the delete's inventory scan and leave an orphan row.
	if _, err := qtx.GetUser(ctx, target.ID); err != nil {
		if isNoRows(err) {
			return errors.New("the account no longer exists; no invitation was created")
		}
		return errors.New("cannot read the account")
	}
	if _, err := qtx.GetLocalPasswordCredentialByUserID(ctx, target.ID); err == nil {
		return fmt.Errorf("user %s already has a password; invite is only for first-time activation", target.Email)
	} else if !isNoRows(err) {
		return fmt.Errorf("check existing credential: %w", err)
	}
	expiresAt := time.Now().Add(time.Duration(minutes) * time.Minute)
	if _, err := qtx.RevokeRegistrationInvitesForUser(ctx, target.ID); err != nil {
		return fmt.Errorf("revoke previous invitations: %w", err)
	}
	invite, err := qtx.CreateRegistrationInvite(ctx, db.CreateRegistrationInviteParams{
		UserID:    target.ID,
		TokenHash: auth.RegistrationTokenHash(raw),
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("create invitation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.New("cannot commit the invitation")
	}
	if stdoutToken {
		fmt.Fprintf(stdout, "invite=%s expires_at=%s id=%s\n", raw, invite.ExpiresAt.Time.UTC().Format(time.RFC3339), util.UUIDToString(invite.ID))
		return nil
	}
	fmt.Fprintf(stdout, "invite_id=%s expires_at=%s token_sha256=%s\n", util.UUIDToString(invite.ID), invite.ExpiresAt.Time.UTC().Format(time.RFC3339), auth.RegistrationTokenHash(raw))
	fmt.Fprintln(stdout, "The raw secret was withheld. A host helper passes --stdout-token only when it can consume the value in memory.")
	return nil
}

// runGrantAdmin sets the instance administrator flag on exactly the user
// resolved from --user-email/--user-id. The host helper validates the pinned
// identity (UUID + normalized email hash) before invoking this.
func runGrantAdmin(ctx context.Context, q *db.Queries, userEmail, userID string, stdout io.Writer) error {
	target, err := resolveUser(ctx, q, userEmail, userID)
	if err != nil {
		return err
	}
	if target.IsInstanceAdmin {
		fmt.Fprintf(stdout, "Instance administrator already granted for %s.\n", target.Email)
		return nil
	}
	if err := q.SetUserInstanceAdmin(ctx, db.SetUserInstanceAdminParams{ID: target.ID, IsInstanceAdmin: true}); err != nil {
		return fmt.Errorf("grant instance administrator: %w", err)
	}
	fmt.Fprintf(stdout, "Instance administrator granted for %s.\n", target.Email)
	return nil
}

// pinnedOperatorEmailSHA256 is the SHA-256 of the operator's canonical address
// (trimmed, lowercased, UTF-8). The one-time host bootstrap refuses any account
// whose address does not hash to this value, so a wrong --user-id or a renamed
// account can never be provisioned by accident.
var pinnedOperatorEmailSHA256 = "5983f9551dc3f2370d8f04a7291d1d03e1c7fb96eac1b5112791572728122788"

// bootstrapInviteMaxMinutes is the ceiling for the one-time host onboarding.
const bootstrapInviteMaxMinutes = 15

// runBootstrapHost is the candidate one-time host onboarding helper. It never
// prints, logs or stores the invitation secret: the value goes from this process
// into the URL fragment of a private browser window.
func runBootstrapHost(ctx context.Context, pool *pgxpool.Pool, userID, appOrigin string, minutes int, stdout io.Writer) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("--user-id is required: pass the pinned operator UUID explicitly")
	}
	id, err := util.ParseUUID(userID)
	if err != nil {
		return errors.New("--user-id is not a valid UUID")
	}
	if minutes <= 0 || minutes > bootstrapInviteMaxMinutes {
		minutes = bootstrapInviteMaxMinutes
	}
	queries := db.New(pool)
	target, err := queries.GetUser(ctx, id)
	if err != nil {
		if isNoRows(err) {
			return fmt.Errorf("no user with id %q", userID)
		}
		return errors.New("cannot read the account")
	}
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(target.Email))))
	if hex.EncodeToString(sum[:]) != pinnedOperatorEmailSHA256 {
		return errors.New("the resolved account does not match the pinned operator identity; nothing was changed")
	}
	existing := func() error {
		fmt.Fprintln(stdout, "This account already has a password; no invitation was minted.")
		if err := openBrowser(appOrigin + "/login"); err != nil {
			return fmt.Errorf("open the private sign-in page: %w", err)
		}
		fmt.Fprintln(stdout, "Opened the private sign-in page. Sign in with the existing password.")
		return nil
	}
	if _, err := queries.GetLocalPasswordCredentialByUserID(ctx, target.ID); err == nil {
		return existing()
	} else if !isNoRows(err) {
		return errors.New("cannot read the credential state")
	}
	raw, err := auth.NewRegistrationInviteToken()
	if err != nil {
		return errors.New("cannot generate an invitation")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return errors.New("cannot start the invitation transaction")
	}
	defer tx.Rollback(ctx)
	qtx := queries.WithTx(tx)
	if err := qtx.LockAccountKey(ctx, "user:"+util.UUIDToString(target.ID)); err != nil {
		return errors.New("cannot serialize the invitation")
	}
	// Re-check under the lock: a credential created between the read above and
	// this point must not be replaced by a first-time invitation, and an account
	// deleted in the same window must not receive an orphan invitation.
	if _, err := qtx.GetUser(ctx, target.ID); err != nil {
		if isNoRows(err) {
			return errors.New("the account no longer exists; no invitation was created")
		}
		return errors.New("cannot read the account")
	}
	if _, err := qtx.GetLocalPasswordCredentialByUserID(ctx, target.ID); err == nil {
		return existing()
	} else if !isNoRows(err) {
		return errors.New("cannot read the credential state")
	}
	expiresAt := time.Now().Add(time.Duration(minutes) * time.Minute)
	if _, err := qtx.RevokeRegistrationInvitesForUser(ctx, target.ID); err != nil {
		return fmt.Errorf("revoke previous invitations: %w", err)
	}
	invite, err := qtx.CreateRegistrationInvite(ctx, db.CreateRegistrationInviteParams{
		UserID:    target.ID,
		TokenHash: auth.RegistrationTokenHash(raw),
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("create invitation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.New("cannot commit the invitation")
	}
	fmt.Fprintf(stdout, "Invitation %s expires at %s (in %d minutes).\n", util.UUIDToString(invite.ID), invite.ExpiresAt.Time.UTC().Format(time.RFC3339), minutes)
	if err := openBrowser(appOrigin + "/register#token=" + raw); err != nil {
		return errors.New("cannot open the private setup page; the invitation expires shortly and its secret was never printed, so run the command again")
	}
	fmt.Fprintln(stdout, "Opened the private setup page. The link stays in the browser and is not printed, logged or written to disk.")
	return nil
}

// resolvePublicOrigin reads the deployment's public origin. Only a HTTPS origin
// is accepted (a loopback HTTP origin is tolerated for local candidates), so the
// helper can never hand the invitation to a plaintext or made-up host.
func resolvePublicOrigin(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("bootstrap-host needs --deployment-config to learn the HTTPS origin")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("cannot read the deployment config")
	}
	var config struct {
		PublicOrigin string `json:"PublicOrigin"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return "", errors.New("deployment config is not valid JSON")
	}
	return normalizePublicOrigin(config.PublicOrigin)
}

func normalizePublicOrigin(raw string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", errors.New("the deployment config has no PublicOrigin")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return "", errors.New("the deployment PublicOrigin is not a valid URL")
	}
	loopback := parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1"
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
		return "", errors.New("the deployment PublicOrigin must be HTTPS")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

// openBrowser is the side effect the bootstrap helper performs. A variable so
// tests can observe the URL without launching a real browser.
var openBrowser = openPrivateBrowser

// openPrivateBrowser opens target in a private browser window. Private is the
// point: the invitation lives in the address bar of a session that does not
// share the operator's everyday profile.
func openPrivateBrowser(target string) error {
	switch runtime.GOOS {
	case "windows":
		for _, candidate := range []struct {
			path string
			flag string
		}{
			{filepath.Join(os.Getenv("ProgramFiles(x86)"), `Microsoft\Edge\Application\msedge.exe`), "--inprivate"},
			{filepath.Join(os.Getenv("ProgramFiles"), `Microsoft\Edge\Application\msedge.exe`), "--inprivate"},
			{filepath.Join(os.Getenv("ProgramFiles"), `Google\Chrome\Application\chrome.exe`), "--incognito"},
			{filepath.Join(os.Getenv("ProgramFiles(x86)"), `Google\Chrome\Application\chrome.exe`), "--incognito"},
		} {
			if candidate.path == "" {
				continue
			}
			if _, err := os.Stat(candidate.path); err != nil {
				continue
			}
			return exec.Command(candidate.path, candidate.flag, target).Start()
		}
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	case "darwin":
		return exec.Command("open", "-n", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}
