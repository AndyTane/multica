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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
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

Flags (all commands):
  --database-url <url>          PostgreSQL connection string. Falls back to DATABASE_URL.
  --deployment-config <path>    A native Windows deployment.json to read the connection
                                from. Only used when neither of the above is set.

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
	if command != "create" && command != "reset" && command != "delete" && command != "list" {
		fmt.Fprint(stderr, usageText)
		return fmt.Errorf("unknown command %q", command)
	}

	fs := flag.NewFlagSet("local-auth "+command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var databaseURL, deploymentConfig, userEmail, userID, username string
	var passwordStdin bool
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
		return runCreate(ctx, queries, userEmail, userID, username, passwordStdin, stdout, stderr)
	case "reset":
		return runReset(ctx, queries, username, passwordStdin, stdout, stderr)
	case "delete":
		return runDelete(ctx, queries, username, stdout)
	default:
		return runList(ctx, queries, stdout)
	}
}

func runCreate(ctx context.Context, q *db.Queries, userEmail, userID, username string, passwordStdin bool, stdout, stderr io.Writer) error {
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

	credential, err := q.CreateLocalPasswordCredential(ctx, db.CreateLocalPasswordCredentialParams{
		UserID:             target.ID,
		UsernameNormalized: normalized,
		PasswordHash:       hash,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return errors.New("that username or user was provisioned concurrently; run the command again")
		}
		return fmt.Errorf("create credential: %w", err)
	}
	fmt.Fprintf(stdout, "Created credential %q for %s (user %s).\n", credential.UsernameNormalized, target.Email, util.UUIDToString(target.ID))
	return nil
}

func runReset(ctx context.Context, q *db.Queries, username string, passwordStdin bool, stdout, stderr io.Writer) error {
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
	user, err := q.GetUser(ctx, credential.UserID)
	if err != nil {
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
	if _, err := q.UpdateLocalPasswordCredentialHash(ctx, db.UpdateLocalPasswordCredentialHashParams{
		UsernameNormalized: normalized,
		PasswordHash:       hash,
	}); err != nil {
		return fmt.Errorf("update credential: %w", err)
	}
	fmt.Fprintf(stdout, "Password reset for %q (%s). The previous password no longer works.\n", normalized, user.Email)
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
