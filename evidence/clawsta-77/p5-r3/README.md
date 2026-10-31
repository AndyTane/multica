# CLAWSTA-77 P5-R3: allowlist must not bypass the closed registration switch

Baseline `8b15524e8` (PR #7, base `8c61cdef`). Backend-only fix; not merged, not
deployed. The live desktop instance stays on release
`v0.2.0-20261031111027` and was only read (deployment.json + instance switch
snapshot), never written.

## Finding

`Handler.instanceSignupPolicy` evaluated the email/domain allowlists first and
returned `nil` on a match, before `ALLOW_SIGNUP` and the persistent instance
`open_registration` switch. A deployment with any allowlist configured
therefore let a visitor self-register a matching address even when open
registration was off (and even with `ALLOW_SIGNUP=false`). The public page could
show "closed" while `POST /api/register/open` kept creating accounts.

## Fix

The single policy every enabled account-creation entry point uses now evaluates,
in order: the deployment-level `ALLOW_SIGNUP` hard switch, the instance
open-registration switch (default off), and only then the allowlists — which can
narrow the admitted set but never open a closed instance or replace the switch.
Existing sign-in and explicit administrator invitation redemption stay outside
this path. Tests updated to pin the corrected semantics.

## Verification actually run

- `go build ./...`, `go vet ./...` pass.
- Isolated PostgreSQL (`multica_p5candidate` on the desktop host PG through a
  temporary SSH tunnel, migrations through 582; live database never opened for
  writes): **26 handler tests passed / 0 failed** — the new
  `TestP5R3AllowlistCannotBypassClosedRegistration` (closed + matching
  allowlist => 403 `registration_disabled` and no user/credential rows; open +
  matching => registers; open + non-matching => 403 `email_not_allowed`),
  `TestP5R3InvitationRedemptionStaysAvailable`, plus all P5 / P5-R1 / P5-R2
  regressions and the workspace-deletion manifest test.
- `internal/auth`, `internal/middleware`, `internal/realtime` pass.
- Live deployment read-only check: release unchanged; instance switch read; no
  production writes.
- Full handler/views suites still have pre-existing environment failures (see
  `evidence/clawsta-77/p5-r2/full-handler-failures.txt`); they are not claimed
  green and need a CI/Linux run before merge.

## Not done

No merge, no deployment/HTTPS switch (authorized deployment batch after
acceptance). Front-end non-blocking suggestions (Switch contrast/size,
ja/ko/fr native review) remain follow-ups. No UI changed in this batch, so no
new screenshots.
