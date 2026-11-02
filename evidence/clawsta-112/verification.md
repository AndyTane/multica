# CLAWSTA-112 - daemon status under a non-ASCII device name

Date: 2026-11-02. Repository: `AndyTane/UWorkers-nice`, branch `agent/dsh/308d3be5b0c8` (PR #19).
Fix commit: `ea31eb5da` (`fix(deploy): read CLI JSON as UTF-8 so a Chinese device name
cannot hide a running daemon`).

## Symptom

On Windows PowerShell 5.1 the portable package reported a running daemon as
`stopped` on any machine whose computer name is not ASCII (this laptop is
`移动专用机`). Start.cmd runs on the system 5.1 at sign-in, so a machine with a
Chinese name could silently skip the daemon start while the desktop (ASCII name)
was fine.

## Reproduction (Windows PowerShell 5.1)

The captured `u-workers daemon status --output json` document was read with
`Get-Content -Raw` without `-Encoding`. On 5.1 that decodes a BOM-less file in
the ANSI code page (gb2312 here), while PowerShell 7 uses UTF-8:

    PS> $raw = Get-Content -LiteralPath status.json -Raw
    PS> $raw
    {
      "device_name": "缁夎濮╂稉鎾舵暏閺?,
      "status": "running",
      ...
    PS> $raw.Trim() | ConvertFrom-Json
    PARSE FAILED: Invalid object passed in, ':' or '}' expected. (37)

The mangled bytes swallowed the quote that closes the JSON string, so the parse
failed, `Get-PortableDaemonStatus` treated the document as unparseable, and
`Running = $false` was reported for a live daemon.

The installer had the same class of read: `local-auth` output was read the same
way, and HTTPS response bodies (runtime rows carry the device name) were decoded
the same way before `ConvertFrom-Json`.

## Fix

Every read of a document the package's own tools write now decodes UTF-8
explicitly in both shells:

- `Read-PortableUtf8Text` reads a captured file with an explicit UTF-8 decoder.
- `Invoke-PortableCommand -Utf8Output` decodes captured stdout/stderr as UTF-8;
  `Invoke-PortableCli` sets it because the CLI is a Go program. Tools that write
  the console code page (Caddy, PostgreSQL, robocopy, native server script) keep
  the previous decoding.
- `Invoke-PortableWebRequest` reads the HTTPS response body as UTF-8.
- `Invoke-PortablePasswordInput` in install.ps1 reads the local-auth logs as
  UTF-8.
- `Find-OldDeployment.cmd` reads `deployment.json` with `-Encoding UTF8`.

The daemon verdict moved into `ConvertFrom-PortableDaemonStatus` (pure
exit-code + output) so the parse can be tested without a process.

## Tests

`deploy/portable/test/portable-common.test.ps1` now writes the exact UTF-8 bytes
the CLI writes (a fake status document with device name `移动专用机`), pushes them
through the real capture path (`cmd.exe` redirection -> captured-output read) and
judges the captured text with `ConvertFrom-PortableDaemonStatus`. It asserts
running/starting count as alive, stopped stays stopped, and the device name
round-trips byte for byte. A second assertion checks that `Invoke-PortableCli`
asks the capture for UTF-8.

- Both shells pass: `powershell.exe` 5.1 -> `portable-common tests passed.`;
  `pwsh` 7 -> `portable-common tests passed.`
- To prove the test is a real regression test, the UTF-8 read was temporarily
  replaced by the previous ANSI read in a scratch copy: under 5.1 it fails with
  `FAILED: A running daemon with a non-ASCII device name must be reported as
  running.`, and under 7 it passes (7 never had the bug). With the fix both
  shells pass.
- The other portable suites still pass in both shells (`adoption.test.ps1`,
  `profile-ownership.test.ps1`).
- CI: `portable-common.test.ps1` now also runs under PowerShell 7, and
  `deploy/portable/**` selects the `installer` job, so a PR that only touches the
  portable package actually runs these tests (it did not before: the `installer`
  job was skipped on PR #19).

## Acceptance

Package: `UWorkers-Portable-v0.2.4` assembled with `build-package.ps1
-ReusePayloadFrom` (v0.2.2 payload + this branch's scripts; `-SkipBuildKit
-NoZip`), manifest commit `ea31eb5da`.

PowerShell 7 (the full documented run):

    pwsh -File deploy/portable/test/acceptance.ps1 \
      -Package D:\UWorkers\clawsta112\dist\UWorkers-Portable-v0.2.4 \
      -WorkRoot D:\UWorkers\uworkers-112-accept \
      -EvidencePath D:\UWorkers\clawsta112\acceptance-112.json -SkipInstallerBuild

    ACCEPTANCE PASSED (63 checks, 237.4 s)

All 62 recorded checks pass, cleanup left no process or port behind, and the
runtime rows show the Chinese device name decoded correctly:
`Claude (移动专用机)`, `Codex (移动专用机)`, `Openclaw (移动专用机)`,
`Opencode (移动专用机)`. The checks include "local daemon reports running", "a
stopped daemon is reported as stopped" and "the local daemon is running again
after the recovery".

An earlier run of the same script hit the known second-machine cleanup race
(the daemon still held its profile log when cleanup removed it; the same race is
in `evidence` from CLAWSTA-110). That run still passed all functional checks;
the leftovers were stopped by exact PID and removed, and the rerun above is
clean.

Windows PowerShell 5.1 (the shell where the bug lived):

    powershell.exe -File deploy/portable/test/acceptance.ps1 ... (same arguments)

The run passed 56 checks - including `local daemon reports running`, `a stopped
daemon is reported as stopped`, `the local daemon is running again after the
recovery`, the runtime rows and the Caddy listener checks - and then failed at
the pre-existing installer check: `& u-workers-setup.exe --help 2>&1 | Out-String`
under `$ErrorActionPreference = 'Stop'` throws a `NativeCommandError` on 5.1
because the installer writes its usage to stderr. That is unrelated to this
change (and predates it; the full acceptance has only ever been run under
PowerShell 7). The UTF-8 fix itself is verified on 5.1 by the regression test and
by the 56 acceptance checks above.

No deployment on the desktop was touched; no U Workers server, daemon or profile
from the acceptance area is left on this laptop.
