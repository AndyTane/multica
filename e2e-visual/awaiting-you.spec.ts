import { test, expect } from "@playwright/test";
import pg from "pg";
import { mkdirSync } from "node:fs";
import { join } from "node:path";
import { TestApiClient } from "../e2e/fixtures";
import demo from "../evidence/clawsta-151/review-cards.json" with { type: "json" };

// Run only against an isolated R1 database. Real-card content has no credentials,
// comments or descriptions; the fixture owns all inserted issues and agents.
test("23 real review cards: themes, widths, badges, send-back and undo", async ({ browser }) => {
  test.setTimeout(600_000);
  const connection = process.env.DATABASE_URL;
  if (!connection?.includes("clawsta151_r1")) throw new Error("Use the isolated clawsta151_r1 database");
  const client = new pg.Client(connection);
  await client.connect();
  const api = new TestApiClient();
  const agentIds: string[] = [];
  const memberUserIds: string[] = [];
  const r2 = process.env.CLAWSTA151_REVIEW_ROUND === "R2";
  const output = join(process.cwd(), "e2e-visual/__out__/clawsta-151", ...(r2 ? ["r2"] : []));
  mkdirSync(output, { recursive: true });
  let workspace: { id: string; slug: string } | undefined;
  try {
    await client.query(`INSERT INTO "user"(name,email,email_verified,onboarded_at,language) VALUES('Tane','clawsta151-r1@example.test',true,now(),'zh-Hans') ON CONFLICT(email) DO UPDATE SET onboarded_at=now(),language='zh-Hans'`);
    const login = await api.login("clawsta151-r1@example.test", "Tane");
    workspace = await api.ensureWorkspace("UWorkers", "uworkers-r1");
    await api.markUserOnboarded();
    const ws = workspace;
    const userId: string = login.user.id;
    await client.query(`UPDATE workspace SET issue_prefix='CLAWSTA' WHERE id=$1`, [ws.id]);
    const actors = new Map<string, string>();
    for (const card of demo.cards) {
      if (card.assignee_type !== "member" || !card.assignee_id || actors.has(card.assignee_id)) continue;
      if (card.assignee_name === login.user.name) { actors.set(card.assignee_id, userId); continue; }
      const member = await client.query(`INSERT INTO "user"(name,email) VALUES($1,$2) RETURNING id`, [card.assignee_name ?? "成员", `review-member-${Date.now()}-${memberUserIds.length}@example.test`]);
      memberUserIds.push(member.rows[0].id); actors.set(card.assignee_id, member.rows[0].id);
      await client.query(`INSERT INTO member(workspace_id,user_id,role) VALUES($1,$2,'member')`, [ws.id, member.rows[0].id]);
    }
    for (const card of demo.cards) {
      if (card.assignee_type !== "agent" || !card.assignee_id || actors.has(card.assignee_id)) continue;
      const actor = await client.query(`INSERT INTO agent(workspace_id,name,description,owner_id,runtime_mode,runtime_config,visibility,permission_mode,max_concurrent_tasks) VALUES($1,$2,'',$3,'cloud','{}','private','private',1) RETURNING id`, [ws.id, card.assignee_name ?? "交付负责人", userId]);
      actors.set(card.assignee_id, actor.rows[0].id); agentIds.push(actor.rows[0].id);
    }
    const issues = await api.seedTableIssues(demo.cards.map((card) => ({ title: card.title, status: "in_review" as const, position: card.position })));
    const byNumber = new Map<number, string>();
    for (let n = 0; n < demo.cards.length; n++) {
      const card = demo.cards[n]!; const issue = issues[n]!; byNumber.set(card.number, issue.id);
      const metadata = card.number === 143 ? { review_by: "user", review_user: userId, review_action: "在公司 G1024152 装好 v0.2.16，跑一次四十多分钟的编译，中途没被停掉就关" }
        : card.number === 149 ? { review_by: "user", review_user: userId, review_action: "看完本次汇报，没问题就关" }
        : card.number === 148 ? { review_by: "specialist", review_agent: actors.get(card.assignee_id!) }
        : {};
      await client.query(`UPDATE issue SET number=$2,assignee_type=$3,assignee_id=$4,created_at=$5,updated_at=$6,status_changed_at=$7,metadata=$8::jsonb WHERE id=$1`, [issue.id, card.number, card.assignee_type, card.assignee_id ? actors.get(card.assignee_id) ?? null : null, card.created_at, card.updated_at, card.status_changed_at, JSON.stringify(metadata)]);
    }
    await client.query(`UPDATE workspace SET issue_counter=1000 WHERE id=$1`, [ws.id]);
    for (const theme of ["light", "dark"] as const) {
      const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, colorScheme: theme });
      await context.addCookies([{ name: "multica-locale", value: "zh-Hans", domain: "localhost", path: "/" }]);
      await context.addInitScript(({ theme, token, slug }) => {
        localStorage.setItem("theme", theme); localStorage.setItem("multica_token", token);
        localStorage.setItem("multica-locale", "zh-Hans"); localStorage.setItem("multica:chat:isOpen", "false");
        localStorage.setItem(`multica_issue_surface_views:${slug}`, JSON.stringify({ state: { surfaces: { "workspace:all": { state: { viewMode: "board" }, updatedAt: new Date().toISOString() } } }, version: 1 }));
        document.cookie = "sidebar_state=true; path=/";
      }, { theme, token: api.getToken()!, slug: ws.slug });
      const page = await context.newPage();
      const errors: string[] = []; page.on("pageerror", (err) => errors.push(err.message));
      try {
        await page.goto(`/${ws.slug}/awaiting`, { waitUntil: "domcontentloaded", timeout: 240_000 });
        const list = page.getByRole("list", { name: "等我处理" });
        await expect(list.getByRole("listitem")).toHaveCount(2, { timeout: 90_000 });
        await expect(list.getByRole("listitem").first()).toContainText("CLAWSTA-143");
        await expect(page.locator(`a[href="/${ws.slug}/awaiting"]`)).toContainText("2");
        if (r2) {
          if (theme === "light") {
            await page.setViewportSize({ width: 1024, height: 900 });
            await page.screenshot({ path: join(output, "awaiting-1024-light.png"), animations: "disabled" });
          }
          await page.setViewportSize({ width: 1280, height: 900 });
          await list.getByRole("button", { name: "打回", exact: true }).first().click();
          const textbox = page.getByRole("textbox", { name: "打回意见" });
          await textbox.fill("编译结束后补一张产物截图。");
          await page.screenshot({ path: join(output, `send-back-${theme}.png`), animations: "disabled" });
          if (theme === "light") {
            const issueId = byNumber.get(143);
            const before = await client.query(`SELECT status,revision,(SELECT count(*) FROM comment WHERE issue_id=$1) AS comments,(SELECT count(*) FROM agent_task_queue WHERE issue_id=$1) AS runs FROM issue WHERE id=$1`, [issueId]);
            await textbox.fill("/note 请修复编译错误");
            await page.getByRole("button", { name: "打回", exact: true }).last().click();
            await expect(page.getByRole("alert")).toHaveText("以 /note 开头的意见只做记录，不会交给 协调负责人");
            await expect(textbox).toHaveValue("/note 请修复编译错误");
            await expect(textbox).toBeEditable();
            await expect(list.getByRole("listitem")).toHaveCount(2);
            const after = await client.query(`SELECT status,revision,(SELECT count(*) FROM comment WHERE issue_id=$1) AS comments,(SELECT count(*) FROM agent_task_queue WHERE issue_id=$1) AS runs FROM issue WHERE id=$1`, [issueId]);
            expect(after.rows).toEqual(before.rows);
            await page.screenshot({ path: join(output, "note-not-delivered-light.png"), animations: "disabled" });
          }
          expect(errors).toEqual([]);
          continue;
        }
        for (const width of [1280, 1024]) {
          await page.setViewportSize({ width, height: 900 });
          await page.screenshot({ path: join(output, `awaiting-${width}-${theme}.png`), animations: "disabled" });
        }
        await page.setViewportSize({ width: 1280, height: 900 });
        await list.getByRole("button", { name: "打回", exact: true }).first().click();
        await page.getByRole("textbox").fill("编译结束后补一张产物截图。");
        await page.screenshot({ path: join(output, `send-back-${theme}.png`), animations: "disabled" });
        await page.keyboard.press("Escape");
        await list.getByRole("button", { name: "通过", exact: true }).first().click();
        const toast = page.locator("[data-sonner-toast]").filter({ hasText: "已通过 CLAWSTA-143" });
        await expect(toast).toBeVisible();
        await expect(list.getByRole("listitem")).toHaveCount(1);
        await page.screenshot({ path: join(output, `approved-${theme}.png`), animations: "disabled" });
        await toast.getByRole("button", { name: "撤销" }).click();
        await expect(list.getByRole("listitem")).toHaveCount(2);
        // Undo begins a new review round. Restore the historical demo timestamp
        // directly in the isolated fixture before recording the other theme.
        const original = demo.cards.find((card) => card.number === 143)!;
        await client.query(`UPDATE issue SET status_changed_at=$2 WHERE id=$1`, [byNumber.get(143), original.status_changed_at]);
        await page.goto(`/${ws.slug}/issues`, { waitUntil: "domcontentloaded", timeout: 240_000 });
        const mine = page.locator(`[data-peek-target="${byNumber.get(143)}"]`).first();
        await expect(mine).toBeVisible({ timeout: 90_000 });
        await mine.scrollIntoViewIfNeeded();
        await expect(mine).toContainText("等你");
        await expect(page.locator(`[data-peek-target="${byNumber.get(148)}"]`).first()).toContainText("等验收");
        await page.screenshot({ path: join(output, `board-${theme}.png`), animations: "disabled" });
        expect(errors).toEqual([]);
      } finally { await context.close(); }
    }
  } finally {
    await api.cleanup();
    if (agentIds.length) await client.query(`DELETE FROM agent WHERE id=ANY($1::uuid[])`, [agentIds]);
    if (memberUserIds.length) {
      await client.query(`DELETE FROM member WHERE workspace_id=$1 AND user_id=ANY($2::uuid[])`, [workspace?.id, memberUserIds]);
      await client.query(`DELETE FROM "user" WHERE id=ANY($1::uuid[])`, [memberUserIds]);
    }
    await client.end();
  }
});
