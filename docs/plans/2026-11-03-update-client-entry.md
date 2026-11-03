# Windows update and client entry implementation plan

**Goal:** Implement the approved CLAWSTA-124 R1 contract, delivering a PR and an external bootstrap without merging or publishing.

**Architecture:** Keep Fetch/Apply as the release engine. Add a Chinese Update coordinator for installation selection, configuration discovery, verified download, read-only preflight and explicit confirmation. Keep client lifecycle operations in client.ps1 and validate public settings before atomic import.

**Tech Stack:** Windows PowerShell 5.1 / PowerShell 7, cmd launchers, existing JSON inventories and isolated C# CLI/OSS doubles.

### 1. Server update coordinator

- Add deploy/portable/Update.cmd and Update.ps1; reuse Fetch-Upgrade.ps1, Apply-Upgrade.ps1 and oss-release-common.ps1.
- Read only deployment version/origin and install-report profile/listener settings. Reject conflicts and unsafe paths before lifecycle operations.
- Download outside the installation, validate the chain, run CheckOnly, check the installation daemon, and require confirmation. Report backup/recovery locations on failure.
- Preserve listener settings and check readyz after restoring the original listener.

### 2. Client settings and actions

- Extend deploy/client-kit/client.ps1 and add Import.cmd. Prompt only with explicit interactive mode; preserve automated noninteractive use.
- Validate HTTPS origins, profiles, certificate contents and path ancestry. Import only allowlisted settings and referenced public certificates; atomically replace connection.json after validation and confirmation.
- Keep reconnect login reuse and environment isolation. Treat stopped as normal; distinguish starting, local running and authenticated server access.

### 3. Packaging and documentation

- Update build-package.ps1, build-fetch-bootstrap.ps1, build-full-release.ps1 and release allowlists. Version the client inventory schema so old inventories remain accepted.
- Include the new coordinator in server packages, delta plans and full-package download tools. Explain the external bootstrap required for v0.2.5.
- Replace verbose upgrade/client instructions with short Chinese action guides and retain detailed recovery instructions.

### 4. Verification and handoff

- Run client.test.ps1, oss-release.test.ps1, full-release.test.ps1 and new entry regression tests under both PowerShell engines in isolated directories.
- Cover cancellation, bad inputs, hashes/download failures, active daemon, backup/upgrade failures, configuration preservation, certificates/import, launchers and legacy inventories.
- Build and inspect a bootstrap artifact with SHA-256. Use available real components only for isolated read-only checks; identify company acceptance still required.
- Run git diff --check, commit using Tane's identity, push and open a CLAWSTA-124 PR. Return base..head, evidence and bootstrap in the original thread; keep the overall issue in_progress.
