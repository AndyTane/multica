# CLAWSTA-121 R1 verification

The implementation adds OSS publishing, fetching, safe delta preparation/application and a standalone company bootstrap. Initial automated checks used local fakes. Subsequent authorized real publishing succeeded as recorded below. No credentials were recorded and no existing service/daemon or firewall was changed.

## Checks

- `pwsh -NoProfile -File deploy/portable/test/oss-release.test.ps1 -EvidencePath evidence/clawsta-121/pwsh.json`: 66 checks, PowerShell 7.6.9, pass.
- `powershell.exe -NoProfile -File deploy/portable/test/oss-release.test.ps1 -EvidencePath evidence/clawsta-121/windows-powershell.json`: 66 checks, PowerShell 5.1.26100.9444, pass.
- The transport is an explicitly compiled fake ossutil; the temporary installation uses fake lifecycle functions/runners. It checks real release scripts, hashes, baseline validation, overlay writes, backups/journals and child exit handling without contacting OSS or starting real services.
- Existing portable-common and profile-ownership suites passed on Windows PowerShell 5.1. Additional regression output is recorded in the handoff.
- Official ossutil 2.4.1 Windows amd64 archive verified against the published SHA-256 `06ebec1680ce68b1807393e65a1e3f5f99c3b69c9b47c40578a80187da4462c3`. Its actual version/help and an upload `--dry-run` accepted the flags (including `--forbid-overwrite=true`, `--config-file NUL`, `--mode AK`, logging disabled). No request was sent.
- `real119-prepare.json`: actual 119 delta prepared without credentials, ZIP extracted, program write hashes/bytes and exact allowed file inventory checked. This reuses existing binaries; it does not claim a source rebuild.
- `existing-baseline-preflight.json`: actual old isolated v0.2.1/cfc93fef6 installation was correctly rejected because its modified install.ps1 differs from the required baseline hash. CheckOnly did not change files or services. A matching company baseline still needs its own preflight.
- All portable scripts parsed, and `git diff --check` passed.

## Remaining delivery inputs

Owner review corrected one reproduced defect: explicit `payload/source/*` deletions other than source.zip were discarded when rebuilding the obsolete-file list. The regression failed against R1, then passed after the scoped fix. Both PowerShell 5.1.26100.9444 and 7.6.9 now pass 67 checks; see `review-windows-powershell.json` and `review-pwsh.json`. The lifecycle remains the same; the fix retains all validated explicit source deletions in its backup journal. `review-real119.json` verifies the actual package again with the corrected helper. The updated bootstrap supersedes the executor's earlier ZIP.

Reviewed the direct write-only PUT sequence, immutable version keys, latest-last ordering, environment isolation and suppressed raw tool errors, download SHA-256 checks, archive/path boundaries, baseline checks and backup/overlay flow. Local implementation review passes after the deletion fix. Cloud/company acceptance and the PR exit are still open below.

The initial missing Git identity was resolved on 2026-11-03 when Tane confirmed using his identity. The authenticated account is AndyTane / AdnyTane (55218494); the worktree now uses AdnyTane <55218494+AndyTane@users.noreply.github.com>. No global identity was changed.

The first authorized real publish failed with HTTP 403 AccessDenied; see live-publish-attempt.json for that historical attempt. After Tane changed bucket authorization and requested a retry, publishing v0.2.2 succeeded: upgrade.zip, UPGRADE.md, release.json, then latest.json. Every immutable object used forbid-overwrite. The publisher made no read requests. The uploaded ZIP has 50,918,644 bytes and SHA-256 e6607ac33a6ea6e4e747611372e9d66abda5db3cbfa33b6052348d2d77e0f90b; see live-publish-success.json. ZIP metadata differs from earlier local preparations, so the uploaded manifest is authoritative.

Historical permissions review is closed by Tane: changing or narrowing account permissions is no longer in scope. The company installation directory is absent on this runner; do not claim company execution from local tests. The latest R2 acceptance permits in_review after merged/tagged full-source publication, validation and handoff, without waiting for company execution.

On Tane's request, also published releases/v0.2.2/UWorkers-v0.2.2-company-kit.zip: one archive containing the existing verified delta, Fetch/Apply helpers, official ossutil 2.4.1 and a Chinese start guide. Payload hashes, official executable/archive match and 291-file archive inventory checked locally. PutObject succeeded with forbid-overwrite; no existing release object or latest pointer changed. See company-kit-publish.json. This packaging-only delivery did not rerun code tests or perform a company upgrade.

Full package fallback: Tane reported the company baseline hash mismatch and requested a full portable package, then requested compression before upload. The complete 119 target inventory was assembled from exact-hash matching program files only: 9,132 files / 721,555,114 bytes; no data, .env, deployment configuration or credentials copied. The final ZIP includes tools and the full-package runbook: 9,142 files / 264,011,537 bytes, SHA-256 695494e810c627416a39fc2e1063b3bb274877306ed5a6ef3c8685a4013c6a81. Every ZIP file was hashed after compression. CLI v0.2.2/3e62bb6eb, Node v24.21.0 and PostgreSQL 17.11 version commands passed. No fresh compilation or code/lifecycle modification occurred. New-object PUT succeeded with forbid-overwrite; see full-package-publish.json and docs/windows-full-package-upgrade.md. Original delta/latest remain unchanged. Exact company error and actual full-package upgrade remain unverified.

## R2 full-source release candidate

Tane authorized merging the publishing tools with existing main fixes and releasing a new full baseline (v0.2.3). Current origin/main is 3e62bb6ebb9393210a7ed44fbad021611e9bc7ff and already includes PRs #20-22; there were no open PRs at inspection. The implementation starts from base 3e62bb6ebb9393210a7ed44fbad021611e9bc7ff. The three product package versions are prepared at 0.2.3.

The full-release path now builds from a clean tag, packages full.zip plus a complete file inventory and download tools, validates compressed contents, publishes immutable objects before latest, and downloads/verifies full releases without entering the differential Apply lifecycle. Existing delta behavior is retained. Evidence: r2-delta-pwsh.json and r2-delta-windows-powershell.json (67 checks each); full-review-pwsh.json and full-review-windows-powershell.json (full prepare/fetch/publish fixtures, including every failed upload stage). These are local fake transport/build tests, not fresh product compilation or company execution.

The full v0.2.3 source build and OSS publication follow the authorized merge and matching tag. At code review time, no v0.2.3 OSS request has occurred. Real build and upload evidence will be delivered on CLAWSTA-121 without amending the released tag. The previous v0.2.2 reused-binary full ZIP is historical and is not the new baseline release.
