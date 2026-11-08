import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e-visual",
  testMatch: "clawsta154-port.spec.ts",
  timeout: 180_000,
  workers: 1,
  retries: 0,
  reporter: "list",
  use: { baseURL: "http://localhost:13004", headless: true, browserName: "chromium" },
});
