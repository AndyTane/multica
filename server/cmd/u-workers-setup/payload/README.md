# Installer payload

`u-workers-setup.exe` embeds the CLI binaries it installs from this directory:

| File | Installed as | Source |
| --- | --- | --- |
| `u-workers.exe` | `%LOCALAPPDATA%\U Workers\bin\u-workers.exe` | built from `server/cmd/multica` |
| `multica.exe` | `%LOCALAPPDATA%\U Workers\bin\multica.exe` | copy of `u-workers.exe` (legacy command name) |

Neither binary is committed: both are build outputs produced by
`scripts/build-windows-installer.ps1`, which stages them here, builds the
installer, and removes them again.

```powershell
pwsh -NoProfile -File scripts/build-windows-installer.ps1 -Version 1.2.3
```

This README is what keeps `//go:embed payload/*` valid for a plain
`go build ./...`; the embed pattern needs at least one file in the directory. An
installer built without the payload fails at runtime with:

    installer payload missing; build it with scripts/build-windows-installer.ps1
