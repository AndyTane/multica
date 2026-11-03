# Client-Kit Implementation Plan

**Goal:** Ship a versioned Windows client alongside the portable server, with a double-click reconnect entry.

**Architecture:** Reuse the existing CLI `connect` and profile-owned daemon lifecycle. Build the client from the same freshly compiled CLI as the server package; publish its ZIP before release metadata and the stable pointer. Keep machine-specific URLs and profile selection in a local JSON file, credentials in the existing CLI profile.

**Tech Stack:** PowerShell 5.1/7, Windows batch launchers, existing Go CLI, OSS release scripts.

1. Investigate the existing kit and CLI contracts. Local common folders and source history currently contain no kit; the user has been asked for its location. Avoid assuming its credentials or profile.
2. Add `deploy/client-kit/client.ps1`, configuration template, launchers and concise instructions. Reconnect checks profile status before starting; never restart a running daemon. Verify server access separately from local liveness.
3. Add `deploy/portable/build-client-kit.ps1`; inventory and validate only explicit program files. Extend release metadata and publisher upload order. Add `Fetch-Upgrade.ps1 -ClientKit` for standalone download and verification, with no installation side effects.
4. Verify stopped/running/starting/error/profile and server failure paths using a fake CLI, and exercise client publishing/download/hash failures using fake OSS under PowerShell 5.1 and 7. Run existing release suites.
5. Bump package versions to 0.2.5, review the diff, open and merge the PR using Tane's approved identity, tag the merged commit, rebuild and publish a delta from the original v0.2.3 package plus the client ZIP. Update latest last. Deliver immutable OSS paths, checksums and transferable usage instructions.

Manual reconnect is the default. Recommend an optional per-user Startup shortcut only after the user has connected successfully; no automatic startup registration during setup or release.
