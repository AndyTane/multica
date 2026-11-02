//go:build windows

// Command u-workers-setup installs the U Workers CLI for the current user.
//
// It is a single Windows executable: the CLI binaries are embedded, so running
// it with no arguments installs u-workers.exe and multica.exe under
// %LOCALAPPDATA%\U Workers\bin, adds that directory to the per-user PATH, and
// registers the installation in Add/Remove Programs. Everything it writes is
// per user, so it never needs administrator rights.
//
// Supplying a connect flag (--server-url, --root-ca, --token, --app-url,
// --skip-browser-trust) turns the install into the one-command flow: after a
// successful install the installed CLI runs its connect command with exactly
// those flags, in the current console.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// version is the installer build version. scripts/build-windows-installer.ps1
// stamps it with -ldflags "-X main.version=..."; a plain go build reports dev.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is main without the process exit, so the whole flow reads in one place.
func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if opts.printVersion {
		fmt.Fprintf(stdout, "u-workers-setup %s\n", version)
		return 0
	}

	installDir := opts.installDir
	if installDir == "" {
		installDir, err = defaultInstallDir(os.Getenv("LOCALAPPDATA"), os.Getenv("USERPROFILE"))
		if err != nil {
			return fail(stderr, err)
		}
	}
	installDir, err = filepath.Abs(installDir)
	if err != nil {
		return fail(stderr, fmt.Errorf("resolve install directory %q: %w", installDir, err))
	}

	inst := installer{
		payload: payloadFS,
		env:     newUserEnvironment(),
		out:     stdout,
		errOut:  stderr,
		silent:  opts.silent,
	}

	if opts.uninstall {
		removed, err := inst.uninstall(installDir)
		if err != nil {
			return fail(stderr, err)
		}
		if !removed {
			fmt.Fprintf(stdout, "U Workers CLI is not installed in %s.\n", installDir)
		}
		return 0
	}

	if err := inst.install(installDir, !opts.noPath); err != nil {
		return fail(stderr, err)
	}

	exe := filepath.Join(installDir, primaryBinaryName)
	switch {
	case !opts.connect.any():
		if !opts.silent {
			fmt.Fprintf(stdout, "Next: %s\n", commandLine(exe, "connect"))
		}
	case opts.noConnect:
		if !opts.silent {
			fmt.Fprintf(stdout, "Skipped the connect step because --no-connect was given.\n")
		}
	default:
		return runConnect(exe, opts.connect, os.Stdin, os.Stdout, os.Stderr)
	}
	return 0
}

// fail prints an actionable one-liner and returns the process exit code.
func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "u-workers-setup: %v\n", err)
	return 1
}

// options is the command line after parsing.
type options struct {
	installDir   string
	uninstall    bool
	noPath       bool
	noConnect    bool
	silent       bool
	printVersion bool
	connect      connectFlags
}

// connectFlags carries the connect pass-through flags together with whether the
// operator supplied them: only supplied flags are forwarded to the CLI.
type connectFlags struct {
	serverURL           string
	serverURLSet        bool
	rootCA              string
	rootCASet           bool
	token               string
	tokenSet            bool
	appURL              string
	appURLSet           bool
	skipBrowserTrust    bool
	skipBrowserTrustSet bool
}

// any reports whether the operator supplied at least one connect flag.
func (c connectFlags) any() bool {
	return c.serverURLSet || c.rootCASet || c.tokenSet || c.appURLSet || c.skipBrowserTrustSet
}

// args is the argument list handed to the installed CLI: its connect command
// plus exactly the flags the operator supplied. An explicit
// --skip-browser-trust=false means the same as leaving the flag out, so only a
// true value is forwarded.
func (c connectFlags) args() []string {
	args := []string{"connect"}
	if c.serverURLSet {
		args = append(args, "--server-url", c.serverURL)
	}
	if c.rootCASet {
		args = append(args, "--root-ca", c.rootCA)
	}
	if c.tokenSet {
		args = append(args, "--token", c.token)
	}
	if c.appURLSet {
		args = append(args, "--app-url", c.appURL)
	}
	if c.skipBrowserTrustSet && c.skipBrowserTrust {
		args = append(args, "--skip-browser-trust")
	}
	return args
}

// parseOptions reads the flags. A connect flag counts as supplied only when it
// appears on the command line, so an unset flag is never forwarded.
func parseOptions(args []string, stderr io.Writer) (options, error) {
	set := flag.NewFlagSet("u-workers-setup", flag.ContinueOnError)
	set.SetOutput(stderr)
	set.Usage = func() {
		fmt.Fprint(stderr, usage)
	}

	var (
		opts                             options
		serverURL, rootCA, token, appURL string
		skipBrowserTrust                 bool
	)
	set.StringVar(&opts.installDir, "install-dir", "", "install directory (default %LOCALAPPDATA%\\U Workers\\bin)")
	set.BoolVar(&opts.uninstall, "uninstall", false, "remove the installation in --install-dir")
	set.BoolVar(&opts.noPath, "no-path", false, "leave the per-user PATH unchanged")
	set.BoolVar(&opts.noConnect, "no-connect", false, "never run the CLI connect command")
	set.BoolVar(&opts.silent, "silent", false, "print errors only")
	set.BoolVar(&opts.printVersion, "version", false, "print the installer version")
	set.StringVar(&serverURL, "server-url", "", "forwarded to the CLI connect command")
	set.StringVar(&rootCA, "root-ca", "", "forwarded to the CLI connect command")
	set.StringVar(&token, "token", "", "forwarded to the CLI connect command")
	set.StringVar(&appURL, "app-url", "", "forwarded to the CLI connect command")
	set.BoolVar(&skipBrowserTrust, "skip-browser-trust", false, "forwarded to the CLI connect command")

	if err := set.Parse(args); err != nil {
		return options{}, err
	}
	if set.NArg() > 0 {
		// The flag package reports unknown flags itself; a positional argument is
		// ours to explain, and it is the only thing the operator has to see.
		err := fmt.Errorf("unexpected argument %q", set.Arg(0))
		fmt.Fprintf(stderr, "u-workers-setup: %v\n", err)
		set.Usage()
		return options{}, err
	}
	set.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "server-url":
			opts.connect.serverURL, opts.connect.serverURLSet = serverURL, true
		case "root-ca":
			opts.connect.rootCA, opts.connect.rootCASet = rootCA, true
		case "token":
			opts.connect.token, opts.connect.tokenSet = token, true
		case "app-url":
			opts.connect.appURL, opts.connect.appURLSet = appURL, true
		case "skip-browser-trust":
			opts.connect.skipBrowserTrust, opts.connect.skipBrowserTrustSet = skipBrowserTrust, true
		}
	})
	return opts, nil
}

// usage is the help text an operator sees after a flag mistake or with -h.
const usage = `u-workers-setup installs the U Workers CLI for the current user.

Usage:
  u-workers-setup [flags]

Flags:
  --install-dir <dir>    Install into <dir> (default %LOCALAPPDATA%\U Workers\bin)
  --no-path              Leave the per-user PATH unchanged
  --no-connect           Install only; never run the CLI connect command
  --silent               Print errors only
  --uninstall            Remove the installation in --install-dir
  --version              Print the installer version and exit

Connect flags, handed to "<install-dir>\u-workers.exe" connect when supplied:
  --server-url <url>  --root-ca <path>  --token <token>  --app-url <url>  --skip-browser-trust

Running with no flags installs and prints the command that connects next.
`

// runConnect hands the current console to the installed CLI and reports its exit
// code, so the one-command flow behaves like the CLI invocation it stands in for.
func runConnect(exe string, flags connectFlags, stdin io.Reader, stdout, stderr io.Writer) int {
	cmd := exec.Command(exe, flags.args()...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
		return 1
	}
	fmt.Fprintf(stderr, "u-workers-setup: run %s: %v\n", exe, err)
	return 1
}

// commandLine renders a command the operator can paste into a Windows shell.
// The default install directory contains a space, so paths are quoted when they
// need it.
func commandLine(args ...string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		if strings.ContainsAny(arg, " \t") {
			quoted[i] = "\"" + arg + "\""
			continue
		}
		quoted[i] = arg
	}
	return strings.Join(quoted, " ")
}
