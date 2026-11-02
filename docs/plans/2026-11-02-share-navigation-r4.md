# Share and navigation separation implementation plan

**Goal:** Keep web navigation on the logged-in browser origin while copying the configured public frontend URL.

**Architecture:** Restore `getShareableUrl` to its existing navigation semantics. Add `getExternalShareUrl` for links explicitly copied or attached for others. Derive accepted internal origins from both adapter methods; retain the singular `useAppOrigin` API and add `useAppOrigins` for link recognition. Both desktop providers use their existing connected app URL for both methods.

**Tech stack:** TypeScript, React navigation adapters, Vitest, Playwright, isolated Windows native deployment.

The R4 taskbook and frontend review approve this design. Changing every navigation call to another method would leave unreviewed callers vulnerable; rewriting browser cookies across origins would expand authentication scope. Separating the copy method addresses the actual regression without either change.

## Steps

1. Add failing web provider regressions for configured versus current origins, actual `useRowLink` middle-click wiring, late configuration and preserved path/query/hash. Extend internal-link tests to accept two exact origins and reject a third or prefix impostor.
2. Add the explicit copy method to `navigation/types.ts` and the web, desktop main and dedicated issue-window providers. Keep server rendering relative. Update typed test adapters to implement the complete contract.
3. Migrate members, instance invitations, issue/comment copies, project copies, search copy and feedback links. Leave issue open actions, row/content/chat/attachment new-tab operations on `getShareableUrl`.
4. Add `useAppOrigins`; wire editor, rich content and hover-card parsing through both exact origins. Keep old string arguments and `useAppOrigin` compatible. Verify missing-provider and malformed/relative fallbacks.
5. Run focused views/web/desktop tests, frontend typechecks and changed-file lint. Build production web with the native script; stage only the existing isolated test deployment.
6. In Chromium, log in on loopback and Ctrl-click a real issue row. Assert the new page retains loopback, displays that issue and answers `/api/me` without login. Assert copied member and issue links still use the public test origin. Capture sanitized origin-and-content evidence.
7. Stop all isolated services, check four ports closed, document verification in `evidence/clawsta-118/r4/`, update tutorial and PR #22, append commits and hand back to the architect. Preserve R1/R2/R3 evidence and backend/install logic. Do not merge, release or touch company services.
