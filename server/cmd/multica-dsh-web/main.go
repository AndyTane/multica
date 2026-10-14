package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/multica-ai/multica/server/internal/dshweb"
)

// nodeUnavailable describes a Node.js runtime that could not be started, which
// is a different failure class than a cache or bridge failure.
const nodeUnavailable = "DSH web bridge requires Node.js 24 or newer"

func main() {
	code, err := run(os.Getenv("MULTICA_DSH_NODE_PATH"), os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

// run executes the embedded bridge with Node and returns the bridge's exit code.
// A non-nil error reports a failure of this launcher, never a bridge exit status.
func run(node string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if node == "" {
		node = "node"
	}
	dir, err := cacheDir()
	if err != nil {
		return 0, cacheFailure(err)
	}
	script, discard, err := materializeScript(dir, dshweb.Script, os.Rename)
	if err != nil {
		return 0, cacheFailure(err)
	}
	defer discard()
	cmd := exec.Command(node, bridgeArgs(script, args)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), nil
		}
		return 0, errors.New(nodeUnavailable)
	}
	return 0, nil
}

// bridgeArgs is the Node command line for the cached script. The script is the
// entry point and no Node options or `--` separator precede the user arguments,
// so the bridge's default process.argv.slice(2) receives exactly args.
func bridgeArgs(script string, args []string) []string {
	return append([]string{script}, args...)
}

func cacheFailure(err error) error {
	return fmt.Errorf("DSH web bridge could not prepare its script cache: %w", err)
}

// cacheDir is the user-private directory holding executable bridge scripts. A
// machine-shared location such as TempDir would expose code execution to other
// local users, so there is deliberately no fallback when it is unavailable.
func cacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "multica", "dsh-web")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// materializeScript returns a Node-runnable copy of script in dir. An existing
// cached file is reused only after its content hash is recomputed and matches,
// so a truncated or edited cache is rewritten. The write goes to a unique
// temporary file in the same directory and is published by rename, which keeps
// concurrent launches from observing partial content.
func materializeScript(dir, script string, rename func(oldpath, newpath string) error) (string, func(), error) {
	sum := sha256.Sum256([]byte(script))
	digest := hex.EncodeToString(sum[:])
	target := filepath.Join(dir, "bridge-"+digest[:16]+".mjs")
	if hasDigest(target, digest) {
		return target, func() {}, nil
	}
	file, err := os.CreateTemp(dir, "bridge-"+digest[:16]+"-*.mjs")
	if err != nil {
		return "", nil, err
	}
	temp := file.Name()
	if _, err := file.WriteString(script); err != nil {
		file.Close()
		os.Remove(temp)
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		os.Remove(temp)
		return "", nil, err
	}
	if err := rename(temp, target); err != nil {
		// Windows refuses to replace a cached script that another Node process is
		// executing. Reuse the target when it already holds this content, and
		// otherwise run this call's own file instead of failing the launch.
		if hasDigest(target, digest) {
			os.Remove(temp)
			return target, func() {}, nil
		}
		return temp, func() { os.Remove(temp) }, nil
	}
	return target, func() {}, nil
}

func hasDigest(path, digest string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]) == digest
}
