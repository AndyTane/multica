# Backend privacy validation — CLAWSTA-79

Scope: Task 1, server/core portions of Task 2, and Go/installer portions of
Task 6 in the 2026-10-30 plan. Base: `b60cca871f18ff93d2763941ee409187c3525f10`.
Environment: Windows, Go 1.26.9, repository pnpm workspace. Tests used fake
transports, local fake servers and test-created executables. No original
deployment, real model/agent account, recipient or database was used.

## Completed checks

From the repository root:

```sh
pnpm --filter @multica/core exec vitest run analytics/index.test.ts analytics/privacy.test.ts analytics/privacy-network.test.ts --maxWorkers=1 --pool=threads
pnpm --filter @multica/core typecheck
pnpm install --lockfile-only --frozen-lockfile --offline --ignore-scripts
```

Passed: three Vitest files / nine tests, core typecheck and frozen lockfile
validation. Targeted analytics ESLint also passed. The initial cold jsdom
load exceeded Vitest's startup limit; after warming the dependency, the same
tests passed without changing assertions or runner limits.

From `server/`:

```sh
go test ./internal/privacy ./pkg/llm -count=1
go test -p 2 ./internal/analytics ./internal/handler/configpolicy -count=1
go build -p 2 ./cmd/server ./cmd/multica
```

All passed. The model test runs the actual SDK with a fake transport to a
non-loopback configured URL: one retryable response followed by a successful
stream. It does not assert that authorized model traffic is zero.

Updater, local reload and CLI checks also passed from `server/` (with
`GOPROXY=off`, `GOSUMDB=off`, `GOMAXPROCS=2`):

```sh
go test -p 2 ./internal/cli/update.go ./internal/cli/update_windows.go ./internal/cli/update_test.go ./internal/cli/update_source_test.go -count=1 -v
go test -p 2 ./internal/cli ./cmd/multica -run 'Test(ReportingPolicyPreservesTaskToolsAndDownloads|ResolveLoginToken|TokenLoginWithoutServer|Setup|RunLoginToken|FetchReleaseByTag)' -count=1
go test -p 2 ./internal/daemon ./cmd/multica -run 'Test(AutoUpdateLoop|TrySelfReload|ParseSelfVersion|TryEnterClaim|RunUpdate|.*AutoUpdate|ResolveLoginToken|TokenLoginWithoutServer|Setup|RunLoginToken)' -count=1
```

From the repository root, PowerShell:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-backend-privacy.ps1
```

Passed router assembly, telemetry protocol, storage, cloudruntime, Composio
SDK/integration and email tests. The script explicitly selects fake-only tests
instead of package TestMain functions that open deployment databases.
Old cloud routes return local 410 responses; configured S3 still sends its
expected fake upload. Compose default, custom override and source-build
configurations passed read-only parsing assertions; no containers were started.

On Windows only, this script skips `TestLocalStorageUploadsAreWorldReadable`:
the original run reported mode `0666` against its POSIX expectation `0644`.
Production storage and the original permission assertion remain unchanged;
Linux permission behavior still requires Linux verification.

Installer fake-only checks from the repository root:

```sh
bash scripts/install-source.test.sh
bash -x scripts/install.test.sh
```

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/install-source.test.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/install.ps1.test.ps1
```

Passed. The source tests cover isolated installation targets, avoiding the
existing PATH CLI, local source builds without registry pulls, and explicit
custom image handling. No real installer was executed. The PowerShell suite
also checks the displayed setup command uses the published backend/frontend
ports and the isolated daemon profile.

The full Bash suite passed all 13 port-matrix cases and unavailable-port
handling when invoked directly from Git Bash. An earlier PowerShell-wrapped
Bash session stopped making progress and was cancelled; the final traced
direct-Bash run exited 0 with `install.sh tests passed`.

## Remaining acceptance

Task 8 owns isolated deployment and browser/Node/Go/Electron traffic observation,
including configured integration lifecycle, MCP/Git and offline management.
The complete `scripts/selfhost-config.test.sh` harness was not run here;
Compose default/build/custom configuration was checked directly instead.
This batch does not substitute unit tests for that acceptance, real-agent
authorization or platform release validation. No production installer or
release artifact was downloaded. Frontend/Desktop/mobile scope is owned by
the companion task. The branch is proposed for review, not merged or released.
