# Task Client Trust Implementation Plan

**Goal:** Let task-scoped CLI subprocesses use the daemon's selected public CA without discovering the owner's login profile.

**Architecture:** Keep the existing product environment filter. Explicitly inject the selected CA and seed the existing task-local default CLI configuration with only `root_ca_file`. Keep TLS verification and task-token requirements unchanged.

**Tech Stack:** Go, existing CLI/daemon packages, `httptest`, Windows PowerShell 5.1 and PowerShell 7.

## Design decisions

- Passing only an environment variable fixes ordinary subprocesses but loses trust when a shell removes that variable. Use both the task environment and the existing task-private config lookup.
- Do not forward the owner's profile or relax the product environment filter. Task config is refreshed at dispatch with public trust only, clearing stale trust on reuse; credentials remain in task environment.
- Resolve relative CA paths against the daemon's working directory before changing child working directories. An invalid CA or failed task-config write prevents task startup.
- Existing environment alias precedence stays current `U_WORKERS_` over legacy `MULTICA_`; explicit CA environment beats profile CA. Blocked `custom_env` keys cannot replace the selected CA.
- A CLI that loses task identity/root continues to fail closed. Bootstrap must not read the owner's trust profile when a task marker survives without a task config root. This change is not OS-level file isolation.

## Implementation and verification

1. Reproduce missing CA in `server/internal/daemon/task_client_trust_test.go` with `go test ./internal/daemon -run TestTaskMulticaEnvironmentIncludesClientTrust -count=1`.
2. Add task trust preparation, atomic public-only config writes, and explicit environment injection in `server/internal/daemon/`. Cover refresh, relative paths, invalid files, write failures, and blocked custom overrides.
3. Guard profile trust lookup in `server/cmd/multica/main.go` when task context lacks its private config root.
4. Use an isolated private-CA HTTPS fixture, synthetic task/owner identities, and freshly built CLI aliases. Launch through a fake provider using the real agent backend environment merge; no real model executable or account is accessed. Cover CA source/precedence, missing/wrong/malformed CA, shell CA loss, total identity loss, HTTPS writes, and unchanged synthetic owner files. Exercise Windows PS5.1/7.
5. Run affected daemon, CLI, TLS, environment alias and agent filtering regressions plus vet. Record commands, baseline failure and final results in a concise verification document. Open one candidate PR against current main; no merge, release or installed-program changes.

## Scope

Credential replacement and historical message/session cleanup are excluded by CLAWSTA-146. Production services, profiles, model calls and system trust remain outside this implementation and its tests.
