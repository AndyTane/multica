package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/multica-ai/multica/server/internal/dshweb"
)

// TestMain lets the test binary impersonate Node when a test needs a chosen
// child exit code: run() launches os.Executable() as the "node" program.
func TestMain(m *testing.M) {
	if value, ok := os.LookupEnv("MULTICA_DSH_WEB_TEST_FAKE_NODE_EXIT"); ok {
		code, err := strconv.Atoi(value)
		if err != nil {
			fmt.Fprintf(os.Stderr, "MULTICA_DSH_WEB_TEST_FAKE_NODE_EXIT=%q is not an exit code\n", value)
			os.Exit(2)
		}
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestBridgeArgsKeepUserArgumentsAfterScript(t *testing.T) {
	script := filepath.Join("cache dir", "bridge-0123456789abcdef.mjs")
	args := bridgeArgs(script, []string{"--stdio", "--profile", "multica"})
	if want := []string{script, "--stdio", "--profile", "multica"}; !slices.Equal(args, want) {
		t.Fatalf("bridgeArgs = %q, want %q", args, want)
	}
	if slices.Contains(args, "--input-type=module") || slices.Contains(args, "--") {
		t.Fatalf("bridgeArgs = %q, want the script as the only entry point", args)
	}
}

func TestSupportsImportMetaMain(t *testing.T) {
	cases := map[string]bool{
		"v22.11.0": false,
		"v24.0.0":  false,
		"v24.1.0":  false,
		"v24.2.0":  true,
		"v24.21.0": true,
		"v25.0.0":  true,
		"v26.1.0":  true,
		"v24":      false,
		"garbage":  false,
	}
	for version, want := range cases {
		if got := supportsImportMetaMain(version); got != want {
			t.Errorf("supportsImportMetaMain(%q) = %v, want %v", version, got, want)
		}
	}
}

func TestMaterializeScriptWritesHashedFile(t *testing.T) {
	dir := t.TempDir()
	path, discard, err := materializeScript(dir, dshweb.Script, os.Rename)
	if err != nil {
		t.Fatalf("materializeScript: %v", err)
	}
	if want := cachedPath(dir, dshweb.Script); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	assertContent(t, path, dshweb.Script)
	if entries := dirEntries(t, dir); !slices.Equal(entries, []string{filepath.Base(path)}) {
		t.Fatalf("cache dir = %q, want only the hashed script", entries)
	}
	discard()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("discard removed the stable cache file: %v", err)
	}
}

func TestMaterializeScriptReusesVerifiedCache(t *testing.T) {
	dir := t.TempDir()
	target := cachedPath(dir, dshweb.Script)
	path, discard, err := materializeScript(dir, dshweb.Script, os.Rename)
	if err != nil {
		t.Fatalf("first materializeScript: %v", err)
	}
	discard()
	before, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat cached script: %v", err)
	}
	renames := 0
	path, discard, err = materializeScript(dir, dshweb.Script, func(oldpath, newpath string) error {
		renames++
		return errors.New("the cached script is locked")
	})
	if err != nil {
		t.Fatalf("second materializeScript: %v", err)
	}
	discard()
	if renames != 0 {
		t.Fatalf("a verified cache must not be rewritten: %d rename attempts", renames)
	}
	if path != target {
		t.Fatalf("path = %q, want the cached %q", path, target)
	}
	after, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat cached script: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("the cached script was replaced instead of reused")
	}
	assertContent(t, target, dshweb.Script)
	if entries := dirEntries(t, dir); !slices.Equal(entries, []string{filepath.Base(target)}) {
		t.Fatalf("cache dir = %q, want only the cached script", entries)
	}
}

func TestMaterializeScriptRewritesDamagedCache(t *testing.T) {
	damaged := map[string]string{
		"tampered":  "// not the bridge\n",
		"truncated": dshweb.Script[:len(dshweb.Script)/2],
		"empty":     "",
	}
	for name, content := range damaged {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			target := cachedPath(dir, dshweb.Script)
			if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
				t.Fatalf("damage cached script: %v", err)
			}
			path, discard, err := materializeScript(dir, dshweb.Script, os.Rename)
			if err != nil {
				t.Fatalf("materializeScript: %v", err)
			}
			if path != target {
				t.Fatalf("path = %q, want the cached %q", path, target)
			}
			assertContent(t, path, dshweb.Script)
			discard()
			if entries := dirEntries(t, dir); !slices.Equal(entries, []string{filepath.Base(target)}) {
				t.Fatalf("cache dir = %q, want only the rewritten script", entries)
			}
		})
	}
}

// Concurrent first launches all get a runnable script. Windows can transiently
// deny the replace while a competing rename is in flight, so a launcher may fall
// back to its own file; the cache still converges on the hashed file.
func TestMaterializeScriptConcurrentCreation(t *testing.T) {
	dir := t.TempDir()
	target := cachedPath(dir, dshweb.Script)
	const launchers = 8
	paths := make([]string, launchers)
	discards := make([]func(), launchers)
	errs := make([]error, launchers)
	var wg sync.WaitGroup
	for i := range launchers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			paths[i], discards[i], errs[i] = materializeScript(dir, dshweb.Script, os.Rename)
		}()
	}
	wg.Wait()
	// Read the results only after every launcher stopped touching the directory:
	// a concurrent rename can transiently lock the file being published.
	for i, err := range errs {
		if err != nil {
			t.Fatalf("launcher %d: %v", i, err)
		}
		if paths[i] == "" {
			t.Fatalf("launcher %d returned no path", i)
		}
		assertContent(t, paths[i], dshweb.Script)
	}
	for _, discard := range discards {
		if discard != nil {
			discard()
		}
	}
	path, discard, err := materializeScript(dir, dshweb.Script, os.Rename)
	if err != nil {
		t.Fatalf("materializeScript after concurrent launches: %v", err)
	}
	discard()
	if path != target {
		t.Fatalf("path = %q, want the converged %q", path, target)
	}
	assertContent(t, target, dshweb.Script)
	if entries := dirEntries(t, dir); !slices.Equal(entries, []string{filepath.Base(target)}) {
		t.Fatalf("cache dir = %q, want only the hashed script", entries)
	}
}

func TestMaterializeScriptReusesTargetWhenReplacementIsDenied(t *testing.T) {
	dir := t.TempDir()
	target := cachedPath(dir, dshweb.Script)
	calls := 0
	path, discard, err := materializeScript(dir, dshweb.Script, func(oldpath, newpath string) error {
		calls++
		// Another launch publishes the same content while a running Node process
		// keeps the target locked, so the replacement is refused.
		if err := os.WriteFile(newpath, []byte(dshweb.Script), 0o600); err != nil {
			t.Fatalf("publish target: %v", err)
		}
		return errors.New("access is denied")
	})
	if err != nil {
		t.Fatalf("materializeScript: %v", err)
	}
	discard()
	if calls != 1 {
		t.Fatalf("rename attempts = %d, want 1", calls)
	}
	if path != target {
		t.Fatalf("path = %q, want the matching cached %q", path, target)
	}
	assertContent(t, target, dshweb.Script)
	if entries := dirEntries(t, dir); !slices.Equal(entries, []string{filepath.Base(target)}) {
		t.Fatalf("cache dir = %q, want the temporary file removed", entries)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("discard removed the reused cached script: %v", err)
	}
}

func TestMaterializeScriptFallsBackToOwnFileWhenReplacementIsDenied(t *testing.T) {
	dir := t.TempDir()
	target := cachedPath(dir, dshweb.Script)
	const stale = "// stale bridge script\n"
	if err := os.WriteFile(target, []byte(stale), 0o600); err != nil {
		t.Fatalf("write stale cache: %v", err)
	}
	denied := func(oldpath, newpath string) error { return errors.New("access is denied") }
	first, discardFirst, err := materializeScript(dir, dshweb.Script, denied)
	if err != nil {
		t.Fatalf("first materializeScript: %v", err)
	}
	second, discardSecond, err := materializeScript(dir, dshweb.Script, denied)
	if err != nil {
		t.Fatalf("second materializeScript: %v", err)
	}
	prefix := "bridge-" + digestOf(dshweb.Script)[:16] + "-"
	for _, path := range []string{first, second} {
		if path == target {
			t.Fatalf("path = %q, want an exclusive fallback file", path)
		}
		if filepath.Dir(path) != dir || !strings.HasPrefix(filepath.Base(path), prefix) || !strings.HasSuffix(path, ".mjs") {
			t.Fatalf("fallback path = %q, want %q in %q", path, prefix+"<unique>.mjs", dir)
		}
		assertContent(t, path, dshweb.Script)
	}
	if first == second {
		t.Fatalf("both fallbacks used %q; the fallback name needs a per-launch unique part", first)
	}
	assertContent(t, target, stale)
	discardFirst()
	if _, err := os.Stat(first); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("discard kept the fallback %q: %v", first, err)
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatalf("discard of the first fallback removed the second: %v", err)
	}
	discardSecond()
	if entries := dirEntries(t, dir); !slices.Equal(entries, []string{filepath.Base(target)}) {
		t.Fatalf("cache dir = %q, want only the unusable cached script", entries)
	}
	assertContent(t, target, stale)
}

func TestRunReportsCacheFailureInsteadOfNodeFailure(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not a directory")
	if err := os.WriteFile(blocker, []byte("blocked"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	setCacheBase(t, blocker)
	missingNode := filepath.Join(t.TempDir(), "missing-node.exe")
	code, err := run(missingNode, []string{"--version"}, strings.NewReader(""), io.Discard, io.Discard)
	if err == nil {
		t.Fatal("run succeeded with an unusable cache location")
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 for a launcher failure", code)
	}
	if !strings.Contains(err.Error(), "script cache") {
		t.Fatalf("error = %v, want a script cache failure", err)
	}
	if strings.Contains(err.Error(), nodeUnavailable) {
		t.Fatalf("cache failure reused the Node runtime message: %v", err)
	}
}

// TestRunRejectsMissingUserCacheDirectory removes the platform user cache
// variable: the launcher must fail instead of falling back to a machine-shared
// directory such as os.TempDir().
func TestRunRejectsMissingUserCacheDirectory(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows reports a missing user cache directory through %LocalAppData%")
	}
	tempBase := t.TempDir()
	t.Setenv("TMP", tempBase)
	t.Setenv("TEMP", tempBase)
	setCacheBase(t, "")
	missingNode := filepath.Join(t.TempDir(), "missing-node.exe")
	code, err := run(missingNode, []string{"--version"}, strings.NewReader(""), io.Discard, io.Discard)
	if err == nil {
		t.Fatal("run succeeded without a user cache directory")
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 for a launcher failure", code)
	}
	if !strings.Contains(err.Error(), "script cache") {
		t.Fatalf("error = %v, want a script cache failure", err)
	}
	if strings.Contains(err.Error(), nodeUnavailable) {
		t.Fatalf("cache failure reused the Node runtime message: %v", err)
	}
	shared := filepath.Join(tempBase, "multica", "dsh-web")
	entries, err := os.ReadDir(shared)
	if err == nil && len(entries) > 0 {
		t.Fatalf("launcher wrote executable scripts to the shared temp directory %q: %q", shared, entries)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read %q: %v", shared, err)
	}
}

func TestRunReportsUnavailableNodeRuntime(t *testing.T) {
	cacheBase := setCacheBase(t, t.TempDir())
	missingNode := filepath.Join(t.TempDir(), "missing-node.exe")
	code, err := run(missingNode, []string{"--version"}, strings.NewReader(""), io.Discard, io.Discard)
	if err == nil {
		t.Fatal("run succeeded without a Node runtime")
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 for a launcher failure", code)
	}
	if err.Error() != nodeUnavailable {
		t.Fatalf("error = %q, want %q", err, nodeUnavailable)
	}
	if _, err := os.Stat(cachedPath(filepath.Join(cacheBase, "multica", "dsh-web"), dshweb.Script)); err != nil {
		t.Fatalf("cache was not prepared before the Node attempt: %v", err)
	}
}

func TestRunPassesChildExitCodeThrough(t *testing.T) {
	setCacheBase(t, t.TempDir())
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	var stdout, stderr bytes.Buffer
	for _, want := range []int{0, 3, 7, 42} {
		t.Setenv("MULTICA_DSH_WEB_TEST_FAKE_NODE_EXIT", strconv.Itoa(want))
		stdout.Reset()
		stderr.Reset()
		code, err := run(self, []string{"--version"}, strings.NewReader(""), &stdout, &stderr)
		if err != nil {
			t.Fatalf("exit %d: run: %v", want, err)
		}
		if code != want {
			t.Fatalf("exit code = %d, want %d", code, want)
		}
	}
}

func TestRunPassesBridgeFailureAndExitCodeThrough(t *testing.T) {
	node := testNode(t)
	setCacheBase(t, t.TempDir())
	t.Setenv("MULTICA_DSH_WEB_CONFIG", filepath.Join(t.TempDir(), "missing-dsh-web.json"))
	var stdout, stderr bytes.Buffer
	code, err := run(node, []string{"--stdio"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("run: %v (stderr %q)", err, stderr.String())
	}
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr %q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "web_bridge_config_unavailable") {
		t.Fatalf("stderr = %q, want the bridge config failure", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunPrintsVersionFromPathWithSpacesAndChinese(t *testing.T) {
	node := testNode(t)
	cacheBase := setCacheBase(t, filepath.Join(t.TempDir(), "用户 缓存"))
	assertVersion(t, node)
	if _, err := os.Stat(cachedPath(filepath.Join(cacheBase, "multica", "dsh-web"), dshweb.Script)); err != nil {
		t.Fatalf("bridge script was not cached under the user cache directory: %v", err)
	}
}

// TestRunPrintsVersionThroughDirectoryJunction is the regression gate for the
// entry guard. Node resolves the junction for the main module while argv[1]
// keeps the junction path, so the previous guard compared a resolved
// import.meta.url with an unresolved argv[1], never ran main() and exited 0 with
// no output. The junction is asserted twice: Windows reports it as a junction,
// and Node reports the resolved target.
func TestRunPrintsVersionThroughDirectoryJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("directory junctions are Windows-only")
	}
	node := testNode(t)
	target := filepath.Join(t.TempDir(), "真实 缓存 目标")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("create junction target: %v", err)
	}
	link := filepath.Join(t.TempDir(), "链接's 目录")
	makeJunction(t, link, target)
	t.Cleanup(func() { removeJunction(t, link) })
	assertJunction(t, link)
	assertNodeResolvesJunction(t, node, link, target)

	setCacheBase(t, link)
	assertVersion(t, node)
	cached := cachedPath(filepath.Join(target, "multica", "dsh-web"), dshweb.Script)
	if _, err := os.Stat(cached); err != nil {
		t.Fatalf("bridge script was not cached through the junction: %v", err)
	}

	removeJunction(t, link)
	if _, err := os.Stat(cached); err != nil {
		t.Fatalf("removing the junction damaged the redirected target: %v", err)
	}
}

// TestBridgeFailsWithoutImportMetaMain covers Node.js 24.0 and 24.1: they load
// the script but have no import.meta.main, so the bridge must fail loudly
// instead of exiting 0 without running main(). Deleting the property reproduces
// those runtimes.
func TestBridgeFailsWithoutImportMetaMain(t *testing.T) {
	node := testNode(t)
	script := filepath.Join(t.TempDir(), "bridge-without-import-meta-main.mjs")
	if err := os.WriteFile(script, []byte("delete import.meta.main;\n"+dshweb.Script), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(node, script, "--version")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = strings.NewReader(""), &stdout, &stderr
	var exit *exec.ExitError
	if err := cmd.Run(); !errors.As(err, &exit) {
		t.Fatalf("run: %v, want a non-zero exit (stdout %q, stderr %q)", err, stdout.String(), stderr.String())
	}
	if exit.ExitCode() == 0 {
		t.Fatalf("exit code = 0, want a non-zero exit (stdout %q)", stdout.String())
	}
	if got, want := stderr.String(), nodeUnavailable+"\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

// testNode returns an explicit Node.js 24.2+ path. Node is the bridge runtime,
// not an agent CLI: no agent command is resolved or executed here.
func testNode(t *testing.T) string {
	t.Helper()
	node := os.Getenv("MULTICA_DSH_NODE_PATH")
	if node == "" {
		found, err := exec.LookPath("node")
		if err != nil {
			t.Skipf("node is not on PATH; the bridge requires Node.js 24.2 or newer")
		}
		node = found
	}
	output, err := exec.Command(node, "--version").Output()
	if err != nil {
		t.Skipf("could not run %q: %v; the bridge requires Node.js 24.2 or newer", node, err)
	}
	version := strings.TrimSpace(string(output))
	if !supportsImportMetaMain(version) {
		t.Skipf("node %s has no import.meta.main; the bridge requires Node.js 24.2 or newer", version)
	}
	return node
}

// supportsImportMetaMain reports whether version, for example "v24.21.0", is
// Node.js 24.2 or newer: the first release with import.meta.main.
func supportsImportMetaMain(version string) bool {
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	if len(parts) < 2 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	return major > 24 || major == 24 && minor >= 2
}

func assertVersion(t *testing.T, node string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code, err := run(node, []string{"--version"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("run --version: %v (stderr %q)", err, stderr.String())
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr %q)", code, stderr.String())
	}
	if got, want := stdout.String(), "multica-dsh-web 1.0.3\n"; got != want {
		t.Fatalf("stdout = %q, want %q (stderr %q)", got, want, stderr.String())
	}
}

func cachedPath(dir, script string) string {
	return filepath.Join(dir, "bridge-"+digestOf(script)[:16]+".mjs")
}

func digestOf(script string) string {
	sum := sha256.Sum256([]byte(script))
	return hex.EncodeToString(sum[:])
}

// setCacheBase points the platform user cache directory at base so tests never
// touch the real one.
func setCacheBase(t *testing.T, base string) string {
	t.Helper()
	t.Setenv("LocalAppData", base)
	t.Setenv("XDG_CACHE_HOME", base)
	t.Setenv("HOME", base)
	if runtime.GOOS == "darwin" {
		return filepath.Join(base, "Library", "Caches")
	}
	return base
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	if string(content) != want {
		t.Fatalf("%q holds %d bytes, want %d bytes of the bridge script", path, len(content), len(want))
	}
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %q: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	return names
}

// makeJunction creates a directory junction, which needs no administrator rights
// unlike a symbolic link.
func makeJunction(t *testing.T, link, target string) {
	t.Helper()
	command := fmt.Sprintf("New-Item -ItemType Junction -Path '%s' -Target '%s' | Out-Null",
		psQuote(link), psQuote(target))
	if output, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", command).CombinedOutput(); err != nil {
		t.Fatalf("create junction %q -> %q: %v: %s", link, target, err, output)
	}
}

// assertJunction checks through Windows itself that link is a directory
// junction. Go reports a junction as an irregular reparse point when it cannot
// resolve it, so this test does not rely on Go's own link detection.
func assertJunction(t *testing.T, link string) {
	t.Helper()
	command := fmt.Sprintf("(Get-Item -LiteralPath '%s' -Force).LinkType", psQuote(link))
	output, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", command).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect junction %q: %v: %s", link, err, output)
	}
	if linkType := strings.TrimSpace(string(output)); !strings.EqualFold(linkType, "Junction") {
		t.Fatalf("%q LinkType = %q, want Junction; the junction case would not be exercised", link, linkType)
	}
}

// assertNodeResolvesJunction proves Node resolves the junction for the main
// module while argv[1] keeps the junction path: the asymmetry behind the
// replaced entry guard.
func assertNodeResolvesJunction(t *testing.T, node, link, target string) {
	t.Helper()
	probe := filepath.Join(target, "resolve-probe.mjs")
	const source = "process.stdout.write(JSON.stringify({ main: import.meta.main, url: import.meta.url, argv1: process.argv[1] }) + '\\n');"
	if err := os.WriteFile(probe, []byte(source), 0o600); err != nil {
		t.Fatalf("write junction probe: %v", err)
	}
	viaLink := filepath.Join(link, "resolve-probe.mjs")
	output, err := exec.Command(node, viaLink).Output()
	if err != nil {
		t.Fatalf("run junction probe: %v", err)
	}
	var payload struct {
		Main bool   `json:"main"`
		URL  string `json:"url"`
		Argv string `json:"argv1"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("parse junction probe %q: %v", output, err)
	}
	if !payload.Main {
		t.Fatalf("import.meta.main = false for %q; the entry guard could not run", viaLink)
	}
	parsed, err := url.Parse(payload.URL)
	if err != nil {
		t.Fatalf("parse %q: %v", payload.URL, err)
	}
	resolved := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(parsed.Path, "/")))
	if !strings.EqualFold(resolved, filepath.Clean(probe)) {
		t.Fatalf("node resolved %q to %q, want %q", viaLink, resolved, probe)
	}
	if !strings.EqualFold(filepath.Clean(payload.Argv), filepath.Clean(viaLink)) {
		t.Fatalf("process.argv[1] = %q, want the unresolved %q", payload.Argv, viaLink)
	}
}

// removeJunction deletes the junction only: os.Remove drops the reparse point
// without recursing into the redirected target.
func removeJunction(t *testing.T, link string) {
	t.Helper()
	if _, err := os.Lstat(link); errors.Is(err, os.ErrNotExist) {
		return
	}
	if err := os.Remove(link); err != nil {
		t.Errorf("remove junction %q: %v", link, err)
	}
}

func psQuote(path string) string {
	return strings.ReplaceAll(path, "'", "''")
}
