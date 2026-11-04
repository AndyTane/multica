import { test, expect } from "@playwright/test";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import pg from "pg";
import { TestApiClient } from "./fixtures";

const isolatedRoot = process.env.E2E_B2_ISOLATED_ROOT;
test.skip(!isolatedRoot, "Requires a fresh isolated B2 native deployment");
test.use({ locale: "en-US", viewport: { width: 1440, height: 900 } });

test("B2: board keeps fixed column widths, measured comments fold, and Chinese terms read naturally", async ({ page, baseURL }, testInfo) => {
  test.setTimeout(240_000);
  const deployment = JSON.parse(readFileSync(join(isolatedRoot!, "deployment.json"), "utf8"));
  const database = new URL(process.env.DATABASE_URL!);
  expect(database.hostname).toBe("127.0.0.1");
  expect(database.port).toBe(String(deployment.DatabasePort));
  expect(database.pathname).toBe("/uworkers_b2");
  expect(baseURL).toBe(`http://127.0.0.1:${deployment.WebPort}`);
  expect(process.env.NEXT_PUBLIC_API_URL).toBe(`http://127.0.0.1:${deployment.ApiPort}`);
  const evidence = process.env.E2E_B2_EVIDENCE_DIR ?? testInfo.outputPath("screenshots");
  mkdirSync(evidence, { recursive: true });
  const capture = (filename: string) => page.screenshot({ path: join(evidence, filename), animations: "disabled" });
  const api = new TestApiClient();
  const db = new pg.Client(process.env.DATABASE_URL);
  await db.connect();
  let workspace: { id: string; slug: string } | undefined;
  const suffix = Date.now().toString(36);
  const email = `b2-${suffix}@example.test`;
  try {
    await db.query("INSERT INTO instance_setting (key,value) VALUES ('open_registration', $1::jsonb) ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value", [JSON.stringify({ enabled: true })]);
    await api.login(email, "前端体验验证");
    workspace = await api.ensureWorkspace("产品设计", `b2-${suffix}`);
    await api.markUserOnboarded();
    const user = (await db.query('SELECT id FROM "user" WHERE email = $1', [email])).rows[0];
    const headers = { Authorization: `Bearer ${api.getToken()}`, "X-Workspace-ID": workspace.id };
    const statuses = ["backlog", "todo", "in_progress", "in_review", "blocked"];
    const titles = ["整理下一批需求与用户反馈", "改善多智能体协作的日常体验", "核对侧栏展开时的五列布局", "检查标题头像与超长标签的边界", "等待接口依赖并跟进处理结果"];
    const issues = [];
    for (const [index, status] of statuses.entries()) {
      issues.push(await api.createIssue(titles[index]!, { status, assignee_type: "member", assignee_id: user.id }));
    }
    const issue = issues[1]!;
    // Seed a long label through the same API as the product, on this test workspace only.
    const labelResponse = await page.request.post(`${process.env.NEXT_PUBLIC_API_URL}/api/labels`, {
      headers, data: { name: "产品体验与可访问性检查的超长标签", color: "#dbeafe" },
    });
    expect(labelResponse.ok()).toBe(true);
    const label = await labelResponse.json();
    const labelAttachment = await page.request.post(`${process.env.NEXT_PUBLIC_API_URL}/api/issues/${issue.id}/labels`, {
      headers, data: { label_id: label.id },
    });
    expect(labelAttachment.ok()).toBe(true);
    const longText = "结论：三个体验改进已经完成，下面是验证记录。\n\n" + Array.from({ length: 16 }, (_, i) =>
      `### 验证步骤 ${i + 1}\n\n检查任务标题、负责人和标签在窄列里的布局，确认信息清晰可读。后台刷新后仍保留当前状态，页面在浅色和深色主题中都能正常显示。\n\n实际高度超过阈值后，评论先展示结论，读者可以按需展开完整记录。`).join("\n\n") + "\n\n完整记录结束：B2-searchable-tail";
    const longComment = await api.createComment(issue.id, longText);
    const sha = "0123456789abcdef".repeat(4);
    const tableComment = await api.createComment(issue.id, `宽表格与长代码保持完整：\n\n| 版本 | 校验值一 | 校验值二 | 校验值三 |\n| --- | --- | --- | --- |\n| v0.2.8 | ${sha} | ${sha} | ${sha} |\n\n\`\`\`text\n${sha.repeat(4)}\n\`\`\``);
    const attachmentIds: string[] = [];
    for (const name of ["验收记录.txt", "布局测量.txt", "测试汇总.txt", "文案清单.txt"]) {
      const upload = await page.request.post(`${process.env.NEXT_PUBLIC_API_URL}/api/upload-file`, {
        headers, multipart: { issue_id: issue.id, file: { name, mimeType: "text/plain", buffer: Buffer.from("B2 isolated acceptance fixture") } },
      });
      expect(upload.ok()).toBe(true);
      attachmentIds.push((await upload.json()).id);
    }
    const attachedResponse = await page.request.post(`${process.env.NEXT_PUBLIC_API_URL}/api/issues/${issue.id}/comments`, {
      headers, data: { content: "交付文件集中在这里，按需展开查看。", attachment_ids: attachmentIds },
    });
    expect(attachedResponse.ok()).toBe(true);
    const attachedComment = await attachedResponse.json();
    await page.addInitScript((token) => {
      localStorage.setItem("multica_token", token!);
      localStorage.setItem("multica:chat:isOpen", "false");
      if (!localStorage.getItem("theme")) localStorage.setItem("theme", "light");
    }, api.getToken());
    await page.goto(`/${workspace.slug}/issues`);
    const column = (status: string) => page.locator(`[data-board-column="status:${status}"]`);
    await expect(column("blocked")).toContainText(titles[4]!);
    await page.getByRole("button", { name: "显示", exact: true }).click();
    await page.getByRole("button", { name: "标签", exact: true }).click();
    await page.keyboard.press("Escape");
    await expect(column("todo").getByText(label.name, { exact: true })).toBeVisible();
    await expect(page.getByRole("link", { name: "运行环境", exact: true }).first()).toBeVisible();
    const measurements: Record<string, unknown> = {};
    // The board keeps its fixed column widths: 280px populated, 200px empty.
    const widths = async () => page.locator("[data-board-column]").evaluateAll((columns) =>
      columns.map((col) => col.getBoundingClientRect().width));
    measurements["1440"] = await widths();
    expect(measurements["1440"]).toEqual([280, 280, 280, 280, 280]);
    // Cards keep their pre-B2 content: no chip or meta row is squeezed by a narrow column.
    for (const card of await page.locator("[data-board-card]").all()) {
      expect(await card.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
    }
    const hiddenColumns = page.getByText("隐藏的列", { exact: true }).locator("xpath=../..");
    await expect(hiddenColumns).toContainText("已完成");
    await expect(hiddenColumns).toContainText("已取消");
    expect(await hiddenColumns.evaluate((element) => element.closest("[data-board-scroller]") !== null)).toBe(true);
    await capture("01-board-1440-light.png");
    await page.evaluate(() => { localStorage.setItem("theme", "dark"); });
    await page.reload();
    await expect(column("blocked")).toContainText(titles[4]!);
    await expect(page.locator("html")).toHaveClass(/dark/);
    await capture("02-board-1440-dark.png");
    await page.evaluate(() => { localStorage.setItem("theme", "light"); });
    await page.reload();
    await expect(column("blocked")).toContainText(titles[4]!);
    await expect(page.locator("html")).not.toHaveClass(/dark/);
    await page.setViewportSize({ width: 1280, height: 800 });
    await capture("03-board-1280.png");
    await page.setViewportSize({ width: 1920, height: 1080 });
    measurements["1920"] = await widths();
    expect(measurements["1920"]).toEqual([280, 280, 280, 280, 280]);
    await capture("04-board-1920.png");
    await page.setViewportSize({ width: 1440, height: 900 });
    const card = column("todo").locator("[data-board-card]").first();
    const start = (await card.boundingBox())!;
    const target = (await column("in_progress").boundingBox())!;
    await page.mouse.move(start.x + start.width / 2, start.y + start.height / 2);
    await page.mouse.down();
    await page.mouse.move(start.x + start.width / 2 + 12, start.y + start.height / 2, { steps: 4 });
    await page.mouse.move(target.x + 60, target.y + 160, { steps: 12 });
    await capture("05-board-drag.png");
    await page.mouse.up();
    await expect(column("in_progress")).toContainText(issue.title);
    await expect(column("todo")).toHaveCSS("width", "200px");
    // Drop into an empty column as well.
    const moved = column("in_progress").locator("[data-board-card]").filter({ hasText: issue.title });
    const movedBox = (await moved.boundingBox())!;
    const empty = (await column("todo").boundingBox())!;
    await page.mouse.move(movedBox.x + 40, movedBox.y + 40);
    await page.mouse.down();
    await page.mouse.move(empty.x + 60, empty.y + 150, { steps: 20 });
    await page.mouse.up();
    await expect(column("todo")).toContainText(issue.title);
    await expect(column("todo")).toHaveCSS("width", "280px");

    await page.goto(`/${workspace.slug}/issues/${issue.identifier}`);
    const body = page.locator(`[data-comment-content="${longComment.id}"]`);
    await expect(body.getByRole("button", { name: "展开全文" })).toBeVisible();
    await body.scrollIntoViewIfNeeded();
    await capture("06-comment-collapsed.png");
    await body.getByRole("button", { name: "展开全文" }).focus();
    await page.keyboard.press("Enter");
    await expect(body.getByRole("button", { name: "收起", exact: true })).toHaveAttribute("aria-expanded", "true");
    await capture("07-comment-expanded.png");
    await body.getByRole("button", { name: "收起", exact: true }).click();
    await expect(body).toContainText("B2-searchable-tail");
    expect(await page.evaluate(() => {
      const found = (window as unknown as { find: (text: string) => boolean }).find("B2-searchable-tail");
      window.getSelection()?.removeAllRanges();
      return found;
    })).toBe(true);
    await page.goto(`/${workspace.slug}/issues/${issue.identifier}#comment-${longComment.id}`);
    await expect(body.getByRole("button", { name: "收起", exact: true })).toHaveAttribute("aria-expanded", "true");

    await page.goto(`/${workspace.slug}/issues/${issue.identifier}#comment-${tableComment.id}`);
    const tableBody = page.locator(`[data-comment-content="${tableComment.id}"]`);
    const table = tableBody.locator(".tableWrapper");
    await expect(table).toBeVisible();
    const tableSize = await table.evaluate((element) => ({ clientWidth: element.clientWidth, scrollWidth: element.scrollWidth }));
    expect(tableSize.scrollWidth).toBeGreaterThan(tableSize.clientWidth);
    expect(await tableBody.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
    await expect(tableBody).toContainText(sha.repeat(4));
    await table.evaluate((element) => { element.scrollLeft = 120; });
    await capture("08-wide-table-scroll.png");
    measurements.table = tableSize;

    await page.goto(`/${workspace.slug}/issues/${issue.identifier}#comment-${attachedComment.id}`);
    const attachments = page.locator(`#comment-${attachedComment.id} details`);
    await expect(attachments.locator("summary")).toHaveText("4 个附件");
    await expect(attachments).not.toHaveAttribute("open");
    await expect(page.getByRole("heading", { name: "跟进", exact: true })).toBeVisible();
    await capture("09-attachments-and-followup.png");
    const attachmentSummary = attachments.locator("summary");
    const attachmentChevron = attachmentSummary.locator("svg.lucide-chevron-right");
    await expect(attachmentChevron).toHaveAttribute("aria-hidden", "true");
    await expect(attachmentSummary).toHaveCSS("list-style-type", "none");
    await expect(attachmentChevron).toHaveCSS("rotate", "none");
    await capture("12-attachments-collapsed.png");
    // Keep the native disclosure keyboard behavior while matching the activity icon.
    await attachmentSummary.focus();
    await page.keyboard.press("Enter");
    await expect(attachments).toHaveAttribute("open", "");
    await expect(attachmentChevron).toHaveCSS("rotate", "90deg");
    await expect(attachments.getByRole("button", { name: "测试汇总.txt" })).toBeVisible();
    await capture("13-attachments-expanded.png");
    await page.emulateMedia({ reducedMotion: "reduce" });
    await expect(attachmentChevron).toHaveCSS("transition-property", "none");
    await page.keyboard.press("Space");
    await expect(attachments).not.toHaveAttribute("open");
    await expect(attachmentChevron).toHaveCSS("rotate", "none");
    await page.emulateMedia({ reducedMotion: "no-preference" });

    // A new long comment sent through the real composer opens immediately.
    const composer = page.locator("div.relative.flex.flex-col.rounded-lg.bg-card.pb-8");
    await composer.getByTestId("comment-composer-shell").click();
    const editor = composer.locator('.ProseMirror[contenteditable="true"]');
    await expect(editor).toBeVisible();
    await editor.fill("刚发布的结论。\n\n" + "本次发布的长评论保持展开。\n\n".repeat(100));
    const sent = page.waitForResponse((response) =>
      [issue.id, issue.identifier].some((id) => response.url().endsWith(`/api/issues/${id}/comments`))
        && response.request().method() === "POST", { timeout: 30_000 });
    await composer.getByRole("button", { name: "发送", exact: true }).click();
    const posted = await (await sent).json();
    await expect(page.locator(`[data-comment-content="${posted.id}"]`).getByRole("button", { name: "收起", exact: true })).toBeVisible();
    await page.goto(`/${workspace.slug}/runtimes`);
    await expect(page.getByRole("heading", { name: "运行环境", exact: true })).toBeVisible();
    await expect(page.locator("p").filter({ hasText: "智能体干活用的电脑和云端环境。" })).toBeVisible();
    await expect(page.getByRole("link", { name: "小组", exact: true }).first()).toBeVisible();
    await capture("10-runtimes-and-sidebar.png");
    writeFileSync(join(evidence, "verification.json"), JSON.stringify(measurements, null, 2));
  } finally {
    await db.end();
    await api.cleanup();
    if (workspace) await api.deleteWorkspace();
  }
});
