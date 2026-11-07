import { test, expect, type Browser, type BrowserContext, type Page } from "@playwright/test";
import pg from "pg";
import { mkdirSync } from "node:fs";
import { join } from "node:path";
import { TestApiClient } from "../e2e/fixtures";

// CLAWSTA-151 R4b acceptance screenshots (integration card R4b-4).
//
// Runs ONLY against an isolated worktree database seeded here: this spec
// rewrites one demo workspace (slug uworkers-r4) and refuses the shared
// database. Start the checkout environment first (scripts/dev-env.sh up or
// the Makefile targets), then:
//
//   CLAWSTA151_R4=1 PLAYWRIGHT_BASE_URL=http://localhost:<frontend port> \
//     pnpm exec playwright test -c playwright.visual.config.ts clawsta-151-r4.spec.ts
//
// The evidence set is the R4b list: three waiting groups, the reply and done
// popovers, the expanded unmarked footer, the sync toggle in both wordings and
// its locked state, the transfer preflight, the inbox parent line, and the
// blocked-column labels — each in light and dark.

const OUTPUT = join(process.cwd(), "evidence/clawsta-151/r4");

type Workspace = { id: string; slug: string };

type Ids = {
  reply: string;
  act: string;
  approve: string;
  unmarkedPlain: string;
  unmarkedProblem: string;
  boardMineReply: string;
  boardOtherAct: string;
  boardOtherReply: string;
  syncOwnerChild: string;
  syncRecipientChild: string;
  syncLockedChild: string;
  transferCard: string;
  inboxChild: string;
};

function isoAgo(hours: number): string {
  return new Date(Date.now() - hours * 3600_000).toISOString();
}

async function seedDemo(client: pg.Client, ws: Workspace, ownerId: string): Promise<Ids> {
  // Wipe only this demo workspace, in dependency order.
  const wipe: Array<[string, unknown[]]> = [
    ["DELETE FROM inbox_item WHERE workspace_id=$1", [ws.id]],
    ["DELETE FROM comment WHERE workspace_id=$1", [ws.id]],
    ["DELETE FROM activity_log WHERE workspace_id=$1", [ws.id]],
    ["DELETE FROM issue WHERE workspace_id=$1", [ws.id]],
    ["DELETE FROM squad WHERE workspace_id=$1", [ws.id]],
    ["DELETE FROM agent WHERE workspace_id=$1", [ws.id]],
    ["DELETE FROM agent_runtime WHERE workspace_id=$1", [ws.id]],
    ["DELETE FROM member WHERE workspace_id=$1 AND user_id<>$2", [ws.id, ownerId]],
  ];
  for (const [sql, params] of wipe) {
    await client.query(sql, params);
  }

  const insertUser = async (name: string, email: string): Promise<string> => {
    const res = await client.query(
      'INSERT INTO "user"(name,email) VALUES($1,$2) ON CONFLICT(email) DO UPDATE SET name=EXCLUDED.name RETURNING id',
      [name, email],
    );
    return res.rows[0].id as string;
  };
  const chen = await insertUser("老陈", "clawsta151-r4-chen@example.test");
  const lin = await insertUser("林工", "clawsta151-r4-lin@example.test");
  const departed = await insertUser("已离职同事", "clawsta151-r4-departed@example.test");
  for (const memberId of [chen, lin]) {
    await client.query(
      "INSERT INTO member (workspace_id,user_id,role) VALUES ($1,$2,'member') ON CONFLICT DO NOTHING",
      [ws.id, memberId],
    );
  }
  await client.query("DELETE FROM member WHERE workspace_id=$1 AND user_id=$2", [ws.id, departed]);

  const insertRuntime = async (name: string): Promise<string> => {
    const res = await client.query(
      "INSERT INTO agent_runtime (workspace_id, daemon_id, name, runtime_mode, provider, status, device_info, metadata, owner_id, last_seen_at) " +
        "VALUES ($1,NULL,$2,'cloud','dsh','online','本机','{}'::jsonb,$3,now()) RETURNING id",
      [ws.id, name, ownerId],
    );
    return res.rows[0].id as string;
  };
  const insertAgent = async (name: string, owner: string, runtimeId: string): Promise<string> => {
    const res = await client.query(
      "INSERT INTO agent (workspace_id,name,description,owner_id,runtime_mode,runtime_config,runtime_id,visibility,permission_mode,max_concurrent_tasks) " +
        "VALUES ($1,$2,'',$3,'cloud','{}'::jsonb,$4,'private','private',1) RETURNING id",
      [ws.id, name, owner, runtimeId],
    );
    return res.rows[0].id as string;
  };
  const coordinator = await insertAgent("协调负责人", ownerId, await insertRuntime("协调负责人运行环境"));
  const assistant = await insertAgent("资料助手", chen, await insertRuntime("资料助手运行环境"));

  await client.query("UPDATE workspace SET issue_prefix=$1, issue_counter=$2 WHERE id=$3", ["CLAWSTA", 400, ws.id]);

  const createIssue = async (spec: {
    number: number;
    title: string;
    status: "backlog" | "todo" | "in_progress" | "in_review" | "blocked" | "done";
    creatorId: string;
    assigneeType: string | null;
    assigneeId: string | null;
    parentId?: string | null;
    metadata?: Record<string, unknown>;
    statusChangedAt: string;
    priority?: "none" | "low" | "medium" | "high" | "urgent";
  }): Promise<string> => {
    const res = await client.query(
      "INSERT INTO issue (workspace_id,title,description,status,priority,assignee_type,assignee_id,creator_type,creator_id,parent_issue_id,position,number,metadata,created_at,updated_at,status_changed_at,last_activity_at,revision) " +
        "VALUES ($1,$2,$3,$4,$5,$6,$7,'member',$8,$9,$10,$11,$12::jsonb,$13,$13,$14,$13,1) RETURNING id",
      [
        ws.id, spec.title, "演示数据，仅用于 R4b 真实渲染截图。", spec.status, spec.priority ?? "none",
        spec.assigneeType, spec.assigneeId, spec.creatorId, spec.parentId ?? null, spec.number, spec.number,
        JSON.stringify(spec.metadata ?? {}), spec.statusChangedAt, spec.statusChangedAt,
      ],
    );
    return res.rows[0].id as string;
  };

  const reply = await createIssue({ number: 301, title: "预览接口改到 v2", status: "blocked", creatorId: ownerId, assigneeType: "agent", assigneeId: coordinator, statusChangedAt: isoAgo(52), metadata: { review_by: "user", review_kind: "reply", review_user: ownerId, review_action: "这个接口用 v1 还是 v2？" } });
  const act = await createIssue({ number: 302, title: "公司电脑跑一次完整编译", status: "blocked", creatorId: ownerId, assigneeType: "agent", assigneeId: coordinator, statusChangedAt: isoAgo(9), metadata: { review_by: "user", review_kind: "act", review_user: ownerId, review_action: "在公司电脑装好 v0.2.17，跑一次完整编译" } });
  const approve = await createIssue({ number: 303, title: "看一下本轮发布汇报", status: "in_review", creatorId: ownerId, assigneeType: "agent", assigneeId: coordinator, statusChangedAt: isoAgo(1), metadata: { review_by: "user", review_kind: "approve", review_user: ownerId, review_action: "看完这次汇报，没问题就关" } });
  const unmarkedPlain = await createIssue({ number: 304, title: "补齐升级器的失败回滚", status: "blocked", creatorId: ownerId, assigneeType: "agent", assigneeId: coordinator, statusChangedAt: isoAgo(30) });
  const unmarkedProblem = await createIssue({ number: 305, title: "把旧账本迁到新库", status: "blocked", creatorId: ownerId, assigneeType: "agent", assigneeId: coordinator, statusChangedAt: isoAgo(6), metadata: { review_by: "user", review_action: "确认迁移窗口" } });
  const boardMineReply = await createIssue({ number: 306, title: "补充安装映射清单", status: "blocked", creatorId: ownerId, assigneeType: "agent", assigneeId: coordinator, statusChangedAt: isoAgo(20), metadata: { review_by: "user", review_kind: "reply", review_user: ownerId, review_action: "映射里少一项，确认下要不要补" } });
  const boardOtherAct = await createIssue({ number: 307, title: "台式机防火墙放行", status: "blocked", creatorId: ownerId, assigneeType: "agent", assigneeId: coordinator, statusChangedAt: isoAgo(15), metadata: { review_by: "user", review_kind: "act", review_user: lin, review_action: "在现场电脑上放行 13443 端口" } });
  const boardOtherReply = await createIssue({ number: 308, title: "确认公司出口地址", status: "blocked", creatorId: ownerId, assigneeType: "agent", assigneeId: coordinator, statusChangedAt: isoAgo(4), metadata: { review_by: "user", review_kind: "reply", review_user: chen, review_action: "公司出口是固定 IP 吗？" } });

  // A little life outside the blocked column so the board reads like a board.
  await createIssue({ number: 309, title: "整理本周验收记录", status: "todo", creatorId: ownerId, assigneeType: "member", assigneeId: lin, statusChangedAt: isoAgo(70) });
  await createIssue({ number: 310, title: "写 v0.2.17 升级说明", status: "in_progress", creatorId: ownerId, assigneeType: "agent", assigneeId: coordinator, statusChangedAt: isoAgo(12) });
  await createIssue({ number: 311, title: "上传证据包", status: "done", creatorId: ownerId, assigneeType: "member", assigneeId: ownerId, statusChangedAt: isoAgo(2) });

  // Sub-issue trees for the sync toggle.
  const parentA = await createIssue({ number: 320, title: "发布检查项", status: "in_progress", creatorId: ownerId, assigneeType: "agent", assigneeId: coordinator, statusChangedAt: isoAgo(28) });
  const syncOwnerChild = await createIssue({ number: 321, title: "核对差量包清单", status: "in_progress", creatorId: ownerId, assigneeType: "member", assigneeId: ownerId, parentId: parentA, statusChangedAt: isoAgo(26) });
  const parentB = await createIssue({ number: 322, title: "资料归档", status: "in_progress", creatorId: chen, assigneeType: "agent", assigneeId: assistant, statusChangedAt: isoAgo(22) });
  const syncRecipientChild = await createIssue({ number: 323, title: "补一份归档说明", status: "in_progress", creatorId: ownerId, assigneeType: "member", assigneeId: ownerId, parentId: parentB, statusChangedAt: isoAgo(21) });
  // The locked state is the one the server already reports: the ordinary route
  // for this comment already reaches the parent owner, so the toggle cannot add
  // a second delivery. A squad parent whose leader owns the child card does it.
  const squad = await client.query(
    "INSERT INTO squad (workspace_id,name,description,leader_id,creator_id,instructions) VALUES ($1,$2,'',$3,$4,'') RETURNING id",
    [ws.id, "发布小组", coordinator, ownerId],
  );
  const squadId = squad.rows[0].id as string;
  const parentC = await createIssue({ number: 324, title: "日志导出", status: "in_progress", creatorId: chen, assigneeType: "squad", assigneeId: squadId, statusChangedAt: isoAgo(18) });
  const syncLockedChild = await createIssue({ number: 325, title: "导出格式确认", status: "in_progress", creatorId: chen, assigneeType: "agent", assigneeId: coordinator, parentId: parentC, statusChangedAt: isoAgo(17) });

  // Keep the four blocked-label variants in the first screenful of the board
  // column: reply/act for me first, then the two "waiting on someone else".
  await client.query(
    "UPDATE issue SET position = CASE number WHEN 301 THEN 1 WHEN 302 THEN 2 WHEN 307 THEN 3 WHEN 308 THEN 4 WHEN 306 THEN 5 WHEN 304 THEN 6 WHEN 305 THEN 7 ELSE position END WHERE workspace_id=$1",
    [ws.id],
  );

  // A card someone else dispatched: the transfer preflight cannot let that
  // dispatcher wake the private agent, while the current member (its owner) can.
  const transferCard = await createIssue({ number: 330, title: "增加安装包校验页", status: "in_progress", creatorId: chen, assigneeType: "member", assigneeId: ownerId, statusChangedAt: isoAgo(5) });

  await client.query(
    "INSERT INTO inbox_item (workspace_id,recipient_type,recipient_id,type,severity,issue_id,title,body,actor_type,actor_id,details,created_at) " +
      "VALUES ($1,'member',$2,'new_comment','info',$3,$4,$5,'member',$6,'{}'::jsonb,$7)",
    [ws.id, ownerId, syncOwnerChild, "核对差量包清单", "补一个日志导出入口。", lin, isoAgo(3)],
  );

  return { reply, act, approve, unmarkedPlain, unmarkedProblem, boardMineReply, boardOtherAct, boardOtherReply, syncOwnerChild, syncRecipientChild, syncLockedChild, transferCard, inboxChild: syncOwnerChild };
}

async function makeContext(browser: Browser, theme: "light" | "dark", slug: string, token: string): Promise<BrowserContext> {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, colorScheme: theme });
  await context.addCookies([{ name: "multica-locale", value: "zh-Hans", domain: "localhost", path: "/" }]);
  await context.addInitScript(
    (args: { theme: string; token: string; slug: string }) => {
      localStorage.setItem("theme", args.theme);
      localStorage.setItem("multica_token", args.token);
      localStorage.setItem("multica-locale", "zh-Hans");
      localStorage.setItem("multica:chat:isOpen", "false");
      localStorage.setItem(
        "multica_issue_surface_views:" + args.slug,
        JSON.stringify({ state: { surfaces: { "workspace:all": { state: { viewMode: "board" }, updatedAt: new Date().toISOString() } } }, version: 1 }),
      );
      document.cookie = "sidebar_state=true; path=/";
    },
    { theme, token, slug },
  );
  return context;
}

async function shot(page: Page, name: string) {
  await page.screenshot({ path: join(OUTPUT, name + ".png"), animations: "disabled", fullPage: false });
}

test("CLAWSTA-151 R4b: real rendering evidence in light and dark", async ({ browser }) => {
  test.setTimeout(2_700_000);
  const connection = process.env.DATABASE_URL;
  if (!connection) throw new Error("DATABASE_URL is required");
  if (/\/multica(\?|$)/.test(connection)) throw new Error("Refusing the shared database; point DATABASE_URL at this checkout worktree database");
  if (process.env.CLAWSTA151_R4 !== "1") throw new Error("Set CLAWSTA151_R4=1 to acknowledge seeding an isolated database");
  mkdirSync(OUTPUT, { recursive: true });
  const client = new pg.Client(connection);
  await client.connect();
  const api = new TestApiClient();
  try {
    await client.query(
      'INSERT INTO "user"(name,email,email_verified,onboarded_at,language) VALUES($1,$2,true,now(),\'zh-Hans\') ON CONFLICT(email) DO UPDATE SET onboarded_at=now(),language=\'zh-Hans\'',
      ["Tane", "clawsta151-r4@example.test"],
    );
    const login = await api.login("clawsta151-r4@example.test", "Tane");
    const existing = (await api.getWorkspaces()).find((item) => item.slug === "uworkers-r4");
    let ws: Workspace;
    if (existing) {
      api.setWorkspaceId(existing.id);
      api.setWorkspaceSlug(existing.slug);
      ws = existing;
    } else {
      ws = await api.createWorkspace("UWorkers", "uworkers-r4");
    }
    await api.markUserOnboarded();
    const ownerId = login.user.id as string;
    const ids = await seedDemo(client, ws, ownerId);

    for (const theme of ["light", "dark"] as const) {
      const context = await makeContext(browser, theme, ws.slug, api.getToken() as string);
      const page = await context.newPage();
      const errors: string[] = [];
      page.on("pageerror", (error) => errors.push(String(error.message)));
      try {
        // 1. Three groups, reply -> act -> approval, each with its count.
        await page.goto("/" + ws.slug + "/awaiting", { waitUntil: "domcontentloaded", timeout: 240_000 });
        await expect(page.getByText("这个接口用 v1 还是 v2？")).toBeVisible({ timeout: 120_000 });
        await expect(page.getByText("在公司电脑装好 v0.2.17，跑一次完整编译")).toBeVisible();
        await expect(page.getByText("看完这次汇报，没问题就关")).toBeVisible();
        await shot(page, "awaiting-three-groups-" + theme);

        // 2/3. The two popovers the list introduces (reply, done).
        const replyRow = page.locator("li", { hasText: "这个接口用 v1 还是 v2？" });
        await replyRow.getByRole("button", { name: "回复", exact: true }).click();
        await expect(page.getByRole("textbox", { name: "回复内容" })).toBeVisible();
        await shot(page, "popover-reply-" + theme);
        await page.keyboard.press("Escape");
        const actRow = page.locator("li", { hasText: "在公司电脑装好 v0.2.17，跑一次完整编译" });
        await actRow.getByRole("button", { name: "已完成", exact: true }).click();
        await expect(page.getByRole("textbox", { name: "完成说明" })).toBeVisible();
        await shot(page, "popover-done-" + theme);
        await page.keyboard.press("Escape");

        // 4. Unmarked footer expands in place, one row explains its reason.
        const unmarkedButton = page.getByRole("button", { name: /另有 .* 张卡在等人/ });
        await expect(unmarkedButton).toBeVisible();
        await unmarkedButton.click();
        // 305 carries a review mark the contract cannot route: the row explains
        // the stable code (invalid_review_mark) in words instead of printing it.
        await expect(page.getByText("这张卡的审核标记不完整，没法交给人处理。")).toBeVisible();
        await expect(page.getByText("invalid_review_mark")).toBeHidden();
        await expect(page.getByText("补齐升级器的失败回滚")).toBeVisible();
        await unmarkedButton.scrollIntoViewIfNeeded();
        await shot(page, "unmarked-expanded-" + theme);

        // 5. Sync toggle: parent owner reachable (agent), label names it.
        await page.goto("/" + ws.slug + "/issues/" + ids.syncOwnerChild, { waitUntil: "domcontentloaded", timeout: 240_000 });
        const ownerToggle = page.getByRole("checkbox", { name: /同步给主卡负责人/ });
        await expect(ownerToggle).toBeVisible({ timeout: 120_000 });
        await expect(page.locator("label", { hasText: "同步给主卡负责人 · 协调负责人" })).toBeVisible();
        await page.waitForTimeout(800);
        await shot(page, "sync-toggle-owner-" + theme);

        // 6. Sync toggle: the owner cannot be woken, so it names the fallback.
        await page.goto("/" + ws.slug + "/issues/" + ids.syncRecipientChild, { waitUntil: "domcontentloaded", timeout: 240_000 });
        await expect(page.getByRole("checkbox", { name: "同步给 老陈" })).toBeVisible({ timeout: 120_000 });
        await page.waitForTimeout(800);
        await shot(page, "sync-toggle-recipient-" + theme);

        // 7. Sync toggle locked: the ordinary routing already reaches the parent
        // owner (a squad whose leader is the child card owner), so the box is
        // checked and cannot be changed.
        await page.goto("/" + ws.slug + "/issues/" + ids.syncLockedChild, { waitUntil: "domcontentloaded", timeout: 240_000 });
        const locked = page.getByRole("checkbox", { name: /同步给主卡负责人 · 发布小组/ });
        await expect(locked).toBeVisible({ timeout: 120_000 });
        await expect(locked).toHaveAttribute("aria-disabled", "true");
        await expect(locked).toBeChecked();
        await page.waitForTimeout(800);
        await shot(page, "sync-toggle-locked-" + theme);

        // 8. Transfer preflight: the card was dispatched by 老陈, who cannot
        // wake the (private, member-owned) agent; the dialog says why.
        await page.goto("/" + ws.slug + "/issues/" + ids.transferCard, { waitUntil: "domcontentloaded", timeout: 240_000 });
        const prompt = page.locator("[data-slot='alert-dialog-content']");
        // The detail route remounts the properties section once when the issue
        // lands; a picker opened before that remount loses its pending dialog.
        // Retry the gesture after the page settles instead of flaking the run.
        let promptVisible = false;
        for (let attempt = 0; attempt < 4 && !promptVisible; attempt++) {
          await page.waitForTimeout(attempt === 0 ? 4000 : 2000);
          const assigneeRow = page.locator("div.grid-cols-subgrid", {
            has: page.locator("span", { hasText: /^负责人$/ }),
          });
          await expect(assigneeRow).toBeVisible({ timeout: 120_000 });
          await assigneeRow.getByRole("button").first().click();
          await page
            .locator("[data-picker-item]")
            .filter({ hasText: "协调负责人" })
            .first()
            .click({ timeout: 30_000 })
            .catch(() => {});
          await page.waitForTimeout(1500);
          promptVisible = await prompt.isVisible().catch(() => false);
        }
        await expect(prompt).toBeVisible({ timeout: 60_000 });
        await expect(prompt.getByText(/无法叫醒/)).toBeVisible();
        await shot(page, "transfer-prompt-" + theme);
        await page.keyboard.press("Escape");

        // 9. Inbox: the notification for a sub-issue names its parent card.
        await page.goto("/" + ws.slug + "/inbox", { waitUntil: "domcontentloaded", timeout: 240_000 });
        await expect(page.getByText(/属于 CLAWSTA-320 · 发布检查项/)).toBeVisible({ timeout: 120_000 });
        await shot(page, "inbox-parent-line-" + theme);

        // 10. Board: the blocked column carries the reply/act labels.
        await page.goto("/" + ws.slug + "/issues", { waitUntil: "domcontentloaded", timeout: 240_000 });
        // The board scrolls horizontally; the evidence is the blocked column, so
        // bring that column into the viewport before the shot.
        const blockedHeader = page.getByText("已阻塞", { exact: true }).first();
        await expect(blockedHeader).toBeVisible({ timeout: 120_000 });
        await blockedHeader.scrollIntoViewIfNeeded();
        await page.waitForTimeout(600);
        await expect(page.getByText("等你回复").first()).toBeVisible();
        await expect(page.getByText("等你推进").first()).toBeVisible();
        await expect(page.getByText("等 林工").first()).toBeVisible();
        await expect(page.getByText("等 老陈").first()).toBeVisible();
        await shot(page, "board-blocked-labels-" + theme);

        expect(errors).toEqual([]);
      } finally {
        await context.close();
      }
    }
  } finally {
    await api.cleanup();
    await client.end();
  }
});
