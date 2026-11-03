# OSS release R1 implementation plan

**Goal:** Implement the approved CLAWSTA-121 R1 release and fetch commands.

**Architecture:** Use direct ossutil 2 API operations and fixed object keys. Validate a manifest-driven sparse archive, then reuse the 119 backup/overlay/native lifecycle. Keep preparation usable without cloud credentials.

**Tech Stack:** PowerShell 5.1 consumer, PowerShell 7 source builder, ossutil 2, JSON and ZIP.

1. Add shared validation and direct OSS calls in `deploy/portable/oss-release-common.ps1`. Check paths, program-file scope, hashes, JSON contracts, archive entries and tool failures.
2. Add `build-upgrade.ps1` and `publish-release.ps1`. Build clean selected source through the existing package builder, compare an explicit package baseline, or validate the existing 119 delta. Upload immutable artifacts before release.json and latest.json.
3. Adapt the 119 `Apply-Upgrade.ps1`, add `Fetch-Upgrade.ps1` and reject sparse `-Source` in `upgrade.ps1`. Validate before service operations and retain journals/backups.
4. Add `build-fetch-bootstrap.ps1` and `docs/windows-oss-releases.md`. Deliver scripts and manual verified official ossutil installation instructions.
5. Run fake-OSS failure and success tests on both consumer shells, existing portable regressions, and real 119 PrepareOnly/archive checks. Record evidence, commit with the configured user identity, push the task branch and open a main-based PR. No real upload or merge.
