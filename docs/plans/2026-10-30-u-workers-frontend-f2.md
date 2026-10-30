# U Workers frontend F2 implementation plan

**Goal:** Complete the approved F2 task book on PR #2 without changing backend, deployment, credentials, or mobile code.

**Architecture:** Keep pure brand/docs constants in core, the prop-driven product mark in ui, shared business screens in views, and route/Electron wiring in each application. Continue the existing branch and push each atomic commit.

**Tech Stack:** React, Next.js, Electron, Vitest, existing Chromium/Playwright and local fonts.

1. Remove the obsolete release-fetch test. The release-only marketing chain is removed in step 4.
2. Add `core/config/brand.ts` and `ui/components/common/product-mark.tsx`; wire metadata, titles, application icons, and five locales. Use monochrome UW geometry, distinct 16/24/32 optical paths and local Inter semibold. Isolate Electron userData, desktop configuration/preferences, and daemon profile names; preserve the CLI protocol and literal command identity. Verify profile isolation with fake paths.
3. Check the existing server auth contract, keep email login always available and render Google only when configured, preserve CLI callbacks. Remove platform/cloud fork and attribution/backfill UI/calls/exports/tests. Keep service, workspace, runtime/model, task progression.
4. First add a surface regression test. Remove billing/cloud/promotional navigation, settings, search entries and marketing-only routes/modules. Route legacy billing/cloud links to a shared local unavailable screen with a focused return button. Add one configured-app docs helper and use it for help, integrations, runtimes, skills, agents, issue templates and workspace sharing. Preserve channels, integrations, usage and model selection.
5. Document mobile release exclusion after reading its instructions and tracing root/release commands. Do not modify mobile.
6. Run the four complete package suites, root typecheck/lint and diff check. Reproduce any claimed environmental failures on the original base using an isolated checkout. Capture real components in light/dark at 1440/390 widths, mark sizes and old billing route. Attach complete test output, screenshots, and both line-classified grep audits to the final issue reply; update Draft PR without merging.

**Boundaries:** No `server/**`, `AGENTS.md`, `packages/core/analytics/privacy.ts`, or `docs/outbound-data-policy.md` changes. No `.env` access, real model calls, deployment/database access, or automatic migration. Test doubles only. The supplied task book is the design approval; routine implementation choices proceed without another approval round.
