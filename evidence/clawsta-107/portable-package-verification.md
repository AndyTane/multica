# Portable package — verification record (CLAWSTA-107)

Date: 2026-11-01/02. Machine: laptop `VGHH-XX`, Windows 11 家庭版 10.0.26200, unprivileged user `HuaWei`.
Repository branch: `clawsta-107-stacked` (stacked on CLAWSTA-104 / PR #17), draft PR #18.
Package under test: `UWorkers-Portable-v0.2.0` built from the branch on this machine; upgrade rehearsal target `UWorkers-Portable-v0.2.1` (same payload, different version label).

## What was exercised

The end-to-end harness is `deploy/portable/test/acceptance.ps1`. It copies the package into an isolated folder, points every child process at a dead HTTP proxy (`http://127.0.0.1:9`, i.e. dependency downloads disabled), runs `install.ps1` (the same script `Setup.cmd` calls), then verifies the result. Ports used for the isolation: HTTPS 23443, API 28080, web 23000, PostgreSQL 26433; the package CLI profile is `uworkers-acceptance-server` so the machine's existing Multica CLI state is not used.

### Passed

- Package copied to a clean folder; ports free before install; dependency downloads disabled for the install.
- One-script install completed; install report written; report says clean install; version matches the package.
- PostgreSQL cluster created inside the package; migrations applied (583 migrations up); API ready over HTTPS.
- LAN HTTPS entry started from the bundled Caddy; private root CA generated and `root-ca.crt` exported next to the scripts.
- Sign-in page and board served over HTTPS; the same HTTPS endpoint fails when the generated root CA is not trusted (proves the private-CA path rather than verification being disabled).
- Local operator account created; local password sign-in succeeds; onboarding completed; initial workspace created.
- Bundled daemon connected with a minted token: at least one runtime online and the **opencode** runtime online (the run saw `Opencode (移动专用机)` among the registered runtimes).
- Start-on-login item registered in the current user's Startup folder.
- Prebuilt `u-workers-setup` installer shipped in the package and starts (`--help`).
- Second-machine simulation: a personal access token was minted, the packaged CLI ran `connect` in a separate profile and exited 0. Note: runtimes are unique per (workspace, daemon, provider) and the daemon identity is host-derived, so a second profile on the same host shares the existing runtime rows; the acceptance now asserts what `connect` reports (`Connected to` + `runtimes: N`) rather than a row-count increase.

Evidence logs: `acceptance11.log` (install + HTTPS + account + runtime + startup), `acceptance13.log` (same plus prebuilt installer and second-machine connect).

### Not completed in this run

- **Offline installer rebuild from the bundled toolchain**: the harness only reached it once the earlier package defects were fixed; the final package copy used for the last runs no longer contained a complete `tools\go` tree, so no green rebuild was captured. The build script for it is `build-installer.ps1` (in the package) and the acceptance step exists; it needs one clean package and one run.
- **Overlay upgrade rehearsal**: the harness step exists (`-UpgradePackage`), but it never ran to completion because the preceding install/acceptance steps failed in the runs that reached that point.
- **Web release portability** (see below).

## Known limitation found while testing

The Next.js standalone output links its pnpm dependencies back into the build checkout (Windows junctions). The server side of the package (API, PostgreSQL, migrations, HTTPS entry, CLI/daemon, opencode runtime) is self-contained, but the prebuilt web release currently only runs on the machine that built it. Expanding those junctions breaks Next's sibling dependency resolution. The packaging keeps the links and warns; making the web release portable needs a packaging step that carries the pnpm store as real files and recreates package-local junctions during Setup (junction creation needs no administrator rights). This is the main remaining item for CLAWSTA-107.

## Machine state afterwards

- The acceptance run stops the test server, removes the "U Workers Portable.vbs" startup item and the `uworkers-acceptance-*` CLI profiles, and purges the scratch folder. Ports 23443/28080/23000/26433 are free again.
- The machine's existing Multica CLI config (`%USERPROFILE%\.multica\config.json`) is hashed before and after the run; it is unchanged.
