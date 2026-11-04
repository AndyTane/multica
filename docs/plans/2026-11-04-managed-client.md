# Managed Client-Kit Implementation Plan

**Goal:** After a successful server upgrade, managed Windows kits follow that server when idle and recover automatically if the replacement cannot start.

**Architecture:** Keep the existing authenticated heartbeat, add an optional installed-version package offer, and serve an immutable verified local cache through the daemon API. A managed kit has a stable launcher and separate connection data; complete program versions live in version directories. The daemon holds the existing update CAS and claim barrier before handing off to an independent copy of the previous CLI. The helper journals switching, checks successor liveness, and requires a successful authenticated heartbeat with the offered version before completion. Local liveness without network confirmation stays pending; process startup failure restores the old pointer and process.

**Tech Stack:** Go, existing Chi daemon authentication and TLS transport, PowerShell 5.1/7 launch and release tools.

## Decisions

- Reuse heartbeats rather than the short-lived manual UpdateStore, so busy/offline machines do not expire.
- Use version directories rather than replacing an executing Windows executable or its alias. Both aliases and scripts switch together; credentials and model data are never extracted over user data.
- A stable installation identifier and profile identify a managed kit. General CLI, Desktop, and development versions do not opt in. The explicit R3 authorization is the exception to the repository's manual-only update rule.

## Implementation and checks

1. Add shared strict release/archive validation and cache metadata. Test version/platform/hash/size, traversal, unknown files and credential-bearing templates.
2. Add authenticated runtime-scoped cache download and additive heartbeat offer; bind it to the running server version/commit and activated cache. Test authorization and rollback/version mismatch.
3. Add persistent kit journal, bounded retry, waiting/paused states and exclusive claim draining. Add independent helper with old/new version directories, handshake confirmation and restart recovery. Test claim races, dedup and retry bounds.
4. Wire stable Client-Kit launchers and release-cache preparation/activation into Update/Apply/install; verify affected scripts under PS5.1/7 and document the one-time kit adoption.
5. Build distinct isolated test versions and a real local service, use two separate profiles and a controlled runtime. Exercise busy, offline, corrupt download and invalid executable recovery. Preserve identity/config hashes and collect sanitized evidence. Stop all test-owned services before delivery.
6. Review diff, run scoped checks, push a CLAWSTA-124 PR without merging/tagging/publishing, and attach evidence in the triggering thread. Keep issue in_progress for owner acceptance and release.
