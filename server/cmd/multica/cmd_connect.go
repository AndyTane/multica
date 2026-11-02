package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/internal/tlsx"
)

// connectCmd is the second-machine entry point: one command that trusts the
// deployment's private root CA (when there is one), saves the server URLs,
// signs in, and starts the agent daemon.
//
// It deliberately reuses the same building blocks as setup/login/daemon start
// instead of reimplementing them: the only new behavior is the certificate
// handling and the runtime-online confirmation.
var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Connect this machine to a U Workers server and start the daemon",
	Long: `Connects a machine to an existing U Workers deployment in one step.

Use --root-ca when the server certificate is not publicly trusted (a LAN
self-hosted deployment with an internal CA). The certificate is verified
against the live server before it is trusted; TLS verification is never
disabled. On Windows the same certificate is added to the current user's
certificate store so the browser can open the HTTPS entry without a warning.

The command then saves the server URLs, signs you in (browser by default, or
--token), and starts the agent daemon, waiting until its runtime registers

with the server.`,
	// At most one positional is accepted so the space form of --token
	// ("--token mul_...") can be recovered in runConnect even though pflag does
	// not bind it as the flag value (same shape as `login --token`).
	Args: cobra.MaximumNArgs(1),
	RunE: runConnect,
}

func init() {
	connectCmd.Flags().String("server-url", "", "Server URL of the deployment (env: U_WORKERS_SERVER_URL, legacy MULTICA_SERVER_URL)")
	connectCmd.Flags().String("app-url", "", "Web app URL used for browser sign-in (defaults to the server URL origin for https servers)")
	connectCmd.Flags().String("root-ca", "", "Path to the deployment's public root CA certificate (PEM or DER). Required when the server certificate is not publicly trusted.")
	connectCmd.Flags().Bool("skip-browser-trust", false, "Do not add the root CA to this Windows user's certificate store (the CLI and daemon still trust it; the browser will show a certificate warning)")
	connectCmd.Flags().Bool("no-daemon", false, "Sign in only; do not start the agent daemon")
	connectCmd.Flags().String("token", "", "Sign in with a personal access token (mul_... user PAT or mcn_... Cloud Node PAT). Pass --token alone to be prompted.")
	connectCmd.Flags().Lookup("token").NoOptDefVal = tokenPromptSentinel
	connectCmd.Flags().String(callbackHostFlag, "", callbackHostFlagHelp)
}

func runConnect(cmd *cobra.Command, args []string) error {
	if err := requireHumanLocalCommand("connect"); err != nil {
		return err
	}
	profile := resolveProfile(cmd)
	existing, _ := cli.LoadCLIConfigForProfile(profile)

	serverURL := connectServerURL(cmd, existing)
	if serverURL == "" {
		return fmt.Errorf("a server URL is required: pass --server-url https://<host>[:port], set U_WORKERS_SERVER_URL, or run 'connect' interactively")
	}
	fmt.Fprintf(os.Stderr, "Server: %s\n", serverURL)

	appURL := connectAppURL(cmd, existing, serverURL)
	if appURL == "" {
		return fmt.Errorf("the browser sign-in needs a web app URL: pass --app-url https://<host>[:port] (or use --token for a headless sign-in)")
	}

	// --- certificate trust -------------------------------------------------
	var browserTrusted bool
	rootCAPath := strings.TrimSpace(flagValueIfChanged(cmd, "root-ca"))
	if rootCAPath != "" {
		certs, err := loadConnectRootCA(rootCAPath)
		if err != nil {
			return err
		}
		// Verify before persisting anything: a wrong --root-ca must leave the
		// machine exactly as it was.
		if err := verifyServerTrustsCA(serverURL, certs); err != nil {
			return err
		}
		persistedPath, err := storeConnectRootCA(profile, rootCAPath)
		if err != nil {
			return err
		}
		fingerprint := tlsx.FingerprintSHA256(certs[0])
		fmt.Fprintf(os.Stderr, "Root CA verified: %s\n", fingerprint)
		fmt.Fprintf(os.Stderr, "  stored:       %s\n", persistedPath)
		if err := os.Setenv(tlsx.RootCAEnv, persistedPath); err != nil {
			return fmt.Errorf("export root CA path: %w", err)
		}
		if err := tlsx.InstallFromEnv(); err != nil {
			return err
		}
		skipBrowserTrust, _ := cmd.Flags().GetBool("skip-browser-trust")
		if !skipBrowserTrust {
			browserTrusted, err = installBrowserRootCA(certs[0])
			if err != nil {
				return err
			}
			if browserTrusted {
				fmt.Fprintln(os.Stderr, "  browser:      trusted for the current Windows user")
			}
		}
	} else {
		// No root CA provided: the server must already be publicly trusted. Prove
		// it here so a missing --root-ca fails with an instruction instead of a
		// later TLS error from the first API call.
		if err := verifyServerReachable(serverURL); err != nil {
			return err
		}
	}

	// --- persist the connection -------------------------------------------
	cfg, err := cli.LoadCLIConfigForProfile(profile)
	if err != nil {
		cfg = existing
	}
	cfg.ServerURL = serverURL
	cfg.AppURL = appURL
	if rootCAPath != "" {
		persistedPath, err := cli.RootCAFilePath(profile)
		if err != nil {
			return err
		}
		cfg.RootCAFile = persistedPath
	}
	if err := cli.SaveCLIConfigForProfile(cfg, profile); err != nil {
		return fmt.Errorf("save configuration: %w", err)
	}

	// --- sign in ------------------------------------------------------------
	if cmd.Flags().Changed("token") {
		tokenFlag, _ := cmd.Flags().GetString("token")
		if tokenFlag == tokenPromptSentinel && len(args) == 1 {
			tokenFlag = args[0]
		}
		if err := runAuthLoginToken(cmd, tokenFlag); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Signed in with a personal access token.")
		if err := autoWatchWorkspaces(cmd); err != nil {
			fmt.Fprintf(os.Stderr, "\nCould not auto-configure workspaces: %v\n", err)
		}
	} else {
		if err := runAuthLoginBrowser(cmd); err != nil {
			return err
		}
		if err := autoWatchWorkspaces(cmd); err != nil {
			fmt.Fprintf(os.Stderr, "\nCould not auto-configure workspaces: %v\n", err)
		}
	}

	// --- start the daemon ---------------------------------------------------
	noDaemon, _ := cmd.Flags().GetBool("no-daemon")
	if noDaemon {
		printConnectSummary(cmd, serverURL, appURL, browserTrusted, 0)
		return nil
	}

	fmt.Fprintln(os.Stderr, "\nStarting the agent daemon...")
	if err := runDaemonAfterSetup(cmd, args); err != nil {
		return err
	}
	runtimes := waitForRegisteredRuntimes(profile, 15*time.Second)
	printConnectSummary(cmd, serverURL, appURL, browserTrusted, len(runtimes))
	if len(runtimes) == 0 {
		fmt.Fprintln(os.Stderr, "\nNo agent runtime registered yet: this machine has no supported agent CLI on PATH.")
		fmt.Fprintln(os.Stderr, "Install one (for example codex or claude), then run 'daemon restart'; the daemon picks it up automatically.")
	}
	return nil
}

// connectServerURL resolves the deployment URL from the flag, the environment,
// the existing config, and finally an interactive prompt.
func connectServerURL(cmd *cobra.Command, existing cli.CLIConfig) string {
	raw := cli.FlagOrEnv(cmd, "server-url", "MULTICA_SERVER_URL", "")
	if raw == "" {
		raw = existing.ServerURL
	}
	if raw == "" {
		entered, err := promptLine(fmt.Sprintf("Server URL (e.g. https://192.168.1.10:13443): "))
		if err != nil {
			return ""
		}
		raw = entered
	}
	return normalizeAPIBaseURL(strings.TrimSpace(raw))
}

// connectAppURL resolves the browser app URL. A single-origin HTTPS deployment
// (the Windows LAN entry serves both the app and the API on one origin) needs no
// second value, so that case defaults to the server URL.
func connectAppURL(cmd *cobra.Command, existing cli.CLIConfig, serverURL string) string {
	raw := cli.FlagOrEnv(cmd, "app-url", "MULTICA_APP_URL", "")
	if raw == "" {
		raw = existing.AppURL
	}
	if raw == "" {
		if derived, ok := sameOriginAppURL(serverURL); ok {
			return derived
		}
		entered, err := promptLine("Web app URL (e.g. https://192.168.1.10:13443): ")
		if err != nil {
			return ""
		}
		raw = entered
	}
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

// sameOriginAppURL returns the origin of an https server URL: the LAN HTTPS
// entry in deploy/windows-https proxies both the web app and the API on one
// origin, which makes --app-url optional there.
func sameOriginAppURL(serverURL string) (string, bool) {
	parsed, err := url.Parse(serverURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", false
	}
	return parsed.Scheme + "://" + parsed.Host, true
}

// loadConnectRootCA reads and validates the supplied certificate file, keeping
// only CA certificates as trust anchors. It does not write anything: the
// certificate must be verified against the live server first.
func loadConnectRootCA(path string) ([]*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read root CA %s: %w", path, err)
	}
	certs, err := tlsx.ParseCertificates(data)
	if err != nil {
		return nil, fmt.Errorf("root CA %s: %w", path, err)
	}
	var anchors []*x509.Certificate
	for _, cert := range certs {
		if cert.IsCA {
			anchors = append(anchors, cert)
		}
	}
	if len(anchors) == 0 {
		return nil, fmt.Errorf("root CA %s does not contain a CA certificate", path)
	}
	return anchors, nil
}

// storeConnectRootCA copies the verified certificate into the profile's trust
// directory and returns that path. It runs only after the certificate proved it
// signs the live server chain.
func storeConnectRootCA(profile, path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read root CA %s: %w", path, err)
	}
	target, err := cli.RootCAFilePath(profile)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("create trust directory: %w", err)
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return "", fmt.Errorf("store root CA: %w", err)
	}
	return target, nil
}

// verifyServerTrustsCA proves the supplied CA actually signs the live server
// certificate. It verifies against the supplied CA only (not the system pool)
// so a CA that did not issue this server's certificate fails here instead of
// being silently ignored, and it never disables verification.
func verifyServerTrustsCA(serverURL string, anchors []*x509.Certificate) error {
	parsed, err := url.Parse(serverURL)
	if err != nil || parsed.Hostname() == "" {
		return fmt.Errorf("invalid server URL %q", serverURL)
	}
	if parsed.Scheme != "https" {
		// A CA can only be verified against a TLS handshake. Accepting a plain
		// HTTP /health answer here would mark an arbitrary certificate as
		// "verified" and persist/import it, so the combination is refused before
		// anything is written.
		return fmt.Errorf("cannot verify a root CA over %s: the server URL is not HTTPS, so no server certificate exists to check the CA against\\nuse the https:// address of the deployment, or drop --root-ca when the server certificate is publicly trusted", serverURL)
	}
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	pool := x509.NewCertPool()
	for _, cert := range anchors {
		pool.AddCert(cert)
	}
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(parsed.Hostname(), port), &tls.Config{
		RootCAs:    pool,
		ServerName: parsed.Hostname(),
		MinVersion: tls.VersionTLS12,
	})
	if err != nil {
		return fmt.Errorf("the server's TLS certificate is not issued by %s: %w\nchecked %s; if the certificate was reissued, export the current root CA again", singleCertLabel(anchors), err, serverURL)
	}
	conn.Close()
	return nil
}

// verifyServerReachable checks that <server>/health answers over the currently
// trusted TLS roots. Called when no --root-ca was supplied so a missing CA is
// reported as such before any configuration is written.
func verifyServerReachable(serverURL string) error {
	// Use the process's configured trust (a root CA saved by an earlier connect
	// run) rather than the system pool: after the operator chose to skip the
	// browser trust, the CLI and daemon must still reach a LAN deployment whose
	// certificate is signed by that saved CA.
	client := &http.Client{Timeout: 8 * time.Second, Transport: tlsx.HTTPTransport()}
	resp, err := client.Get(strings.TrimRight(serverURL, "/") + "/health")
	if err != nil {
		return fmt.Errorf("cannot reach %s: %w\nif the deployment uses an internal CA, pass --root-ca <root-ca.crt>", serverURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s/health returned %s; check the server URL", serverURL, resp.Status)
	}
	return nil
}

func singleCertLabel(certs []*x509.Certificate) string {
	if len(certs) == 0 {
		return "the supplied root CA"
	}
	return certs[0].Subject.String()
}

// waitForRegisteredRuntimes waits for the daemon to report at least one
// registered runtime, returning whatever it has when the deadline expires. A
// machine with no supported agent CLI legitimately registers nothing, so an
// empty result is reported to the caller rather than treated as a failure.
func waitForRegisteredRuntimes(profile string, timeout time.Duration) []string {
	deadline := time.Now().Add(timeout)
	var last []string
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		health := checkDaemonHealthOnPort(ctx, healthPortForProfile(profile))
		cancel()
		last = daemonRegisteredRuntimes(health)
		if len(last) > 0 || time.Now().After(deadline) {
			return last
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// daemonRegisteredRuntimes flattens the runtime ids reported by the daemon's
// /health response across all workspaces it registered with.
func daemonRegisteredRuntimes(health map[string]any) []string {
	workspaces, ok := health["workspaces"].([]any)
	if !ok {
		return nil
	}
	var runtimes []string
	for _, item := range workspaces {
		ws, ok := item.(map[string]any)
		if !ok {
			continue
		}
		list, ok := ws["runtimes"].([]any)
		if !ok {
			continue
		}
		for _, runtime := range list {
			if id, ok := runtime.(string); ok && id != "" {
				runtimes = append(runtimes, id)
			}
		}
	}
	return runtimes
}

func printConnectSummary(cmd *cobra.Command, serverURL, appURL string, browserTrusted bool, runtimeCount int) {
	profile := resolveProfile(cmd)
	if path, err := cli.CLIConfigPathForProfile(profile); err == nil {
		fmt.Fprintf(os.Stderr, "\nConfig saved: %s\n", path)
	}
	status := "daemon not started (--no-daemon)"
	if !flagBool(cmd, "no-daemon") {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		health := checkDaemonHealthOnPort(ctx, healthPortForProfile(profile))
		cancel()
		if daemonAlive(health) {
			pid := int64FromHealth(health, "pid")
			status = fmt.Sprintf("daemon %s", healthStatus(health))
			if pid > 0 {
				status = fmt.Sprintf("%s (pid %d)", status, pid)
			}
		}
	}
	fmt.Fprintf(os.Stderr, "\n✓ Connected to %s\n", serverURL)
	fmt.Fprintf(os.Stderr, "  app:      %s\n", appURL)
	fmt.Fprintf(os.Stderr, "  daemon:   %s\n", status)
	if runtimeCount > 0 {
		fmt.Fprintf(os.Stderr, "  runtimes: %d online\n", runtimeCount)
	}
	if browserTrusted {
		fmt.Fprintln(os.Stderr, "  browser:  root CA trusted for this Windows user")
	}
	fmt.Fprintln(os.Stderr, "\nNext: open the board to see this machine's runtime, or run 'daemon status'.")
}

func flagValueIfChanged(cmd *cobra.Command, name string) string {
	flag := cmd.Flags().Lookup(name)
	if flag == nil || !flag.Changed {
		return ""
	}
	return flag.Value.String()
}

func flagBool(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return false
	}
	value, err := cmd.Flags().GetBool(name)
	if err != nil {
		return false
	}
	return value
}

func promptLine(label string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("not a terminal")
	}
	fmt.Fprint(os.Stderr, label)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func healthStatus(health map[string]any) string {
	if status, ok := health["status"].(string); ok && status != "" {
		return status
	}
	return "unknown"
}

func int64FromHealth(health map[string]any, key string) int64 {
	switch value := health[key].(type) {
	case float64:
		return int64(value)
	case int:
		return int64(value)
	case int64:
		return value
	default:
		return 0
	}
}
