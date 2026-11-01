//go:build windows

package main

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Per-user registry locations. Everything sits under HKCU, which is why an
// installation never needs administrator rights.
const (
	userEnvironmentKeyPath = "Environment"
	pathValueName          = "Path"
	uninstallKeyPath       = "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\" + uninstallKeyName
)

// Values for telling running processes the environment changed.
const (
	wmSettingChange            = 0x001A
	smtoAbortIfHung            = 0x0002
	settingChangeTimeoutMillis = 5000
	hwndBroadcast              = 0xFFFF
)

var sendMessageTimeout = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")

// registryEnvironment implements userEnvironment against the current user's
// registry hive: HKCU\Environment for PATH and the per-user Uninstall key for
// Add/Remove Programs.
type registryEnvironment struct{}

// newUserEnvironment returns the HKCU-backed environment writer.
func newUserEnvironment() userEnvironment {
	return registryEnvironment{}
}

// userPath reads HKCU\Environment\Path. A missing key or value is an empty PATH,
// not an error: a fresh profile has neither yet.
func (registryEnvironment) userPath() (pathValue, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, userEnvironmentKeyPath, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return pathValue{}, nil
		}
		return pathValue{}, fmt.Errorf("open HKCU\\%s: %w", userEnvironmentKeyPath, err)
	}
	defer key.Close()

	raw, kind, err := key.GetStringValue(pathValueName)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return pathValue{}, nil
		}
		return pathValue{}, fmt.Errorf("read HKCU\\%s\\%s: %w", userEnvironmentKeyPath, pathValueName, err)
	}
	return pathValue{raw: raw, kind: pathKindFromRegistry(kind)}, nil
}

// setUserPath writes the value back with the kind it was read as, so a PATH
// stored as REG_EXPAND_SZ keeps expanding for the tools that rely on it.
func (registryEnvironment) setUserPath(value pathValue) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, userEnvironmentKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open HKCU\\%s for writing: %w", userEnvironmentKeyPath, err)
	}
	defer key.Close()

	if value.kind == pathKindExpandString {
		if err := key.SetExpandStringValue(pathValueName, value.raw); err != nil {
			return fmt.Errorf("write HKCU\\%s\\%s: %w", userEnvironmentKeyPath, pathValueName, err)
		}
		return nil
	}
	if err := key.SetStringValue(pathValueName, value.raw); err != nil {
		return fmt.Errorf("write HKCU\\%s\\%s: %w", userEnvironmentKeyPath, pathValueName, err)
	}
	return nil
}

// writeUninstallEntry creates the Add/Remove Programs entry. Windows settings
// reads these exact value names.
func (registryEnvironment) writeUninstallEntry(entry uninstallEntry) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("create HKCU\\%s: %w", uninstallKeyPath, err)
	}
	defer key.Close()

	values := []struct{ name, value string }{
		{"DisplayName", entry.displayName},
		{"DisplayVersion", entry.displayVersion},
		{"Publisher", entry.publisher},
		{"InstallLocation", entry.installLocation},
		{"UninstallString", entry.uninstallString},
	}
	for _, value := range values {
		if err := key.SetStringValue(value.name, value.value); err != nil {
			return fmt.Errorf("write %s: %w", value.name, err)
		}
	}
	return nil
}

// removeUninstallEntry deletes the Add/Remove Programs entry. An entry that is
// already gone is not an error, so uninstalling twice stays quiet.
func (registryEnvironment) removeUninstallEntry() error {
	if err := registry.DeleteKey(registry.CURRENT_USER, uninstallKeyPath); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("delete HKCU\\%s: %w", uninstallKeyPath, err)
	}
	return nil
}

// notifyEnvironmentChange broadcasts WM_SETTINGCHANGE with "Environment" as the
// subject, which is what makes Explorer hand the new PATH to processes started
// afterwards. SendMessageTimeout bounds the wait because Explorer can be hung.
func (registryEnvironment) notifyEnvironmentChange() error {
	subject, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return fmt.Errorf("encode the WM_SETTINGCHANGE subject: %w", err)
	}
	result, _, callErr := sendMessageTimeout.Call(
		hwndBroadcast,
		wmSettingChange,
		0,
		uintptr(unsafe.Pointer(subject)),
		smtoAbortIfHung,
		settingChangeTimeoutMillis,
		0,
	)
	if result == 0 {
		if callErr == nil {
			callErr = errors.New("SendMessageTimeoutW returned 0")
		}
		return fmt.Errorf("broadcast WM_SETTINGCHANGE: %w", callErr)
	}
	return nil
}

// pathKindFromRegistry maps a registry value type to the kind PATH is rewritten
// as. Any other type is treated as a plain string.
func pathKindFromRegistry(kind uint32) pathKind {
	if kind == registry.EXPAND_SZ {
		return pathKindExpandString
	}
	return pathKindString
}
