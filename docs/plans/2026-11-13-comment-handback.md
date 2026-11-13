# Comment handback implementation plan

**Goal:** A review receiver's new comment returns their awaiting card to in progress in both forks.

**Architecture:** Reuse the SQL awaiting predicate under the issue lock already acquired by comment creation. Commit comment, attachments and status together. Publish the normal issue update after commit for activity/realtime, and only dispatch through the comment route. Expose optional `issue_status` on the comment receipt for older-client/server compatibility.

**Tech stack:** Go/pgx/PostgreSQL, React/TanStack Query, Vitest/Playwright.

1. Add handler regressions for receiver/nonreceiver/agent/specialist, rollback and one dispatch. Create the comment and conditionally return the locked issue in one transaction; return the final revision and status.
2. Parse the optional receipt status. Skip the feedback dialog's fallback status request when the server already returned control; invalidate awaiting lists/counts after every successful comment.
3. Retain the open peek when a card leaves the list. Preserve its former navigation slot so previous/next use the remaining cards; update the count immediately. Cover removed first/middle/last/only cards.
4. Update runtime waiting instructions and component versions. Run affected Go tests, core/views unit tests, typechecks and lint in both checkouts.
5. Render the actual UI against isolated test data, verify a sidebar reply updates list/count/status while keeping the card open, and capture light/dark evidence. Push and open one PR per fork without merging.

Validation completed on Windows:

- Non-mobile `pnpm typecheck`: all 10 tasks pass in each checkout; changed frontend files pass ESLint.
- Core: 120 tests across receipt schemas, mutations, awaiting selection, revision ordering and comment realtime sync. Views: 111 tests across return controls, peek navigation and comment composers.
- Database-backed Go handler tests matching `Test(CreateComment|CommentMutationEvents|.*Review|.*Awaiting)` pass. Related runtime brief and server activity/projection tests pass.
- `E2E_HANDBACK_ISOLATED=1 pnpm exec playwright test e2e/comment-handback.spec.ts`: light and dark pass in each checkout. Verifies list removal, retained peek, reduced count, final issue revision, cleared review mark, one status activity and exactly one comment-triggered task.
- Test services used a separate native PostgreSQL cluster on 15498 with two empty databases, APIs on 18198/18199 and web on 13198/13199. Native Windows had no make/docker entry point; production services and data were untouched.
- A broader optional runtime selection exposed three existing Windows failures: OpenClaw CLI deadline budgets, Hermes relative external directories and Cursor POSIX file mode. The same three fail on unchanged UWorkers main `22cdfa5f4`.

Fork adaptation: Multica deliberately omits review migrations 584/585 and stores its review time in `metadata.review_since`. Its handback update explicitly removes `reviewKeys` and `review_since` in the comment transaction. UWorkers uses its existing `maintain_issue_review` trigger. Both use the same awaiting receiver predicate and comment dispatch path.
