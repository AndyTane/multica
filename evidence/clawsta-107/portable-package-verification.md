# Portable package — verification record (CLAWSTA-107, formal handover build)

Date: 2026-11-02. Machine: laptop `VGHH-XX`, Windows 11 家庭版 10.0.26200, unprivileged user `HuaWei`.
Branch: `clawsta-107-stacked` (stacked on CLAWSTA-104 / PR #17), draft PR #18.

## Deliverables on the laptop

| item | path | size |
| --- | --- | --- |
| desktop handover archive (runtime) | `C:\UWorkers\packages\UWorkers-Portable-v0.2.0.zip` | 374,192,717 bytes (357 MiB) |
| full build folder (incl. offline rebuild toolchain) | `C:\UWorkers\packages\UWorkers-Portable-v0.2.0-full` | 4,053 MB |

The archive was produced from the full build with `deploy/portable/build-package.ps1` (branch commit at build time: `201e8f12b`, package manifest commit `50387aecd`), then the 3.1 GB offline Go toolchain was removed and `manifest.buildKit` set to `false` for the runtime handover. It contains the desktop handover sheet (`操作单-台式机.md`), the old-deployment finder (`Find-OldDeployment.cmd`), the prebuilt installer, the packaged runtime and the full standalone web release with package-local dependency links.

## Acceptance run on the built package: ACCEPTANCE PASSED (43 checks, 451.7 s)

```powershell
pwsh -File deploy\portable\test\acceptance.ps1 `
  -Package C:\UWorkers\packages\UWorkers-Portable-v0.2.0-full `
  -UpgradePackage C:\UWorkers\packages\UWorkers-Portable-v0.2.1 `
  -WorkRoot $env:TEMP\uw-a `
  -HideCheckoutPath <repository checkout>
```

Evidence: `evidence/clawsta-107/acceptance.json`. Isolated folder, dependency downloads disabled (dead HTTP proxy for every child process), no admin rights, isolated ports 23443/28080/23000/26433, package CLI profile kept away from the machine's Multica CLI state.

Verified in this run:

- one-script install; install report; PostgreSQL cluster + all migrations inside the package
- board, sign-in page and API over the generated CA's HTTPS; the same endpoint fails when that CA is not trusted
- the installed web release references only package files; with the build checkout hidden the board and API keep working (the harness hid the checkout's dependency store, because a build process still held the checkout root; every packaged dependency link pointed there, so this is an equivalent proof)
- local account + workspace + onboarding + local password sign-in
- bundled daemon auto-connect with the opencode runtime online; start-on-login item registered
- the prebuilt installer shipped in the package and runs; **installer rebuilt offline** from the bundled toolchain (20.6 MB)
- second simulated machine: PAT minted, packaged CLI `connect` exit 0, its daemon/runtime registration confirmed
- **overlay upgrade on an instance with data**: version changed, verified database backup created, marker issue still readable, row counts 1/1/1 unchanged
- existing Multica CLI config hash unchanged

Cleanup note: the automated cleanup still reported the simulated second-machine daemon releasing its profile folder a moment after the check (same transient as the previous run); that daemon and the profile folder were removed manually afterwards, the work root and startup item were removed by the harness, and ports were free again.

## Known limits

- The handover archive is the runtime package: it does not contain the offline rebuild toolchain (`tools\go`, `tools\gomodcache`). Rebuilding the installer needs the full build folder on the laptop.
- Install path should be short: Windows still limits paths to 260 characters; Setup fails with an actionable message when the release cannot be copied completely.
- Unsinged binaries; SmartScreen warns.
- Browser root-CA trust needs the one-time Windows confirmation dialog; `Trust-Browser.cmd` covers it.
- `Setup.cmd -AdoptRoot` (take over an existing native deployment after a backup) was rehearsed in the overlay-upgrade test but not against the desktop's real CLAWSTA-81 deployment; that is the desktop step, and it backs up before touching anything.
