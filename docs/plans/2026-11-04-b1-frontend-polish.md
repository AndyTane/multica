# B1 frontend polish implementation plan

**Goal:** Deliver CLAWSTA-128 as a reviewable PR to main, without merging or releasing.

**Architecture:** Keep locale selection in core, reuse it for web request signals, and keep web document titles in the web platform. Reuse board and sidebar components with semantic tokens. Preserve saved preferences and all creation actions.

**Tech stack:** TypeScript, React, Next.js, TanStack Query, Zustand, i18next, Vitest, Playwright.

1. Update the core locale matrix and web routing tests; default to zh-Hans without an explicit choice, including the emergency boundary. Preserve explicit cookie, desktop, and account choices.
2. Translate the join page through auth resources in all five locales. Preserve membership synchronization, admin-role disclosure, capacity recovery, and existing-member navigation. Extend component tests for Chinese and missing inviter/code.
3. Audit untranslated visible Chinese labels; change the brand title and cover a cold issue URL with real query cache behavior.
4. Add done to default hidden statuses without migrating persisted state. Narrow empty, settled board columns to 200px; retain 280px while loading or populated, and keep swimlanes unchanged. Use neutral column backgrounds.
5. Render empty wakeup/PR sections as title plus creation action; keep errors and populated behavior accessible. Cover empty and populated transitions.
6. Run pnpm typecheck, pnpm lint, pnpm test. Render the specified states against an isolated deployment and attach screenshots. Review the diff, commit named files, push and open the PR. Post one final issue comment and request frontend-owner review.

Validation uses a separate database, ports, and generated test identity; no running multica-fork data or services are used.
