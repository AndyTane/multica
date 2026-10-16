# DSH Session Event API Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Keep browser-owned DSH conversations working without reading DSH's private session files, then deploy the verified bridge to the existing Windows installation.

**Architecture:** Authenticate against the existing loopback web host. Use its Remote WebSocket `session/follow` stream and bounded `session/page` requests for headers, history, execution events, and reconnect recovery. Preserve workspace ownership, request correlation, cancellation checks, and task credential isolation; do not resend uncertain prompts or start a second DSH process.

**Tech Stack:** Node.js 24 built-in fetch and WebSocket; Go embedded launcher; Node test runner with a fake HTTP/WebSocket DSH host.

---

### Task 1: Pin the service contract with regressions

Files: `server/internal/dshweb/bridge.test.mjs`, root `package.json` and lockfile (test-only WebSocket server).

1. Replace disk-log fixtures with a fake Remote host serving snapshots, paginated events, and live events. No DSH data directory exists in these fixtures.
2. Run `pnpm test:dsh-web` and confirm the file-based implementation fails the API-only regression.
3. Cover first send, resume, browser steering, tools, usage, retry deduplication, pagination, stream disconnect/gaps, authentication, unknown contracts, and cancellation races.

### Task 2: Replace private storage reads

Files: `server/internal/dshweb/bridge.mjs`.

1. Add authenticated Remote stream transport with bounded queues, sanitized errors, timeouts, and explicit close.
2. Validate opening headers and ordered event sequences; page backwards to a complete history cut and recover disconnected streams without replaying output or re-posting prompts.
3. Validate ownership before adopting existing sessions; correlate the original request with its turn, and recheck current ownership before cancellation.
4. Bind runtime-brief digests to API session identity and durable request witnesses instead of inode/size. Keep task credentials out of prompts and caches.
5. Make probe verify the read-only event API contract and bump the bridge patch version to 1.0.1.

### Task 3: Update configuration and checks

Files: `scripts/configure-dsh-web.ps1`, DSH setup documentation, launcher version assertion, root `AGENTS.md` when documenting the new boundary.

1. Remove the DSH data-directory requirement; existing config's extra keys are harmless.
2. Document event delivery, read-only probe, reconnect handling and unsupported-interface recovery.
3. Run `pnpm test:dsh-web`, `go test ./cmd/multica-dsh-web -count=1`, targeted DSH backend tests, matching `go vet`, and `git diff --check`.

### Task 4: Deploy and publish

1. Build a separate candidate bridge executable. Probe the real host without sending prompts and verify an existing authorized session's header/history through the API.
2. Check for active Multica runs/bridge processes. Back up and atomically replace only the idle bridge executable; preserve the running daemon, DSH host, database and existing sessions.
3. Verify installed version, content hash, probe, model discovery and Multica/LAN health. Roll back the executable if deployment verification fails.
4. Commit only this change and push the existing fork branch. The user's requested publication is this existing self-host deployment; do not trigger an upstream product-wide release tag. Report commit, deployed version and actual verification scope. Real model smoke tests require separate explicit authorization under repository rules.

## Verification

- `pnpm test:dsh-web`: 38 tests passed, including the standalone stdio entry against a fake HTTP/WebSocket host with no DSH session files.
- `go test ./cmd/multica-dsh-web -count=1`: passed.
- `go test ./pkg/agent -run 'Test(ParseDsh|BuildDsh|DshBackend|DiscoverDsh)' -count=1`: passed.
- `go vet ./cmd/multica-dsh-web ./pkg/agent`: passed.
- PowerShell parser check for `scripts/configure-dsh-web.ps1` and `git diff --check`: passed.
- Candidate `1.0.1`: version, authenticated event probe and model discovery passed against the existing local host. API history reads succeeded for five existing Multica sessions, including completed replies.
- No real model prompts were sent. Frontend/mobile suites were not run because their code and API response contracts are unchanged.
