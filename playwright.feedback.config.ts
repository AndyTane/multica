import "./e2e/env";
import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e-visual",
  testMatch: "clawsta-177.spec.ts",
  outputDir: "./e2e-visual/__out__/clawsta-177",
  workers: 1,
  retries: 0,
  use: {
    baseURL: process.env.FRONTEND_ORIGIN,
    browserName: "chromium",
    channel: process.platform === "win32" ? "msedge" : undefined,
    headless: true,
  },
});
