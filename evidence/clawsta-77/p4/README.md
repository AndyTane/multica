# CLAWSTA-77 P4: test-data cleanup and PR #6 merge

Sanitized record of the desktop deployment cleanup and the merge of the accepted
password-login work. No email address, login name, password, token, cookie,
session id, connection string or private UUID is present here; the private
deletion plan and the pre-deletion backup stay on the host only.

## Deletion scope (authorized by the review comment and the P4 task)

Deleted from `D:\UWorkers\clawsta80\native` (the only deployment this batch
touches):

- the acceptance test identity and its `c1-cross-machine` workspace with its 4
  test issues, 10 activity rows, 3 attachment objects (6 upload files including
  metadata), and the workspace member row;
- the mis-created P3 identity and its `uworkers` workspace with its 1 starter
  issue and 1 activity row;
- account-level residue attributable to those two identities: 1 stale
  verification-code row (from the pre-P3 attempt, not the requested identity),
  6 client-usage rows and 5 issue-subscriber rows;
- all temporary credentials created for the deletion API calls.

Preserved and re-verified: the requested identity, its `uworkers-tane`
workspace with its starter issue and activity row, its membership and its
credential state (0 — the formal password was not set or changed by this batch),
the desktop login entry pinned to that identity, and the HTTPS entry.

## How it was done

1. Restricted backup first: `native\backups\native-20261031-130918` (earlier
   backups kept).
2. A fixed private plan (host-only) listed the exact user/workspace UUIDs,
   expected row counts and upload paths, and asserted that the protected
   identity does not intersect the delete targets.
3. The two workspaces were deleted through the application's own owner-gated
   workspace deletion endpoint (`DELETE /api/workspaces/{id}`, 204 each) with
   temporary credentials passed through encrypted SSH stdin — not by raw SQL
   against `workspace`, so the application teardown plan handled tasks,
   comments, activity, members and related rows.
4. Account-level residue was removed in one transaction with explicit UUID
   predicates and post-deletion assertions (including a dynamic residual
   reference check over every FK/column that references `user` or
   `workspace`).
5. The exact upload directory for the deleted workspace was validated to be
   inside `native\uploads` before removal (6 files, now 0).
6. No reinstall, rebuild, restart or configuration change was made.

## Before / after

| Check | Before | After |
| --- | --- | --- |
| Users | 3 | 1 |
| Workspaces | 3 | 1 |
| Members | 3 | 1 |
| Local password credentials | 0 | 0 (not set) |
| Attachments | 3 | 0 |
| Verification codes | 1 | 0 |
| Client-usage rows for deleted identities | 6 | 0 |
| Issue-subscriber rows for deleted identities | 5 | 0 |
| Upload files | 6 | 0 |
| Requested identity | present | present, email hash matches the supplied fingerprint |
| Requested workspace `uworkers-tane` | present | present, 1 starter issue, 1 activity row |
| Residual reference check | — | pass |

Service re-check after cleanup: HTTPS health 200, login page 200, anonymous
`/api/me` 401, password login enabled, email-code login disabled, `APP_ENV=production`,
dev verification code not configured, trusted proxies loopback-only, task
`Running` with `PT0S`. The desktop entry is still pinned to the requested
identity and the shortcut is present.

## Merge

PR #6 (`de719db86`) was marked ready and merged into `main` as a merge commit
after this evidence commit; PR #5 (superseded P1 snapshot) was closed with a
note. The merge commit SHA and the resulting `main` head are reported in the
CLAWSTA-77 delivery comment and the PR timeline. The running deployment was not
rebuilt or restarted: the deployed application commit stays `065e20ad0`
(release `v0.2.0-20261031111027`), while the merge only advances the repository
history.

## Not in this batch

Web registration / first-time password setup on the page is a separate PR and
was not implemented here; the signup scope (invite-only vs open LAN registration)
is still the user's decision, with invite-only recommended because it needs no
mail delivery.
