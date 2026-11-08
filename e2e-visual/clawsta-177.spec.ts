import { test, expect } from "@playwright/test";
import pg from "pg";
import { mkdirSync } from "node:fs";
import { join } from "node:path";
import { TestApiClient } from "../e2e/fixtures";

// Only the task's isolated PostgreSQL instance may receive these demo records.
test("CLAWSTA-177: review feedback and a single footer divider", async ({ browser }) => {
  test.setTimeout(600_000);
  const connection = process.env.DATABASE_URL;
  if (!connection || new URL(connection).port !== "25432" || !new URL(connection).pathname.startsWith("/multica_")) {
    throw new Error("Use the CLAWSTA-177 isolated worktree database on port 25432");
  }
  const db = new pg.Client(connection);
  await db.connect();
  const api = new TestApiClient();
  const output = join(process.cwd(), "evidence/clawsta-177");
  mkdirSync(output, { recursive: true });
  const email = `clawsta177-${Date.now()}@example.test`;
  let agentId: string | undefined;
  try {
    await db.query(`INSERT INTO "user" (name,email,onboarded_at,language) VALUES ('Tane',$1,now(),'zh-Hans') ON CONFLICT(email) DO UPDATE SET onboarded_at=now(),language='zh-Hans'`, [email]);
    const login = await api.login(email, "Tane");
    const ws = await api.ensureWorkspace("审批交付演示", `approval-review-${Date.now()}`);
    await api.markUserOnboarded();
    await db.query("UPDATE workspace SET issue_prefix='DEMO' WHERE id=$1", [ws.id]);
    const agent = await db.query(`INSERT INTO agent(workspace_id,name,description,owner_id,runtime_mode,runtime_config,visibility,permission_mode,max_concurrent_tasks) VALUES($1,'交付负责人','',$2,'cloud','{}','private','private',1) RETURNING id`, [ws.id, login.user.id]);
    agentId = agent.rows[0].id;
    const cards = await api.seedTableIssues([
      { title: "确认接口版本", status: "blocked" },
      { title: "验证桌面端安装", status: "blocked" },
      { title: "审核界面改动", status: "in_review" },
      { title: "确认合并范围", status: "in_review" },
      { title: "补充部署记录", status: "blocked" },
    ]);
    const actions = ["这个接口使用 v1 还是 v2？", "安装新版本后，打开一次任务看板", "检查审批弹层和页面分隔线", "同意就在审批意见里写「同意合并」", ""];
    const kinds = ["reply", "act", "approve", "approve"];
    for (let i = 0; i < cards.length; i++) {
      const metadata = i < 4 ? { review_by: "user", review_kind: kinds[i], review_user: login.user.id, review_action: actions[i], review_since: new Date(Date.now() - 3600_000).toISOString() } : {};
      await db.query("UPDATE issue SET assignee_type='agent',assignee_id=$2,metadata=$3::jsonb WHERE id=$1", [cards[i]!.id, agentId, JSON.stringify(metadata)]);
    }
    for (const theme of ["light", "dark"] as const) {
      const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, colorScheme: theme });
      await context.addCookies([{ name: "multica-locale", value: "zh-Hans", domain: "localhost", path: "/" }]);
      await context.addInitScript(({ theme, token }) => {
        localStorage.setItem("theme", theme);
        localStorage.setItem("multica_token", token);
        localStorage.setItem("multica-locale", "zh-Hans");
        localStorage.setItem("multica:chat:isOpen", "false");
        document.cookie = "sidebar_state=true; path=/";
      }, { theme, token: api.getToken()! });
      const page = await context.newPage();
      const errors: string[] = [];
      page.on("pageerror", error => errors.push(error.message));
      try {
        await page.goto(`/${ws.slug}/awaiting`, { waitUntil: "domcontentloaded", timeout: 240_000 });
        const approvals = page.locator("#awaiting-group-approve + ul");
        await expect(approvals.getByRole("listitem")).toHaveCount(2, { timeout: 120_000 });
        const footer = page.getByRole("button", { name: "另有 1 张卡在等人，但没标明等谁" });
        await expect(footer).toBeVisible();
        // Inspect the actual CSS: separators remain between rows; only the
        // footer supplies a border after the last approval row.
        for (const kind of ["reply", "act", "approve"]) {
          const lastRow = page.locator(`#awaiting-group-${kind} + ul > li:last-child`);
          await expect(lastRow).toHaveCSS("border-bottom-width", "0px");
        }
        await expect(approvals.getByRole("listitem").first()).toHaveCSS("border-bottom-width", "1px");
        await expect(footer.locator("..")).toHaveCSS("border-top-width", "1px");
        await page.screenshot({ path: join(output, `footer-${theme}.png`), animations: "disabled" });
        await approvals.getByRole("button", { name: "审批意见", exact: true }).last().click();
        const textbox = page.getByRole("textbox", { name: "审批意见" });
        await expect(textbox).toBeVisible();
        await expect(textbox).toHaveAttribute("placeholder", "要改的地方，或要交代的事");
        await expect(textbox).toHaveAttribute("required");
        await expect(page.getByText("意见将交回 交付负责人")).toBeVisible();
        const confirm = page.getByRole("dialog").getByRole("button", { name: "发送", exact: true });
        await expect(confirm).toBeDisabled();
        await textbox.fill("同意合并");
        await expect(confirm).toBeEnabled();
        await page.screenshot({ path: join(output, `feedback-popover-${theme}.png`), animations: "disabled" });
        expect(errors).toEqual([]);
      } finally { await context.close(); }
    }
  } finally {
    await api.cleanup();
    if (agentId) await db.query("DELETE FROM agent WHERE id=$1", [agentId]);
    await db.end();
  }
});
