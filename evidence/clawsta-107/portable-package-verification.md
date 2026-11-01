# Portable package — verification record (CLAWSTA-107, formal handover build)

Date: 2026-11-02. Machine: laptop `VGHH-XX`, Windows 11 家庭版 10.0.26200, unprivileged user `HuaWei`.
Branch: `clawsta-107-stacked` (stacked on CLAWSTA-104 / PR #17), draft PR #18.

## Deliverables on the laptop

| item | path | size |
| --- | --- | --- |
| desktop handover archive (runtime) | `C:\UWorkers\packages\UWorkers-Portable-v0.2.0.zip` | 374,192,717 bytes (357 MiB) |
| full build folder (incl. offline rebuild toolchain) | `C:\UWorkers\packages\UWorkers-Portable-v0.2.0-full` | 4,053 MB |

The archive was produced from the full build with `deploy/portable/build-package.ps1`, then the 3.1 GB offline Go toolchain was removed and `manifest.buildKit` set to `false` for the runtime handover. **Historical note:** that build records the branch commit `201e8f12b` while its manifest records `50387aecd`, so the build source and the manifest/source snapshot did not name the same revision. It is kept as a historical record of the pre-R1 handover only; it is not evidence about the R1 head and is not relabelled. The R1 isolated build records build source, manifest and `payload/source/source.zip` against one revision in `evidence/clawsta-107/r1-verification.md`. It contains the desktop handover sheet (`操作单-台式机.md`), the old-deployment finder (`Find-OldDeployment.cmd`), the prebuilt installer, the packaged runtime and the full standalone web release with package-local dependency links.

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
- the installed web release references only package files (no dependency link points outside the package), and the board and API keep working while the build checkout's dependency store is unavailable. The harness could not hide the whole checkout: a build process still held the checkout root, so the directory rename was refused and the fallback hid `node_modules` instead. That only proves the dependency store is not needed at run time; it does **not** prove the whole checkout can disappear, and the R1 record does not claim it does.
- local account + workspace + onboarding + local password sign-in
- bundled daemon auto-connect with the opencode runtime online; start-on-login item registered
- the prebuilt installer shipped in the package and runs; **installer rebuilt offline** from the bundled toolchain (20.6 MB)
- second simulated machine: PAT minted, packaged CLI `connect` exit 0, its daemon/runtime registration confirmed
- **overlay upgrade on an instance with data**: version changed, verified database backup created, marker issue still readable, row counts 1/1/1 unchanged. This is a version-to-version upgrade inside one installation folder; it is **not** a first-time `-AdoptRoot` rehearsal.
- existing Multica CLI config hash unchanged

Cleanup note: the automated cleanup still reported the simulated second-machine daemon releasing its profile folder a moment after the check (same transient as the previous run); that daemon and the profile folder were removed manually afterwards, the work root and startup item were removed by the harness, and ports were free again.

## Known limits

- The handover archive is the runtime package: it does not contain the offline rebuild toolchain (`tools\go`, `tools\gomodcache`). Rebuilding the installer needs the full build folder on the laptop.
- Install path should be short: Windows still limits paths to 260 characters; Setup fails with an actionable message when the release cannot be copied completely.
- Unsinged binaries; SmartScreen warns.
- Browser root-CA trust needs the one-time Windows confirmation dialog; `Trust-Browser.cmd` covers it.
- `Setup.cmd -AdoptRoot` (take over an existing native deployment after a backup) is covered by the R1-1 regression tests and the isolated real-deployment adoption acceptance recorded in `evidence/clawsta-107/r1-verification.md`; the overlay-upgrade run above is not adoption evidence. The desktop's real CLAWSTA-81 deployment is still the step reserved for the desktop machine, and adoption backs it up before touching anything.
