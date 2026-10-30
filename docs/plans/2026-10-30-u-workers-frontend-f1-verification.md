# Frontend F1 implementation and verification

Scope: Task 2 frontend and Task 6 frontend from the frontend local-only plan. The existing reporting gates stay closed. This batch does not change backend services, models, OAuth, MCP, Git, or general external-link handling.

## Configuration

- Packaged Desktop requires its runtime JSON. Missing configuration returns `ok: false`, names the actual path, and shows a complete localhost example. Main skips daemon initialization in that state; the renderer does not mount `CoreProvider`. Development localhost defaults remain.
- Web `/` redirects to the recent workspace when a session and workspace cookie exist, otherwise to `/login`. Existing runtime API, authentication, uploads, WebSocket and docs proxies still accept configured remote HTTP(S) services.
- `NEXT_PUBLIC_APP_URL` opts into public canonical, Open Graph URL and discovery addresses. Without it, the root layout omits those addresses, robots omits sitemap discovery, and sitemap returns no entries. `runtime-urls.ts` and `next.config.ts` already had strict production resolvers and needed no change.
- `U_WORKERS_UPDATE_URL` optionally supplies an explicit generic Desktop update feed. No feed means a manual check returns `No update source configured.` without contacting a release service. There are no startup or interval checks; neither downloading nor installing on quit is automatic. A download requires a successful manual check and a native confirmation displaying the feed address. Installation remains a separate explicit action.
- CLI resolution prefers `U_WORKERS_CLI_PATH` when supplied, then bundled/local managed binaries and PATH. A bad explicit path fails locally rather than silently substituting another binary. There is no CLI download implementation in bootstrap. Old automatic-update preferences cannot reactivate checks.

## Resource trigger inventory

| Trigger | Previous path | F1 behavior |
| --- | --- | --- |
| Packaged boot without JSON | `main/runtime-config-loader.ts` returned upstream API/WS/app defaults | Configuration error; no daemon initialization or API provider |
| Desktop boot / hourly timer | `main/updater.ts` checked and downloaded releases | Timers removed; manual checks only |
| CLI missing or invalid during bootstrap | `main/cli-bootstrap.ts`, `main/daemon-manager.ts` fetched checksums and official release archives | Local resolution only; missing binary is actionable failure |
| Marketing page render/navigation | `landing/utils/github-release.ts`, `use-github-stars.ts` contained release/star fetch implementations behind an already-disabled privacy gate | Fetch implementations deleted; empty release and no star count |
| Workspace/member avatar render | `core/workspace/avatar-url.ts`; absolute `ActorAvatar` props bypassed this helper | Default Gravatar/Libravatar services omitted, including subdomains and protocol-relative URLs; explicit local uploads and other configured integration avatars remain |
| Markdown/Tiptap inline image render | `editor/attachment.tsx` mounted an image for arbitrary URLs | Unresolved external images show their destination and a load action; uploaded records, same-origin/API resources, blob/data URLs and configured storage retain normal behavior |
| Image viewer neighbour prefetch | `editor/attachment-preview-modal.tsx` prefetched URL-only neighbours | Prefetch limited to attachment records; explicit viewer navigation remains available |
| Readonly HTML fence becomes visible | `editor/html-block-preview.tsx` mounted a scripts-enabled sandboxed iframe | Load action and external-service notice precede iframe mounting; changed HTML requires a fresh action |
| Editable HTML fence mounts | `editor/extensions/code-block-view.tsx` defaulted to preview | Defaults to source; user selects preview; sandbox remains unchanged |
| Uploaded attachment preview/download | `editor/attachment-card.tsx`, `attachment-preview-modal.tsx` | Already opened by a user action; retained, including remote integration storage |
| Mermaid diagram | `editor/mermaid-diagram.tsx`, `mermaid-viewer.tsx` | Local rendering and strict sandbox retained |
| Web fonts | `app/layout.tsx` uses `next/font/google` | Build-time font acquisition; font assets are served locally at runtime, so no runtime Google Fonts link to remove |

The scan found no separate Gravatar generator or automatic third-party link-unfurl fetch in `packages/views/editor`. Local entity references, attachment resolution, sandboxed Mermaid, user-selected attachment viewers and configured storage are separate from arbitrary inline resources.

## Verification

All test data and updater/API responses are fake. No model, deployed service, database or `.env` file was used. Dependency installation used the existing lockfile with scripts disabled; Electron's native runtime is not required by the mocked main-process tests.

- Baseline regression proof: isolated copies of `b60cca871` failed the missing-config assertion (1 failure) and root-routing assertions (4 failures). The configured remote API/auth proxy regression passed on that baseline. The temporary baseline copies were removed after the run.
- Desktop focused suite covers configuration, error rendering, updater timing/manual confirmation, local CLI bootstrap and update settings/notification wiring.
- Web focused suite covers proxy, public host/discovery metadata, runtime URL resolvers, and no release/star requests.
- Core focused suite covers avatar filtering and retained upload/integration URLs.
- Views focused suite covers image opt-in, retained explicit file downloads, HTML opt-in, editor source/preview toggling, attachment viewer/sequence behavior, and translation parity.
- Root `pnpm typecheck` and `pnpm lint` are run as requested. Exact commands and final results are posted with the PR on CLAWSTA-78.
- The light configuration-error screenshot uses actual Desktop renderer source and styles in an isolated Chromium context. Network observation is scoped to that renderer fixture, not a production installation.

## Remaining batch boundaries

F2 owns product branding, independent userData/config/profile locations, login/onboarding, cloud/billing/help surfaces and mobile release exclusion. Remaining upstream strings are recorded line by line in the attached grep audit: test/internal/source references or explicit F2 surfaces. The old runtime update section is already unreachable behind the disabled reporting/background-request gate and remains part of the F2 surface cleanup.

Native packaged Electron launch/install, a real update-feed download, real CLI execution, real OAuth/model/integration traffic, and mobile device testing are outside this fake-service verification. The first batch does not claim whole-application or backend network acceptance; that remains the cross-process verification task in the parent plan.
