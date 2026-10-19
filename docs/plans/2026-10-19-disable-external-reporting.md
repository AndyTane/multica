# Disable External Reporting Implementation Plan

**Goal:** Prevent this local fork from sending product analytics, account attribution, exception reports, or daily deployment snapshots, even when environment variables accidentally enable upstream reporting.

**Architecture:** Compile-time privacy gates at the Go application factories, public configuration boundary, and shared browser analytics entry points. Preserve API response shapes and local diagnostics. Hide the source-attribution prompt. Native Windows startup and builds also force existing opt-out settings. Model providers, explicitly configured integrations, LAN access, and existing DSH/ClawStation tunnels are outside this change.

**Tech Stack:** Go, TypeScript/React, Vitest, PowerShell, native Windows deployment.

## 1. Regressions before behavior changes

- Add `server/internal/analytics/privacy_test.go`: configure a fake collector and valid-looking key with opt-out unset/false; require `NewFromEnv` to remain a no-op and deliver no requests.
- Extend `server/internal/selfhosttelemetry/event_test.go`: all environment values disable snapshots; even `New(..., Config{Enabled: true}, ...)` returns no worker.
- Add `packages/core/analytics/privacy.test.ts`: real build policy must reject a supplied key, identity, events, exceptions, person properties, and signup attribution without initializing or calling the SDK.
- Add `packages/views/onboarding/source-backfill-privacy.test.tsx`: default build renders no survey.
- Extend `scripts/native-windows.test.ps1`: imported/inherited reporting settings cannot re-enable native reporting.
- Run the narrow tests; observe failures caused by the missing privacy gates.

## 2. Close application reporting entry points

- Add `server/internal/privacy/reporting.go` and `packages/core/analytics/privacy.ts` with explicit disabled build policy, no environment override.
- Apply it in `server/internal/analytics/client.go`, `server/internal/selfhosttelemetry/config.go`, `server/internal/selfhosttelemetry/worker.go`, and `server/internal/handler/config.go`.
- Gate every emitting/buffering entry point in `packages/core/analytics/index.ts`, including attribution cookies. Keep local diagnostics and callbacks' existing disabled-mode semantics.
- Gate `packages/views/onboarding/source-backfill-modal.tsx` before mounting hooks or querying the server.
- Disable optional landing-page star/release probes and runtime official-update UI through the shared privacy policy. Test their real disabled defaults and keep protocol fixtures test-only.
- Require explicit credentials for `server/cmd/server/health_realtime.go`; disable the endpoint when no token is configured. Test TCP-relayed loopback requests using the standalone handler/test files, avoiding the package's DB-backed TestMain skip.
- Keep upstream protocol tests isolated behind test-only mocks/explicit fake-worker construction. Assert empty analytics fields in config tests.

## 3. Persist deployment protections and documentation

- Update `scripts/native-windows.ps1` to override imported PostHog/analytics settings, force Next.js telemetry off after overrides, and disable telemetry during production builds while restoring the parent environment.
- Update `docs/windows-native.md`, `.env.example`, and the relevant repository instruction: enabling reporting requires a reviewed source change.
- Keep private deployment credentials outside Git; inspect only selected non-secret settings.

## 4. Verify and deliver

- Run Go analytics/self-host telemetry tests, frontend analytics/onboarding tests, native script tests, scoped lint, and frontend typecheck. Run config-handler tests when the managed test database is available; report any actual skip.
- Review outbound HTTP call sites and the deployed configuration; record which external features are configured without printing secrets.
- Build a native release only after checks pass. Preserve the previous release and existing database; do not cancel or interrupt active tasks. Switch the API/web deployment only after the task queue is idle, using its lifecycle script.
- Verify API readiness, empty public analytics config, authenticated data access/realtime, and the published disabled survey/reporting code. No real model or telemetry calls are permitted for verification.
