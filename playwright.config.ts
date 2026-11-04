import "./e2e/env";
import { defineConfig } from "@playwright/test";

// e2e/visual captures the redesign's baseline screenshots (see
// docs/redesign/feature-baseline.md). It needs a running local web + API and
// writes ~100 PNGs, so it stays out of the ordinary suite:
// only `VISUAL_BASELINE=1 pnpm exec playwright test` collects it.
const visualBaselineEnabled = process.env.VISUAL_BASELINE === "1";

export default defineConfig({
  testDir: "./e2e",
  // The performance scenario has its own config: one worker, no retries, and a
  // running production build. It must not be swept up by the ordinary suite.
  testIgnore: visualBaselineEnabled ? "**/perf/**" : ["**/perf/**", "**/visual/**"],
  timeout: 60000,
  workers: 1,
  retries: 0,
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? process.env.FRONTEND_ORIGIN ?? "http://localhost:3000",
    headless: true,
  },
  projects: [
    {
      name: "chromium",
      use: { browserName: "chromium" },
    },
  ],
  // Don't auto-start servers — they must be running already
  // This avoids complexity and port conflicts during testing
});
