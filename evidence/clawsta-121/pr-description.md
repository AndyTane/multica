## What does this PR do?

CLAWSTA-121 adds immutable OSS publishing and verified Windows downloads. v0.2.3 establishes a complete portable baseline after differently rebuilt v0.2.1 installations made delta upgrades fail. Full releases carry a tag/commit, complete inventory, upgrade instructions and download tools; delta releases retain their strict baseline checks.

## Related Issue

CLAWSTA-121

## Type of Change

- [x] New feature
- [x] Documentation update
- [x] Tests

## Changes Made

- Publish source-built full or differential ZIPs, then instructions and manifest, then latest. Version objects forbid overwrite; publisher performs no remote reads.
- Validate release identity, size/hash, ZIP paths and file inventory before handing off upgrades. Full releases use the documented complete Source procedure; differential Apply keeps backup/rollback journals.
- Bundle Fetch and ossutil in full releases so company downloads do not require GitHub.
- Require the version tag to match the clean source commit, and bump root/web/desktop to 0.2.3. The release includes existing main fixes from PRs #20–22.

## How to Test

Run deploy/portable/test/oss-release.test.ps1 and full-release.test.ps1 under PowerShell 7 and Windows PowerShell 5.1. Tests use local fake OSS and isolated fake lifecycle, never company services. Evidence is under evidence/clawsta-121.

Historical real v0.2.2 uploads succeeded (delta, company kit and full ZIP); no old object is overwritten. v0.2.3 source build/upload will follow the merged commit and new tag. Company execution is documented separately from local transport tests. Account-policy changes are out of scope by Tane's decision.

## Checklist

- [x] Full versus differential behavior and recovery paths documented
- [x] Targeted tests added and run locally (see evidence)
- [x] No deployment credentials, data or service operations included
- [x] User confirmed commit identity; configured only in this worktree
- [ ] Authorized merge and tagged release complete
- [ ] Tagged v0.2.3 source-built ZIP uploaded; latest updated last

## AI Disclosure

**AI tool used:** Codex in Multica.

**Prompt / approach:** Follow CLAWSTA-121's approved publishing and full-baseline requirements; reuse existing native build/upgrade lifecycle, preserve strict delta checks and verify packages before publication. Merge and release are explicitly authorized in the latest issue instruction.
