# Portable package — verification record (CLAWSTA-107, green run)

Date: 2026-11-02. Machine: laptop `VGHH-XX`, Windows 11 家庭版 10.0.26200, unprivileged user `HuaWei`.
Repository branch: `clawsta-107-stacked` (stacked on CLAWSTA-104 / PR #17), draft PR #18.
Package under test: `UWorkers-Portable-v0.2.0` built from the branch on this machine; upgrade rehearsal target `UWorkers-Portable-v0.2.1` (same payload, different version label).

## Command

```powershell
pwsh -File deploy\portable\test\acceptance.ps1 `
  -Package D:\UWorkers\clawsta107\dist\UWorkers-Portable-v0.2.0 `
  -UpgradePackage D:\UWorkers\clawsta107\dist\UWorkers-Portable-v0.2.1 `
  -WorkRoot $env:TEMP\uw-a `
  -HideCheckoutPath <repository checkout>
```

The harness copies the package into an isolated folder, points every child process at a dead HTTP proxy (`http://127.0.0.1:9`, dependency downloads disabled), runs `install.ps1` (exactly what `Setup.cmd` calls), and verifies the result. Isolated ports: HTTPS 23443, API 28080, web 23000, PostgreSQL 26433. The package CLI profile is `uworkers-acceptance-server` so the machine's existing Multica CLI state is not used.

## Result: ACCEPTANCE PASSED — 42 checks, 384.7 s

Evidence: `evidence/clawsta-107/acceptance.json` (machine-readable, includes every check, package hashes, runtimes, upgrade report).

### The three requested items

1. **Web dependencies packaged portably, board works with the build checkout hidden.**
   The standalone Next.js release keeps a pnpm store inside the folder but links a few package directories back into the build checkout. Packaging now expands the store-internal links into real files, renames the store directories to short hashes (Windows path limit), records the two app-level links in `portable-links.json`, and Setup/Upgrade recreate them as **package-local junctions** (no administrator rights needed).
   Verified: `installed web release references only files inside the package` PASS; the build checkout was renamed away during the run and `board works with the build checkout hidden` PASS, `API works with the build checkout hidden` PASS.
2. **Offline installer rebuild.** `Build-Installer.cmd` rebuilt `u-workers-setup-v0.2.0-windows-amd64.exe` (20.6 MB) from the bundled Go toolchain, module cache and source snapshot with the dead proxy in place: `installer rebuilt offline from the bundled toolchain` PASS.
3. **Overlay upgrade on an instance with data.** Marker issue created on v0.2.0 → upgrade to v0.2.1: version changed, backup `backups\native-20261102-005306` created and verified, API ready after restart, marker issue still readable, row counts 1 user / 1 workspace / 1 issue before and after: all PASS.

### Also verified in the same run

- one-script install completed; report written; clean install; version matches the package
- PostgreSQL cluster + all migrations inside the package; API ready over HTTPS; private root CA exported; the same endpoint fails when that CA is not trusted (verification is real, not disabled)
- local operator account created, local password sign-in works, onboarding completed, initial workspace created
- bundled daemon auto-connected; opencode runtime online (`Opencode (移动专用机)`), plus the machine's other agent CLIs
- start-on-login item registered in the current user's Startup folder
- prebuilt `u-workers-setup` shipped in the package and runs
- second simulated machine: PAT minted, packaged CLI `connect` exited 0, its daemon/runtime registration confirmed
- the machine's existing Multica CLI config hash unchanged

### Cleanup note

The automated cleanup reported one transient warning: the simulated second-machine daemon was still releasing `~/.multica/profiles/uworkers-acceptance-second` when the harness tried to remove it (the process finished a moment later). After the run the leftover daemon and profile directory were removed manually; the work root, startup item and all test processes are gone and ports 23443/28080/23000/26433 are free again. Multica's own service and config were not touched.

## Known limits (unchanged)

- Install path should be short: Windows still has the 260-character path limit; store directory names are shortened at packaging time and Setup fails with an actionable message if the release cannot be copied completely.
- Unsinged binaries; SmartScreen warns.
- Browser root-CA trust requires the one-time Windows confirmation dialog; `-SkipBrowserTrust` / `Trust-Browser.cmd` cover it.
- A PostgreSQL major-version move is out of scope for the overlay upgrade.
- `Setup.cmd -AdoptRoot` (take over an existing native deployment after a backup) is implemented but was not rehearsed against a real old deployment on this laptop; that step belongs to the desktop machine.
