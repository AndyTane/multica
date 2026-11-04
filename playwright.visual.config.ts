import "./e2e/env";
import { defineConfig } from "@playwright/test";

/**
 * R0 redesign visual baseline screenshots (CLAWSTA-133).
 *
 * The upstream playwright.config.ts is deliberately left untouched: its
 * testDir is ./e2e, so the ordinary suite never collects e2e-visual/. Run the
 * baseline screenshots with this config instead:
 *
 *     pnpm exec playwright test -c playwright.visual.config.ts
 *
 * Same rules as the main suite: no auto-started servers (the local web + api
 * must already be running), one worker, no retries.
 */
export default defineConfig({
  testDir: "./e2e-visual",
  // Playwright empties outputDir before a run; keep failure artifacts inside
  // the gitignored screenshot directory.
  outputDir: "./e2e-visual/__out__/.playwright",
  timeout: 60000,
  workers: 1,
  retries: 0,
  use: {
    baseURL:
      process.env.PLAYWRIGHT_BASE_URL ??
      process.env.FRONTEND_ORIGIN ??
      "http://localhost:3000",
    headless: true,
  },
  projects: [
    {
      name: "chromium",
      use: { browserName: "chromium" },
    },
  ],
});
