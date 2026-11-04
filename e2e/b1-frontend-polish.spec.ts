import { test, expect } from "@playwright/test";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { execFileSync } from "node:child_process";
import { TestApiClient } from "./fixtures";

// This acceptance changes only generated fixtures in an explicitly isolated
// native deployment, never the default local application or its database.
const isolatedRoot = process.env.E2E_B1_ISOLATED_ROOT;
test.skip(!isolatedRoot, "Requires an isolated B1 deployment");
test.use({ locale: "en-US", viewport: { width: 1440, height: 900 } });

test("B1: Chinese first visit, join states, compact board and direct issue URL", async ({ browser, page, baseURL }, testInfo) => {
  test.setTimeout(180_000);
  const deployment = JSON.parse(readFileSync(join(isolatedRoot!, "deployment.json"), "utf8"));
  const database = new URL(process.env.DATABASE_URL!);
  expect(database.hostname).toBe("127.0.0.1");
  expect(database.port).toBe(String(deployment.DatabasePort));
  expect(database.pathname).toBe(`/${deployment.Database}`);
  expect(baseURL).toBe(`http://127.0.0.1:${deployment.WebPort}`);
  const evidence = process.env.E2E_B1_EVIDENCE_DIR ?? testInfo.outputPath("screenshots");
  mkdirSync(evidence, { recursive: true });
  const api = new TestApiClient();
  const suffix = Date.now().toString(36);
  let workspace: { id: string; slug: string } | undefined;
  try {
    await api.login(`b1-${suffix}@example.test`, "B1 验证");
    workspace = await api.ensureWorkspace("设计协作", `b1-${suffix}`);
    await api.markUserOnboarded();
    const backlog = await api.createIssue("梳理下一批需求", { status: "backlog" });
    const issue = await api.createIssue("完善工作区首次体验", { status: "todo" });
    await api.createIssue("核对中文文案", { status: "todo" });
    await api.createIssue("已完成的任务", { status: "done" });
    const headers = { Authorization: `Bearer ${api.getToken()}`, "X-Workspace-ID": workspace.id };
    const inviteResponse = await page.request.post(`${process.env.NEXT_PUBLIC_API_URL}/api/workspaces/${workspace.id}/share-links`, {
      headers, data: { role: "member" },
    });
    expect(inviteResponse.ok()).toBe(true);
    const invite = await inviteResponse.json();

    await page.goto("/join?code=invalid-b1-link");
    await expect(page.getByRole("heading", { name: "邀请链接无效" })).toBeVisible();
    await page.screenshot({ path: join(evidence, "03-join-invalid.png") });
    await page.goto(`/join?code=${invite.code}`);
    await expect(page.getByRole("heading", { name: "加入 设计协作" })).toBeVisible();
    await expect(page.getByRole("button", { name: "登录后加入" })).toBeVisible();
    await page.screenshot({ path: join(evidence, "04-join-signed-out.png") });

    await page.addInitScript((token) => {
      localStorage.setItem("multica_token", token!);
      localStorage.setItem("multica:chat:isOpen", "false");
    }, api.getToken());
    await page.reload();
    await expect(page.getByRole("button", { name: "加入", exact: true })).toBeVisible();
    await page.screenshot({ path: join(evidence, "05-join-signed-in.png") });
    // Existing members enter the invited workspace without an error.
    await page.getByRole("button", { name: "加入", exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/${workspace.slug}/issues$`));
    const column = (status: string) => page.locator(`[data-board-column="status:${status}"]`);
    await expect(column("todo")).toBeVisible();
    await expect(page.locator("html")).toHaveAttribute("lang", "zh-CN");
    await expect(column("done")).toHaveCount(0);
    await expect(column("cancelled")).toHaveCount(0);
    await expect(column("backlog")).toContainText(backlog.title);
    await expect(column("in_progress")).toHaveCSS("width", "200px");
    await expect(column("todo")).toHaveCSS("width", "280px");
    expect((await page.context().cookies()).find((cookie) => cookie.name === "multica-locale")).toBeUndefined();
    await page.evaluate(() => { localStorage.setItem("theme", "light"); });
    await page.reload();
    await expect(column("todo")).toBeVisible();
    await expect(page.locator("html")).not.toHaveClass(/dark/);
    const backgrounds = await page.locator("[data-board-column]").evaluateAll((elements) => elements.map((element) => getComputedStyle(element).backgroundColor));
    expect(new Set(backgrounds).size).toBe(1);
    await page.screenshot({ path: join(evidence, "01-board-en-US-light.png") });
    await page.evaluate(() => { localStorage.setItem("theme", "dark"); });
    await page.reload();
    await expect(page.locator("html")).toHaveClass(/dark/);
    await expect(column("in_progress")).toHaveCSS("width", "200px");
    const darkBackgrounds = await page.locator("[data-board-column]").evaluateAll((elements) => elements.map((element) => getComputedStyle(element).backgroundColor));
    expect(new Set(darkBackgrounds).size).toBe(1);
    await page.screenshot({ path: join(evidence, "02-board-en-US-dark.png") });
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.evaluate(() => { localStorage.setItem("theme", "light"); });
    await page.reload();
    await expect(column("todo")).toBeVisible();
    await page.screenshot({ path: join(evidence, "07-board-1280x800.png") });

    await page.setViewportSize({ width: 1920, height: 1000 });
    const card = page.locator("[data-board-card]").filter({ hasText: issue.title });
    const start = await card.boundingBox();
    const target = await column("in_progress").boundingBox();
    expect(start).not.toBeNull();
    expect(target).not.toBeNull();
    await page.mouse.move(start!.x + start!.width / 2, start!.y + start!.height / 2);
    await page.mouse.down();
    await page.mouse.move(start!.x + start!.width / 2 + 10, start!.y + start!.height / 2, { steps: 4 });
    await page.mouse.move(target!.x + 70, target!.y + 140, { steps: 20 });
    await page.mouse.up();
    await expect(column("in_progress")).toContainText(issue.title);
    await expect(column("in_progress")).toHaveCSS("width", "280px");
    await page.reload();
    await expect(column("in_progress")).toContainText(issue.title);

    // A new page has no React Query cache and opens the identifier URL directly.
    const detail = await page.context().newPage();
    await detail.setViewportSize({ width: 1440, height: 1000 });
    await detail.goto(`/${workspace.slug}/issues/${issue.identifier}`);
    await expect(detail).toHaveTitle(`${issue.identifier}: ${issue.title} — U Workers`);
    const wakeupHeading = detail.getByRole("heading", { name: "唤醒", exact: true });
    await expect(wakeupHeading).toBeVisible();
    await expect(detail.getByRole("button", { name: "新建唤醒", exact: true })).toBeVisible();
    const prHeading = detail.getByRole("heading", { name: "Pull Request", exact: true });
    await expect(prHeading).toBeVisible();
    await expect(detail.getByRole("button", { name: "关联 PR", exact: true })).toBeVisible();
    await detail.screenshot({ path: join(evidence, "06-direct-issue-empty-sections.png") });
    if (process.env.E2E_B1_WINDOW_CAPTURE) {
      execFileSync("pwsh.exe", ["-NoProfile", "-File", process.env.E2E_B1_WINDOW_CAPTURE,
        "-Title", `${issue.identifier}: ${issue.title} — U Workers`,
        "-Output", join(evidence, "08-browser-tab-title.png")]);
    }
    writeFileSync(join(evidence, "verification.json"), JSON.stringify({
      browserLocale: "en-US", localeCookie: null, directUrl: detail.url(), documentTitle: await detail.title(),
      widths: { empty: 200, populated: 280 }, uniformBackgrounds: true, hidden: ["done", "cancelled"],
    }, null, 2));
    await detail.close();

    // Explicit English remains available, including server-rendered join copy.
    const english = await browser.newContext({ locale: "zh-CN" });
    await english.addCookies([{ name: "multica-locale", value: "en", url: baseURL! }]);
    const englishPage = await english.newPage();
    await englishPage.goto(`${baseURL}/join?code=${invite.code}`);
    await expect(englishPage.getByRole("heading", { name: "Join 设计协作" })).toBeVisible();
    await expect(englishPage.getByRole("button", { name: "Log in to join" })).toBeVisible();
    await english.close();
  } finally {
    await api.cleanup();
    if (workspace) await api.deleteWorkspace();
  }
});
