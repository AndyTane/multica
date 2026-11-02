# Daemon Feature Capabilities Implementation Plan

**Goal:** Allow the fork's v0.2.1-based daemon builds to use supported features while preserving checks for older daemons.

**Architecture:** Advertise separate quick-create, quick-create-fields and chat-project-context capabilities through the existing daemon capability header and runtime metadata. Prefer an explicit capability; retain the existing upstream version policy for daemons that do not advertise it. Lowering global floors would admit unsupported upstream releases; rewriting the reported version would break release identity.

**Tech Stack:** Go protocol/daemon/handler, shared TypeScript core, React views, Go tests and Vitest.

## Implementation

1. Add protocol constants in `server/pkg/protocol/messages.go`; advertise them for HTTP and WebSocket in `server/internal/daemon/client.go`. Pin both transports in `client_test.go`.
2. Add metadata-aware checks to `server/internal/handler/issue.go`, preserving the existing 422 response and upstream floors. Test v0.2.1 with and without each capability, malformed metadata, old upstream releases and existing development builds.
3. Extend `packages/core/runtimes/cli-version.ts` to accept advertised capabilities; wire quick-create and chat callers. Test the capability/version matrix in core and the submission/warning behavior in views.
4. Run focused Go and frontend tests, type checking and lint for affected packages; inspect the diff and open a PR to `main`. Do not merge or release.

## Audit

- Basic quick-create: upstream 0.2.21, hard gate in web/desktop and backend.
- Explicit quick-create priority/due date: upstream 0.4.3, hard gate in web/desktop and backend, including captured source-context creation.
- Chat project description/resources: upstream 0.4.10, frontend warning only.
- Local worktree: upstream 0.4.24 is display-only; already checks `local-worktree-v1`.
- Repo checkout authentication: upstream v0.4.30 appears only in diagnostics; authorization checks the actual task credential.
- Third-party agent CLI versions remain independent of the fork's version numbering.

Existing deployed v0.2.1 binaries do not advertise the new capabilities. Applying this change requires rebuilding the web/backend and daemon, then reconnecting the daemon so runtime metadata refreshes; this PR does not publish a release.

## Focused Verification

Run from `server/`:

```powershell
# Run the pure gate tests independently of the handler suite's database TestMain.
go test ./internal/handler/quick_create_version.go ./internal/handler/quick_create_capability_test.go -count=1 -v
go test ./internal/daemon -run '^(TestClient_IdentityHeaders_|TestDaemonFeatureCapabilitiesWebSocket)' -count=1
go test ./pkg/agent -run '^TestCheckMinCLIVersion' -count=1
go build ./internal/handler
```

Run from the repository root:

```powershell
pnpm --filter @multica/core test -- runtimes/cli-version.test.ts
pnpm --filter @multica/views test -- modals/quick-create-issue.test.tsx chat/components/use-chat-project-context-support.test.tsx
pnpm --filter @multica/core typecheck
pnpm --filter @multica/views typecheck
```
