# CLAWSTA-151 R1 implementation

Goal: implement the approved awaiting-you contract without a release or version change.

Architecture: preserve the shared web/desktop surfaces and TanStack Query ownership. A database trigger maintains review lifecycle across every status writer. The existing locked PUT gains a narrowly scoped atomic review restore.

1. Add migration 584, regenerate sqlc, update issue projections and response schemas. Verify backfill, status transitions, and unchanged timestamps.
2. Validate reserved review keys inside row locks. Extend UpdateIssue with revision-guarded review replacement. Test Unicode limits, tenant validation, stale writes, and atomic undo.
3. Add shared predicate, pagination query, navigation and awaiting page. Keep rejection comment state across retries; use trigger preview and revision checks. Add review badges to existing card/row layouts.
4. Cover predicates, pagination, badges, approval/undo and rejection failures. Run typecheck, lint, frontend tests and Go checks. Render isolated real-card data in both themes and widths; inspect screenshots.
5. Push agent/CLAWSTA-151-awaiting, open an unmerged PR to main, and return evidence to the requesting thread.
