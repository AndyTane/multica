//go:build windows

package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Product identity, as shown in the installation and in Add/Remove Programs.
const (
	// displayName is the application name Windows settings lists.
	displayName = "U Workers CLI"
	// productName is the product name used in sentences shown to operators.
	productName = "U Workers"
	// publisher is the vendor Add/Remove Programs shows.
	publisher = "U Workers"
	// installDirName is the per-user directory that owns the installation.
	installDirName = "U Workers"
	// binDirName is the folder inside installDirName that holds the binaries.
	binDirName = "bin"
	// uninstallKeyName is the Add/Remove Programs subkey this installer owns.
	uninstallKeyName = "UWorkersCLI"
)

// Installed executable names. A release installs both: u-workers is the command
// users run, and multica keeps the pre-rename command working from the same
// bytes.
const (
	primaryBinaryName = "u-workers.exe"
	legacyBinaryName  = "multica.exe"
)

// payloadDir is the package directory that //go:embed payload/* reads.
const payloadDir = "payload"

// errPayloadMissing is the failure of an installer built without the payload:
// the directory then only holds its committed README.md, so the operator gets
// the build step instead of a half-install.
var errPayloadMissing = errors.New("installer payload missing; build it with scripts/build-windows-installer.ps1")

// pathKind is the registry value kind the user PATH is stored as. The kind
// travels with the value so a rewrite keeps REG_EXPAND_SZ: writing PATH back as
// REG_SZ would turn other tools' %VAR% entries into literals.
type pathKind int

const (
	pathKindString pathKind = iota
	pathKindExpandString
)

// pathValue is the user PATH exactly as stored, plus the kind it is stored as.
type pathValue struct {
	raw  string
	kind pathKind
}

// normalizePathEntry is the form two PATH entries are compared in: trimmed, with
// a trailing separator dropped, and lowercased.
func normalizePathEntry(entry string) string {
	trimmed := strings.TrimSpace(entry)
	if trimmed == "" {
		return ""
	}
	return strings.ToLower(filepath.Clean(trimmed))
}

// pathContains reports whether the PATH value already holds dir, ignoring case
// and a trailing separator.
func pathContains(raw, dir string) bool {
	target := normalizePathEntry(dir)
	if target == "" {
		return false
	}
	for _, entry := range strings.Split(raw, ";") {
		if normalizePathEntry(entry) == target {
			return true
		}
	}
	return false
}

// appendPathEntry returns raw with dir appended, or raw unchanged when it is
// already present. The second result reports whether the value changed.
func appendPathEntry(raw, dir string) (string, bool) {
	dir = strings.TrimSpace(dir)
	if dir == "" || pathContains(raw, dir) {
		return raw, false
	}
	if strings.TrimSpace(raw) == "" {
		return dir, true
	}
	if strings.HasSuffix(raw, ";") {
		return raw + dir, true
	}
	return raw + ";" + dir, true
}

// removePathEntry returns raw without the entry matching dir, or raw unchanged
// when that entry is absent. Every other entry is kept byte for byte.
func removePathEntry(raw, dir string) (string, bool) {
	target := normalizePathEntry(dir)
	if target == "" || strings.TrimSpace(raw) == "" {
		return raw, false
	}
	entries := strings.Split(raw, ";")
	kept := make([]string, 0, len(entries))
	removed := false
	for _, entry := range entries {
		if normalizePathEntry(entry) == target {
			removed = true
			continue
		}
		kept = append(kept, entry)
	}
	if !removed {
		return raw, false
	}
	return strings.Join(kept, ";"), true
}

// defaultInstallDir resolves the per-user install directory from the values of
// LOCALAPPDATA and USERPROFILE. USERPROFILE is the documented fallback when
// LOCALAPPDATA is unset.
func defaultInstallDir(localAppData, userProfile string) (string, error) {
	if localAppData = strings.TrimSpace(localAppData); localAppData != "" {
		return filepath.Join(localAppData, installDirName, binDirName), nil
	}
	if userProfile = strings.TrimSpace(userProfile); userProfile != "" {
		return filepath.Join(userProfile, "AppData", "Local", installDirName, binDirName), nil
	}
	return "", errors.New("cannot resolve %LOCALAPPDATA% or %USERPROFILE%; pass --install-dir")
}

// payloadBinaries is the pair of executables an installation writes.
type payloadBinaries struct {
	primary []byte
	legacy  []byte
}

// readPayload loads the embedded CLI binaries through fsys, so tests can serve a
// synthetic payload while the build serves the embedded one.
func readPayload(fsys fs.FS) (payloadBinaries, error) {
	names := []string{primaryBinaryName, legacyBinaryName}
	binaries := make(map[string][]byte, len(names))
	for _, name := range names {
		content, err := fs.ReadFile(fsys, path.Join(payloadDir, name))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return payloadBinaries{}, fmt.Errorf("read %s/%s: %w", payloadDir, name, err)
		}
		if len(content) == 0 {
			return payloadBinaries{}, fmt.Errorf("installer payload file %s/%s is empty; rebuild it with scripts/build-windows-installer.ps1", payloadDir, name)
		}
		binaries[name] = content
	}
	if len(binaries) == 0 {
		return payloadBinaries{}, errPayloadMissing
	}
	for _, name := range names {
		if _, ok := binaries[name]; !ok {
			return payloadBinaries{}, fmt.Errorf("installer payload is incomplete: %s/%s is missing; rebuild it with scripts/build-windows-installer.ps1", payloadDir, name)
		}
	}
	return payloadBinaries{primary: binaries[primaryBinaryName], legacy: binaries[legacyBinaryName]}, nil
}

// uninstallEntry is the Add/Remove Programs record of one installation.
type uninstallEntry struct {
	displayName     string
	displayVersion  string
	publisher       string
	installLocation string
	uninstallString string
}

// userEnvironment is the per-user state this installer touches outside the
// install directory: the user PATH and the Add/Remove Programs entry.
// install_windows.go implements it against HKCU; the tests implement it in
// memory, so installation is testable without a registry.
type userEnvironment interface {
	// userPath returns the stored user PATH and the kind it is stored as.
	userPath() (pathValue, error)
	// setUserPath stores the user PATH with the given value kind.
	setUserPath(pathValue) error
	// writeUninstallEntry creates or replaces the Add/Remove Programs entry.
	writeUninstallEntry(uninstallEntry) error
	// removeUninstallEntry deletes it. An absent entry is not an error.
	removeUninstallEntry() error
	// notifyEnvironmentChange broadcasts WM_SETTINGCHANGE so new shells pick up
	// the user PATH without a sign-out.
	notifyEnvironmentChange() error
}

// installer copies the embedded payload and updates the per-user environment
// around it. The payload and the environment are injected so install and
// uninstall are testable without the embed or a registry.
type installer struct {
	payload fs.FS
	env     userEnvironment
	out     io.Writer
	errOut  io.Writer
	silent  bool
}

// progress reports one step to the operator. --silent suppresses these lines.
func (i installer) progress(format string, args ...any) {
	if i.silent || i.out == nil {
		return
	}
	fmt.Fprintf(i.out, format+"\n", args...)
}

// warn reports a degraded step. Warnings survive --silent: they explain why the
// installation is less complete than it looks.
func (i installer) warn(format string, args ...any) {
	if i.errOut == nil {
		return
	}
	fmt.Fprintf(i.errOut, "u-workers-setup: warning: "+format+"\n", args...)
}

// install writes the CLI binaries into dir, points the user PATH at dir, and
// registers the installation for removal. A PATH failure stops the install
// because the user asked for a command they can run; the Add/Remove Programs
// entry only warns, since the binaries work without it.
func (i installer) install(dir string, updatePath bool) error {
	binaries, err := readPayload(i.payload)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create install directory %s: %w", dir, err)
	}
	for _, binary := range []struct {
		name string
		data []byte
	}{
		{primaryBinaryName, binaries.primary},
		{legacyBinaryName, binaries.legacy},
	} {
		target := filepath.Join(dir, binary.name)
		if err := os.WriteFile(target, binary.data, 0o755); err != nil {
			return fmt.Errorf("write %s: %w (stop any running u-workers or multica process and retry)", target, err)
		}
		i.progress("Installed %s", target)
	}
	if updatePath {
		if err := i.addToUserPath(dir); err != nil {
			return err
		}
	}
	i.registerInstall(dir)
	i.progress("Installed %s %s into %s", productName, version, dir)
	return nil
}

// addToUserPath appends dir to the user PATH unless it is already there.
func (i installer) addToUserPath(dir string) error {
	current, err := i.env.userPath()
	if err != nil {
		return err
	}
	updated, appended := appendPathEntry(current.raw, dir)
	if !appended {
		i.progress("%s is already on the user PATH", dir)
		return nil
	}
	if err := i.env.setUserPath(pathValue{raw: updated, kind: current.kind}); err != nil {
		return fmt.Errorf("update the user PATH: %w", err)
	}
	i.progress("Added %s to the user PATH; new terminals find u-workers without a sign-out", dir)
	i.broadcastEnvironmentChange()
	return nil
}

// removeFromUserPath drops the exact entry install added. Other PATH entries are
// left untouched. The second result reports whether the value changed.
func (i installer) removeFromUserPath(dir string) (bool, error) {
	current, err := i.env.userPath()
	if err != nil {
		return false, err
	}
	updated, removed := removePathEntry(current.raw, dir)
	if !removed {
		return false, nil
	}
	if err := i.env.setUserPath(pathValue{raw: updated, kind: current.kind}); err != nil {
		return false, fmt.Errorf("update the user PATH: %w", err)
	}
	i.broadcastEnvironmentChange()
	return true, nil
}

// broadcastEnvironmentChange tells running processes the environment changed.
// A failed broadcast is a warning: the PATH value is already written.
func (i installer) broadcastEnvironmentChange() {
	if err := i.env.notifyEnvironmentChange(); err != nil {
		i.warn("could not broadcast the environment change: %v", err)
	}
}

// registerInstall writes the Add/Remove Programs entry, best effort: a failure
// here never fails the install.
func (i installer) registerInstall(dir string) {
	setupPath, err := os.Executable()
	if err != nil {
		i.warn("could not locate this installer, so the Add/Remove Programs entry was skipped: %v", err)
		return
	}
	entry := uninstallEntry{
		displayName:     displayName,
		displayVersion:  version,
		publisher:       publisher,
		installLocation: dir,
		uninstallString: commandLine(setupPath, "--uninstall"),
	}
	if err := i.env.writeUninstallEntry(entry); err != nil {
		i.warn("could not write the Add/Remove Programs entry: %v", err)
		return
	}
	i.progress("Registered %q in Add/Remove Programs", displayName)
}

// uninstall removes the installed binaries, the directory once it is empty, the
// exact PATH entry install added, and the Add/Remove Programs entry. The result
// reports whether there was an installation to remove; registry failures only
// warn so that the binaries are always gone.
func (i installer) uninstall(dir string) (bool, error) {
	removedBinaries := false
	for _, name := range []string{primaryBinaryName, legacyBinaryName} {
		target := filepath.Join(dir, name)
		if err := os.Remove(target); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return removedBinaries, fmt.Errorf("remove %s: %w (stop any running u-workers or multica process and retry)", target, err)
		}
		removedBinaries = true
		i.progress("Removed %s", target)
	}
	if removedBinaries {
		// Only a directory this install emptied is ours to delete.
		i.removeEmptyDir(dir)
	} else if _, err := os.Stat(dir); err != nil {
		// Nothing is installed here, so leave the environment alone and let the
		// caller report that there was nothing to remove.
		return false, nil
	}
	if changed, err := i.removeFromUserPath(dir); err != nil {
		i.warn("could not remove %s from the user PATH: %v", dir, err)
	} else if changed {
		i.progress("Removed %s from the user PATH", dir)
	}
	if err := i.env.removeUninstallEntry(); err != nil {
		i.warn("could not remove the Add/Remove Programs entry: %v", err)
	} else {
		i.progress("Removed the Add/Remove Programs entry")
	}
	return true, nil
}

// removeEmptyDir deletes dir when nothing is left in it. A directory holding
// other files is kept, because those files are not ours to delete.
func (i installer) removeEmptyDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	if len(entries) > 0 {
		i.progress("Kept %s because it still contains other files", dir)
		return
	}
	if err := os.Remove(dir); err == nil {
		i.progress("Removed %s", dir)
	}
}
