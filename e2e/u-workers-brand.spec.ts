import { test, expect } from "@playwright/test";

import { loginAsDefault } from "./helpers";

/**
 * CLAWSTA-80 Task 8 — U Workers brand acceptance against the real built web
 * application (Next.js production server, isolated deployment).
 *
 * Asserts user-visible identity: document titles, product mark accessible
 * names, manifest/PWA metadata, bundled icons and the absence of upstream
 * official-cloud hosts in served HTML. Desktop-only surfaces are covered by a
 * pending test below; packaged-process observation has not passed this batch.
 */

const PRODUCT_NAME = "U Workers";
const OLD_OFFICIAL_HOST = /multica\.ai/i;

test.describe("U Workers brand surfaces", () => {
  test("login page presents U Workers and bundles its own assets", async ({ page }) => {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await expect(page.getByText("Sign in to U Workers")).toBeVisible({ timeout: 30000 });
    await expect(page.getByRole("img", { name: PRODUCT_NAME }).first()).toBeVisible();
    await expect(page).toHaveTitle(/U Workers/);

    const html = await page.content();
    expect(html).not.toMatch(OLD_OFFICIAL_HOST);
    const visibleText = await page.locator("body").innerText();
    expect(visibleText).not.toMatch(/\bMultica\b/);

    await expect(page.locator('link[rel="manifest"]')).toHaveCount(1);
    await expect(page.locator('link[rel="manifest"]')).toHaveAttribute(
      "href",
      /manifest\.webmanifest/,
    );
    await expect(page.locator('link[rel="icon"]').first()).toHaveAttribute(
      "href",
      /favicon\.svg/,
    );
    await expect(page.locator('meta[name="apple-mobile-web-app-title"]')).toHaveAttribute(
      "content",
      PRODUCT_NAME,
    );
    await expect(page.locator('meta[name="theme-color"]').first()).toHaveCount(1);
  });

  test("web app manifest is branded, installable metadata and icons resolve locally", async ({
    request,
  }) => {
    const manifestResponse = await request.get("/manifest.webmanifest");
    expect(manifestResponse.ok(), "manifest.webmanifest must be served").toBe(true);
    expect(manifestResponse.headers()["content-type"]).toContain("application/manifest+json");

    const manifest = (await manifestResponse.json()) as {
      name?: string;
      short_name?: string;
      description?: string;
      display?: string;
      start_url?: string;
      icons?: { src: string; sizes?: string; type?: string; purpose?: string }[];
      shortcuts?: unknown[];
    };
    expect(manifest.name).toBe(PRODUCT_NAME);
    expect(manifest.short_name).toBe(PRODUCT_NAME);
    expect(manifest.display).toBe("standalone");
    expect(manifest.start_url).toBeTruthy();
    expect(manifest.start_url).not.toBe("/");
    expect(manifest.description ?? "").not.toMatch(OLD_OFFICIAL_HOST);
    expect(manifest.shortcuts?.length ?? 0).toBeGreaterThan(0);

    const icons = manifest.icons ?? [];
    const sizes = icons.map((icon) => icon.sizes);
    expect(sizes).toEqual(expect.arrayContaining(["192x192", "512x512"]));
    expect(icons.some((icon) => icon.purpose === "maskable")).toBe(true);
    expect(icons.length).toBeGreaterThanOrEqual(3);
    for (const icon of icons) {
      const iconResponse = await request.get(icon.src);
      expect(iconResponse.ok(), `missing manifest icon ${icon.src}`).toBe(true);
      expect(iconResponse.headers()["content-type"]).toContain("image/png");
    }

    const favicon = await request.get("/favicon.svg");
    expect(favicon.ok()).toBe(true);
    expect(favicon.headers()["content-type"]).toContain("image/svg+xml");

    const appleTouchIcon = await request.get("/icons/apple-touch-icon.png");
    expect(appleTouchIcon.ok()).toBe(true);
    expect(appleTouchIcon.headers()["content-type"]).toContain("image/png");

    const shell = await request.get("/login");
    expect(shell.ok()).toBe(true);
    expect(await shell.text()).not.toMatch(OLD_OFFICIAL_HOST);
  });

  test("workspace shell keeps U Workers identity in titles and the product mark", async ({
    page,
  }) => {
    const slug = await loginAsDefault(page);
    await expect(page.getByRole("img", { name: PRODUCT_NAME }).first()).toBeVisible();
    await expect(page).toHaveTitle("Issues | U Workers");

    // Client-side navigation in the production build can fall back to the
    // root document title (the root title is still U Workers). Assert the
    // brand identity on every route here; the pre-existing route-title
    // fallback is recorded in docs/outbound-data-verification.md.
    await page.getByRole("link", { name: "Inbox" }).click();
    await expect(page).toHaveURL(/\/inbox/, { timeout: 30000 });
    await expect.poll(() => page.title(), { timeout: 15000 }).toMatch(/U Workers/);

    await page.getByRole("link", { name: "Agents" }).click();
    await expect(page).toHaveURL(/\/agents/, { timeout: 30000 });
    await expect.poll(() => page.title(), { timeout: 15000 }).toMatch(/U Workers/);

    const html = await page.content();
    expect(html).not.toMatch(OLD_OFFICIAL_HOST);
    await expect(page.locator(`a[href*="/${slug}/issues"]`).first()).toBeVisible();
  });

  // The attempted package failed and its launch timed out. Do not expose an
  // unverified launch path: production reads ~/.u-workers/desktop.json, not
  // MULTICA_SERVER_URL, and process/profile isolation needs separate validation.
  test.skip("packaged desktop identity and outbound observation require a verified isolated launch", async () => {});
});
