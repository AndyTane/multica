# CLAWSTA-77 P3-R1: identity source correction and evidence correction

Sanitized results for the corrective batch. No email address, login name,
password, password hash, token, cookie value or session id is present in this
directory. The HTTPS lifecycle was already accepted in the previous batch and
was not modified or restarted again.

## What was wrong and what changed

1. **Identity source.** P3 bound the desktop login entry to an address taken from
   an old verification-code row instead of the explicit On Behalf Of identity of
   the request. The corrected identity's normalized (`trim` + `lowercase`,
   UTF-8) SHA-256 equals the fingerprint supplied by the reviewer; both hashes
   are printed and compared in `host/identity-assertions.json` and no email is
   printed anywhere. The correct identity now exists, completed the normal
   first-use onboarding to its own workspace `UWorkers`
   (`/uworkers-tane`; unique slug because the earlier workspace keeps
   `uworkers`), and the pinned entry points at it (user id + email re-checked).
2. **Old identity preserved.** The identity created in P3 is kept untouched for
   later ownership confirmation: still onboarded, still owner of `uworkers`, no
   credential added, nothing renamed, merged or deleted.
3. **Existing deployment untouched.** The acceptance account and the C1
   workspace are unchanged (1 member, 1 owner); no account was taken over and no
   display-name based merge happened.
4. **Evidence correction, failure history kept.** P3's
   `browser/onboarding-evidence.json` was `passed=false` with 15 passed steps
   and one failure: `identity_is_owner_of_its_own_workspace`. That was an
   assertion bug: the workspace list response carries only `slug`/`name`, no
   role field. P3's report claim of "16/16" was therefore wrong. This batch
   asserts ownership through the supported
   `GET /api/workspaces/{id}/members` endpoint (browser) and keeps a separate
   independent database assertion (`host/identity-assertions.json`), recorded
   separately instead of rewriting the old file.

## Browser verification of the corrected identity (real client, normal TLS)

16 steps passed: HTTPS chain validated against the exported root; login reaches
first-use onboarding; welcome to workspace step; workspace created with the
unique slug; runtime skipped (no agent or paid model started); onboarding
completes into the own workspace; owner role read from the members API; not a
member of C1; own board rendered; C1 API 404 and UI "工作区不可用"; refresh keeps
the session; logout returns `/api/me` 401; re-login returns to the own
workspace; email-code login still disabled; no uncaught page errors. Raw results
and screenshots are in `browser/`.

## Entry under the real desktop account

The pinned identity is checked against the database before any prompt:

| Test | Result |
| --- | --- |
| Healthy preflight then cancel | exit 0, no change |
| Empty login name | exit 5, no change |
| Invalid login name | exit 5, no change |
| Identity mismatch | exit 3, refused before any prompt |
| Existing credential present | exit 4, never overwritten |

A temporary random credential was created for the corrected identity through
encrypted SSH stdin, used only for the browser run, then deleted; the store is
empty again. The formal password remains for Tane to type privately on the
desktop.

## Files

| File | Content |
| --- | --- |
| `host/identity-assertions.json` | Hash comparison of requested vs pinned identity email, pinned user/membership/onboarding flags, old-identity preservation, acceptance/C1 invariants, credential count, entry ACL/shortcut state, HTTPS task readback (untouched) |
| `host/entry-tests-preflight.json` | Entry refusal/cancel tests under the real host account |
| `host/entry-tests-overwrite.json` | Existing credential is not overwritten |
| `host/temporary-credential-lifecycle.json` | Temporary credential creation/deletion record (no identifiers) |
| `browser/browser-evidence.json` + `*.png` | Corrected-identity onboarding, own workspace, owner assertion, C1 denial, refresh, logout, re-login |

## Remaining state

The pinned entry is ready. The only remaining step is Tane double-clicking
"设置 U Workers 登录" on the desktop, typing a login name and the formal password
twice, then signing in at `https://192.168.3.49:13443`. HTTPS settings were not
modified in this batch.
