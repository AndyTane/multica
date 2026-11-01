//go:build windows

package main

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

// testPayload is a synthetic embedded payload: install and uninstall must be
// testable without the real CLI binaries, which the build script stages.
func testPayload() fstest.MapFS {
	return fstest.MapFS{
		"payload/README.md":     {Data: []byte("payload")},
		"payload/u-workers.exe": {Data: []byte("u-workers-binary")},
		"payload/multica.exe":   {Data: []byte("u-workers-binary")},
	}
}

// fakeEnvironment plays the role of HKCU: it records what the installer would
// have written instead of touching the real user environment.
type fakeEnvironment struct {
	path            pathValue
	pathWrites      []pathValue
	uninstall       *uninstallEntry
	notifications   int
	failSetPath     error
	failEntry       error
	failRemoveEntry error
}

func (f *fakeEnvironment) userPath() (pathValue, error) {
	return f.path, nil
}

func (f *fakeEnvironment) setUserPath(value pathValue) error {
	if f.failSetPath != nil {
		return f.failSetPath
	}
	f.path = value
	f.pathWrites = append(f.pathWrites, value)
	return nil
}

func (f *fakeEnvironment) writeUninstallEntry(entry uninstallEntry) error {
	if f.failEntry != nil {
		return f.failEntry
	}
	f.uninstall = &entry
	return nil
}

func (f *fakeEnvironment) removeUninstallEntry() error {
	if f.failRemoveEntry != nil {
		return f.failRemoveEntry
	}
	f.uninstall = nil
	return nil
}

func (f *fakeEnvironment) notifyEnvironmentChange() error {
	f.notifications++
	return nil
}

func TestNormalizePathEntry(t *testing.T) {
	cases := []struct {
		name  string
		entry string
		want  string
	}{
		{"plain", `C:\Users\me\AppData\Local\U Workers\bin`, `c:\users\me\appdata\local\u workers\bin`},
		{"trailing separator", `C:\Users\me\bin\`, `c:\users\me\bin`},
		{"trailing forward slash", `C:\Users\me\bin/`, `c:\users\me\bin`},
		{"surrounding spaces", "  C:\\Users\\me\\bin \t", `c:\users\me\bin`},
		{"drive root keeps its separator", "C:\\", `c:\`},
		{"empty", "", ""},
		{"spaces only", "   ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizePathEntry(tc.entry); got != tc.want {
				t.Fatalf("normalizePathEntry(%q) = %q, want %q", tc.entry, got, tc.want)
			}
		})
	}
}

func TestAppendPathEntry(t *testing.T) {
	const binDir = `C:\Users\me\AppData\Local\U Workers\bin`
	cases := []struct {
		name        string
		raw         string
		dir         string
		want        string
		wantChanged bool
	}{
		{
			name:        "appends to an existing PATH",
			raw:         `C:\Windows;C:\Tools`,
			dir:         binDir,
			want:        `C:\Windows;C:\Tools;C:\Users\me\AppData\Local\U Workers\bin`,
			wantChanged: true,
		},
		{
			name:        "empty PATH becomes the directory",
			raw:         "",
			dir:         binDir,
			want:        binDir,
			wantChanged: true,
		},
		{
			name:        "trailing separator is not doubled",
			raw:         `C:\Windows;`,
			dir:         binDir,
			want:        `C:\Windows;C:\Users\me\AppData\Local\U Workers\bin`,
			wantChanged: true,
		},
		{
			name:        "already present in another case and with a trailing separator",
			raw:         `C:\Windows;c:\users\me\appdata\local\u workers\bin\`,
			dir:         binDir,
			want:        `C:\Windows;c:\users\me\appdata\local\u workers\bin\`,
			wantChanged: false,
		},
		{
			name:        "blank PATH becomes the directory",
			raw:         "   ",
			dir:         binDir,
			want:        binDir,
			wantChanged: true,
		},
		{
			name:        "surrounding spaces around the directory are trimmed",
			raw:         `C:\Windows`,
			dir:         " " + binDir + " ",
			want:        `C:\Windows;C:\Users\me\AppData\Local\U Workers\bin`,
			wantChanged: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := appendPathEntry(tc.raw, tc.dir)
			if got != tc.want || changed != tc.wantChanged {
				t.Fatalf("appendPathEntry(%q, %q) = (%q, %v), want (%q, %v)", tc.raw, tc.dir, got, changed, tc.want, tc.wantChanged)
			}
		})
	}
}

func TestRemovePathEntry(t *testing.T) {
	const binDir = `C:\Users\me\AppData\Local\U Workers\bin`
	cases := []struct {
		name        string
		raw         string
		dir         string
		want        string
		wantChanged bool
	}{
		{
			name:        "removes the matching entry and keeps the rest",
			raw:         `C:\Windows;C:\Users\me\AppData\Local\U Workers\bin;C:\Tools`,
			dir:         binDir,
			want:        `C:\Windows;C:\Tools`,
			wantChanged: true,
		},
		{
			name:        "matches case and trailing separator",
			raw:         `C:\Windows;c:\users\me\appdata\local\u workers\bin\`,
			dir:         binDir,
			want:        `C:\Windows`,
			wantChanged: true,
		},
		{
			name:        "leaves a similarly named directory alone",
			raw:         `C:\Windows;C:\Users\me\AppData\Local\U Workers\bin2`,
			dir:         binDir,
			want:        `C:\Windows;C:\Users\me\AppData\Local\U Workers\bin2`,
			wantChanged: false,
		},
		{
			name:        "absent entry leaves the value unchanged",
			raw:         `C:\Windows;C:\Tools`,
			dir:         binDir,
			want:        `C:\Windows;C:\Tools`,
			wantChanged: false,
		},
		{
			name:        "empty PATH is left alone",
			raw:         "",
			dir:         binDir,
			want:        "",
			wantChanged: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := removePathEntry(tc.raw, tc.dir)
			if got != tc.want || changed != tc.wantChanged {
				t.Fatalf("removePathEntry(%q, %q) = (%q, %v), want (%q, %v)", tc.raw, tc.dir, got, changed, tc.want, tc.wantChanged)
			}
		})
	}
}

func TestDefaultInstallDir(t *testing.T) {
	cases := []struct {
		name         string
		localAppData string
		userProfile  string
		want         string
		wantErr      bool
	}{
		{
			name:         "LOCALAPPDATA wins",
			localAppData: `C:\Users\me\AppData\Local`,
			userProfile:  `C:\Users\me`,
			want:         `C:\Users\me\AppData\Local\U Workers\bin`,
		},
		{
			name:        "USERPROFILE is the documented fallback",
			userProfile: `C:\Users\me`,
			want:        `C:\Users\me\AppData\Local\U Workers\bin`,
		},
		{
			name:         "a blank LOCALAPPDATA falls back",
			localAppData: "   ",
			userProfile:  `C:\Users\me`,
			want:         `C:\Users\me\AppData\Local\U Workers\bin`,
		},
		{
			name:    "neither variable resolves",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := defaultInstallDir(tc.localAppData, tc.userProfile)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("defaultInstallDir(%q, %q) = %q, want an error", tc.localAppData, tc.userProfile, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("defaultInstallDir(%q, %q): %v", tc.localAppData, tc.userProfile, err)
			}
			if got != tc.want {
				t.Fatalf("defaultInstallDir(%q, %q) = %q, want %q", tc.localAppData, tc.userProfile, got, tc.want)
			}
		})
	}
}

func TestReadPayload(t *testing.T) {
	t.Run("reads both binaries", func(t *testing.T) {
		binaries, err := readPayload(testPayload())
		if err != nil {
			t.Fatalf("readPayload: %v", err)
		}
		if string(binaries.primary) != "u-workers-binary" || string(binaries.legacy) != "u-workers-binary" {
			t.Fatalf("readPayload returned %q and %q", binaries.primary, binaries.legacy)
		}
	})

	t.Run("a plain build has no payload", func(t *testing.T) {
		payload := fstest.MapFS{"payload/README.md": {Data: []byte("payload")}}
		_, err := readPayload(payload)
		if !errors.Is(err, errPayloadMissing) {
			t.Fatalf("readPayload error = %v, want %v", err, errPayloadMissing)
		}
		if got := err.Error(); got != "installer payload missing; build it with scripts/build-windows-installer.ps1" {
			t.Fatalf("readPayload error = %q, want the build instruction", got)
		}
	})

	t.Run("one missing binary is incomplete", func(t *testing.T) {
		payload := testPayload()
		delete(payload, "payload/multica.exe")
		_, err := readPayload(payload)
		if err == nil || !strings.Contains(err.Error(), "multica.exe") || !strings.Contains(err.Error(), "incomplete") {
			t.Fatalf("readPayload error = %v, want an incomplete-payload error naming multica.exe", err)
		}
	})

	t.Run("an empty file is rejected", func(t *testing.T) {
		payload := testPayload()
		payload["payload/u-workers.exe"] = &fstest.MapFile{Data: nil}
		_, err := readPayload(payload)
		if err == nil || !strings.Contains(err.Error(), "is empty") {
			t.Fatalf("readPayload error = %v, want an empty-payload-file error", err)
		}
	})

	t.Run("an unreadable entry reports the read failure", func(t *testing.T) {
		payload := testPayload()
		payload["payload/u-workers.exe"] = &fstest.MapFile{Mode: fs.ModeDir}
		_, err := readPayload(payload)
		if err == nil || errors.Is(err, errPayloadMissing) || !strings.Contains(err.Error(), "read payload/u-workers.exe") {
			t.Fatalf("readPayload error = %v, want a read failure for payload/u-workers.exe", err)
		}
	})
}

func TestInstallAndUninstall(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "U Workers", "bin")
	const originalPath = `C:\Windows\system32;C:\Tools`
	env := &fakeEnvironment{path: pathValue{raw: originalPath, kind: pathKindExpandString}}
	var out, errOut bytes.Buffer
	inst := installer{payload: testPayload(), env: env, out: &out, errOut: &errOut}

	if err := inst.install(dir, true); err != nil {
		t.Fatalf("install: %v\nstderr: %s", err, errOut.String())
	}

	for _, name := range []string{primaryBinaryName, legacyBinaryName} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read installed %s: %v", name, err)
		}
		if string(data) != "u-workers-binary" {
			t.Fatalf("installed %s = %q, want the payload bytes", name, data)
		}
	}

	if len(env.pathWrites) != 1 {
		t.Fatalf("PATH writes = %d, want 1", len(env.pathWrites))
	}
	if want := originalPath + ";" + dir; env.pathWrites[0].raw != want {
		t.Fatalf("written PATH = %q, want %q", env.pathWrites[0].raw, want)
	}
	if env.pathWrites[0].kind != pathKindExpandString {
		t.Fatalf("written PATH kind = %v, want REG_EXPAND_SZ preserved", env.pathWrites[0].kind)
	}
	if env.notifications != 1 {
		t.Fatalf("environment notifications = %d, want 1", env.notifications)
	}

	if env.uninstall == nil {
		t.Fatal("no Add/Remove Programs entry was written")
	}
	entry := *env.uninstall
	if entry.displayName != "U Workers CLI" || entry.publisher != "U Workers" || entry.installLocation != dir {
		t.Fatalf("Add/Remove Programs entry = %+v", entry)
	}
	if entry.displayVersion != version {
		t.Fatalf("DisplayVersion = %q, want %q", entry.displayVersion, version)
	}
	if !strings.HasSuffix(entry.uninstallString, " --uninstall") {
		t.Fatalf("UninstallString = %q, want it to end with --uninstall", entry.uninstallString)
	}

	// Reinstalling must not append a second PATH entry.
	if err := inst.install(dir, true); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if len(env.pathWrites) != 1 {
		t.Fatalf("PATH writes after reinstall = %d, want 1", len(env.pathWrites))
	}

	removed, err := inst.uninstall(dir)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !removed {
		t.Fatal("uninstall reported that nothing was installed")
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("install directory still exists after uninstall: %v", err)
	}
	if env.path.raw != originalPath {
		t.Fatalf("PATH after uninstall = %q, want %q", env.path.raw, originalPath)
	}
	if env.uninstall != nil {
		t.Fatal("Add/Remove Programs entry survived the uninstall")
	}
}

func TestInstallSkipsPathUpdate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bin")
	env := &fakeEnvironment{path: pathValue{raw: `C:\Windows`}}
	var errOut bytes.Buffer
	inst := installer{payload: testPayload(), env: env, out: io.Discard, errOut: &errOut, silent: true}

	if err := inst.install(dir, false); err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(env.pathWrites) != 0 || env.notifications != 0 {
		t.Fatalf("PATH was touched despite updatePath=false: writes=%d notifications=%d", len(env.pathWrites), env.notifications)
	}
	if env.uninstall == nil {
		t.Fatal("the Add/Remove Programs entry should still be written")
	}
}

func TestInstallWithoutPayload(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bin")
	env := &fakeEnvironment{}
	inst := installer{
		payload: fstest.MapFS{"payload/README.md": {Data: []byte("payload")}},
		env:     env,
		out:     io.Discard,
		errOut:  io.Discard,
	}

	err := inst.install(dir, true)
	if !errors.Is(err, errPayloadMissing) {
		t.Fatalf("install error = %v, want %v", err, errPayloadMissing)
	}
	if _, statErr := os.Stat(dir); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("install created %s before validating the payload", dir)
	}
	if len(env.pathWrites) != 0 || env.uninstall != nil {
		t.Fatal("a payload-less install must not touch the environment")
	}
}

func TestInstallWarnsWhenAddRemoveProgramsEntryFails(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bin")
	env := &fakeEnvironment{failEntry: errors.New("access is denied")}
	var errOut bytes.Buffer
	inst := installer{payload: testPayload(), env: env, out: io.Discard, errOut: &errOut}

	if err := inst.install(dir, true); err != nil {
		t.Fatalf("install must not fail when the Add/Remove Programs entry fails: %v", err)
	}
	if !strings.Contains(errOut.String(), "warning:") || !strings.Contains(errOut.String(), "Add/Remove Programs") {
		t.Fatalf("stderr = %q, want a warning about the Add/Remove Programs entry", errOut.String())
	}
	if _, err := os.Stat(filepath.Join(dir, primaryBinaryName)); err != nil {
		t.Fatalf("the binaries should still be installed: %v", err)
	}
}

func TestInstallFailsWhenPathUpdateFails(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bin")
	env := &fakeEnvironment{failSetPath: errors.New("access is denied")}
	inst := installer{payload: testPayload(), env: env, out: io.Discard, errOut: io.Discard}

	err := inst.install(dir, true)
	if err == nil || !strings.Contains(err.Error(), "update the user PATH") {
		t.Fatalf("install error = %v, want a PATH update failure", err)
	}
}

func TestUninstallWhenNothingIsInstalled(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing", "bin")
	env := &fakeEnvironment{path: pathValue{raw: `C:\Windows`}}
	inst := installer{payload: testPayload(), env: env, out: io.Discard, errOut: io.Discard}

	removed, err := inst.uninstall(dir)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if removed {
		t.Fatal("uninstall reported an installation that does not exist")
	}
	if len(env.pathWrites) != 0 || env.uninstall != nil {
		t.Fatal("uninstall touched the environment for a directory that does not exist")
	}
}

func TestUninstallKeepsOtherFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{primaryBinaryName, legacyBinaryName, "keep.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	env := &fakeEnvironment{path: pathValue{raw: dir + `;C:\Windows`}}
	inst := installer{payload: testPayload(), env: env, out: io.Discard, errOut: io.Discard}

	removed, err := inst.uninstall(dir)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !removed {
		t.Fatal("uninstall reported that nothing was installed")
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Fatalf("uninstall deleted a file it does not own: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, primaryBinaryName)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("%s survived the uninstall", primaryBinaryName)
	}
	if env.path.raw != `C:\Windows` {
		t.Fatalf("PATH after uninstall = %q, want only the foreign entry", env.path.raw)
	}
}

func TestConnectFlagsArgs(t *testing.T) {
	cases := []struct {
		name  string
		flags connectFlags
		want  []string
	}{
		{name: "nothing supplied", flags: connectFlags{}, want: []string{"connect"}},
		{
			name: "every flag supplied",
			flags: connectFlags{
				serverURL: "https://api.example.test", serverURLSet: true,
				rootCA: `C:\certs\ca.pem`, rootCASet: true,
				token: "tkn", tokenSet: true,
				appURL: "https://app.example.test", appURLSet: true,
				skipBrowserTrust: true, skipBrowserTrustSet: true,
			},
			want: []string{"connect", "--server-url", "https://api.example.test", "--root-ca", `C:\certs\ca.pem`, "--token", "tkn", "--app-url", "https://app.example.test", "--skip-browser-trust"},
		},
		{
			name:  "only one flag supplied",
			flags: connectFlags{rootCA: `C:\certs\ca.pem`, rootCASet: true},
			want:  []string{"connect", "--root-ca", `C:\certs\ca.pem`},
		},
		{
			name:  "an explicit false skip-browser-trust is not forwarded",
			flags: connectFlags{skipBrowserTrust: false, skipBrowserTrustSet: true},
			want:  []string{"connect"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.flags.args(); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("args() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseOptionsForwardsOnlySuppliedFlags(t *testing.T) {
	opts, err := parseOptions([]string{"--no-path", "--server-url", "https://api.example.test"}, io.Discard)
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if !opts.noPath || opts.noConnect || opts.uninstall || opts.printVersion {
		t.Fatalf("parsed flags = %+v", opts)
	}
	if !opts.connect.any() {
		t.Fatal("--server-url was supplied but connect is not enabled")
	}
	if got, want := opts.connect.args(), []string{"connect", "--server-url", "https://api.example.test"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("forwarded args = %v, want %v", got, want)
	}

	plain, err := parseOptions(nil, io.Discard)
	if err != nil {
		t.Fatalf("parseOptions(nil): %v", err)
	}
	if plain.connect.any() {
		t.Fatal("the default command line must not enable connect")
	}
}

func TestParseOptionsRejectsStrayArguments(t *testing.T) {
	if _, err := parseOptions([]string{"install"}, io.Discard); err == nil {
		t.Fatal("parseOptions accepted a positional argument")
	}
}

func TestCommandLine(t *testing.T) {
	if got, want := commandLine(`C:\U Workers\bin\u-workers.exe`, "connect"), `"C:\U Workers\bin\u-workers.exe" connect`; got != want {
		t.Fatalf("commandLine = %q, want %q", got, want)
	}
	if got, want := commandLine(`C:\bin\u-workers.exe`, "--uninstall"), `C:\bin\u-workers.exe --uninstall`; got != want {
		t.Fatalf("commandLine = %q, want %q", got, want)
	}
}
