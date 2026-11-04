# Interface style switch implementation plan

**Goal:** Switch immediately between UWorkers and the original interface in shared Web/Desktop preferences, independently of light/dark/system.

**Architecture:** Scope every optional CSS rule to roots without `data-ui-theme="classic"`. A small shared preference row updates that attribute and `multica-ui-theme` storage. A parser-blocking head script restores classic before first paint. Desktop's static script is generated from the shared canonical source and checked for equality.

**Tech stack:** React, the existing SegmentedToggle/SettingsRow, Next.js, electron-vite, Vitest and Playwright.

1. Add the preference row and its focused tests (default, both transitions, persistence, locale, unavailable storage, color-mode independence).
2. Add shared bootstrap and generated Desktop asset; insert only two lines in Web layout, one Desktop head line and two preference lines. Scope all optional CSS without changing any declaration.
3. Run unchanged preferences tests, the 54 contrast assertions, typecheck and lint. Verify both production builds.
4. Restore the isolated F1 dataset. Capture four settings combinations and two classic boards at 1440×900. Compare the boards byte-for-byte with F1 main originals; record classic reload frames and prepaint timing.
5. Run unchanged settings E2E with the F1 command/config and compare failures. Record the upstream diff ledger and full base diff stat in PR #35; attach evidence and request frontend review without merging.
