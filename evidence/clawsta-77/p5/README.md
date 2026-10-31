# CLAWSTA-77 P5: web invitation registration, instance administration, password change (candidate)

Independent candidate branch `e94e6f3a3` from `main` (`8c61cdef`). The live
deployment was not replaced or restarted; this is the review candidate only.

## Verified in this batch

- Go `build ./...` and `go vet` on the touched packages pass.
- Handler tests pass against a real PostgreSQL database: single-use invitation
  redemption, JSON/Origin transport guards, the persistent open-registration
  switch (off by default, server-side rejection), password change revoking the
  previous session via `auth_version`, and the instance-admin guards.
- Web/core typecheck passes for the changed packages.
- Real browser acceptance against an isolated candidate instance with normal
  certificate validation (21/21 steps): index - instance admin capability;
  admin page; open registration defaults off and restores off; add pending user
  and receive a one-time invitation; fragment removed from the address bar;
  invited user sets password on the web and completes normal onboarding; the
  invited account is an ordinary user and cannot use instance administration
  (UI + API 403); password change in personal settings signs the old session
  out, rejects the old password and accepts the new one; admin enables open
  registration and a new user registers in the browser; after disabling it the
  public API rejects with `registration_disabled`; a user without workspace
  ownership can be deleted and the deleted user's live session is rejected.

Raw browser steps and sanitized screenshots are in `browser/`; the machine
readable candidate summary is `candidate-evidence.json`. No email, login name,
password, invitation secret, token or cookie appears in this directory.

## Not done / next

- Five-language localization of the new pages is still English-only.
- The desktop one-time bootstrap helper and the actual deployment switch remain
  part of the post-review deployment batch.
- The candidate is intentionally not deployed; PR merge and rollout follow the
  review, per the task plan.
