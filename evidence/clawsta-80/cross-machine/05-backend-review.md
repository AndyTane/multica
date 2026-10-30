# C1 backend review (2026-10-31, UTC+08)

Reviewed candidate `6b213959c..5e54bb54f`, adopted as `27e3ae088` on PR #3.
The only cherry-pick conflict was two adjacent sections in
`docs/windows-native.md`; both desktop packaging and clean deployment sections
were retained. Browser evidence and lifecycle evidence were inspected, not rerun.

Independent read-only checks at approximately 03:30:

- Fresh SSH `Status`: Running true, scheduled task registered; supervisor
  7736 at `2026-10-30T19:22:54.9930105Z`, backend 4836 at
  `2026-10-30T19:22:55.5546558Z`, frontend 4624 at
  `2026-10-30T19:22:55.5835551Z`. Executable paths matched C1 records.
- Windows task state 4 (Running), account SYSTEM.
- API loopback readyz: db ok, migrations ok. Client LAN `/login`: HTTP 200;
  `/health`: ok, same backend PID. Its commit field is `unknown`, consistent
  with the documented source-archive build; no claim of a Git-stamped binary.
- Remote lifecycle script SHA-256 exactly matched the candidate source:
  `C85370C6C8EC6443CCCF2C4F9D33979C80AAC04A7FB55B0FE346555012FE2F4F`.
- First bare SSH script invocation failed due to execution policy. Repeating
  with process-scoped `-ExecutionPolicy Bypass` succeeded. The health probes
  already succeeded; no global setting was modified.

Two focused regression cases failed before the reviewer fixes:

1. A single/default origin became `h` instead of a URL in deployment.json.
2. Configure on an imported configuration failed with missing WebHost property.

After collecting origins as an array and adding the optional properties,
`powershell -NoProfile -ExecutionPolicy Bypass -File scripts/native-windows.test.ps1`
passed (exit 0). Tests cover real temporary JSON writes, downstream origin/CORS
environment values and upgrading imported settings. Existing script checks also
passed; no production database, real agent or full acceptance matrix was run.

At approximately 03:33 the corrected script was copied to both target
`repo\scripts\native-windows.ps1` and `native\runtime\native-windows.ps1`.
Both original hashes and the uploaded candidate hash were checked before writes;
each original is retained as the adjacent `.c1-before-review` file. Installed
SHA-256: `C6B88D9F07F25156BF8B2D3D3674B8DCA64CCB6B07C671F35D958C4BEB5D1966`.
The temporary transfer file was removed. Status still reports the same three
process records; the fixes concern preparation/configuration, so the running
two-origin instance did not need a restart.

C1 passes backend review. Physical reboot, company LAN, HTTPS and Tailscale
remain outside these results. D2 desktop observation is separate and pending.
