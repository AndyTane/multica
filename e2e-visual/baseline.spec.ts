/**
 * R0 redesign feature baseline: full-page screenshots (CLAWSTA-133).
 *
 * For every page id in docs/redesign/feature-baseline.md, capture one
 * full-page screenshot at 1440x900 and 1280x800, in both the light and the
 * dark theme:
 *
 *     <id>-<page>-<width>-<theme>.png   -> e2e-visual/__out__/
 *
 * How to run (needs an already running local web + api):
 *
 *     pnpm exec playwright test -c playwright.visual.config.ts
 *
 * The config lives outside playwright.config.ts on purpose: the upstream test
 * config stays untouched, and its testDir (./e2e) never collects this folder.
 * Targeting one page:
 *
 *     pnpm exec playwright test -c playwright.visual.config.ts -g "07 任务-看板"
 *
 * Login, workspace, issues, projects and comments come from the existing
 * TestApiClient (e2e/fixtures.ts); the visual-only rows (share link,
 * attachment, agent, runtime, inbox items, skill, squad, autopilot) come from
 * ./fixtures. The data is created in a workspace this run owns and deletes
 * afterwards. Never point this at a real deployment or a real database
 * (AGENTS.md "Testing").
 *
 * Page ids and list order must match docs/redesign/feature-baseline.md: when a
 * page is added or removed, update the checklist first and this file second.
 */
import { test, type Browser, type Page } from "@playwright/test";
import { mkdirSync } from "fs";
import { join } from "path";
import { TestApiClient } from "../e2e/fixtures";
import { VisualBaselineData } from "./fixtures";

// `playwright test` runs from the repo root, so anchor the output directory
// to the root rather than __dirname: ESM has no __dirname.
const OUTPUT_DIR = join(process.cwd(), "e2e-visual", "__out__");
const BASE_URL =
  process.env.PLAYWRIGHT_BASE_URL ??
  process.env.FRONTEND_ORIGIN ??
  "http://localhost:3000";
const LOCALE_ORIGIN = new URL(BASE_URL).origin;

/** Screenshots always use Simplified Chinese: the checklist is verified against the Chinese UI. */
const LOCALE = "zh-Hans";

const VIEWPORTS = [
  { width: 1440, height: 900 },
  { width: 1280, height: 800 },
];
const THEMES = ["light", "dark"] as const;
type Theme = (typeof THEMES)[number];

/** 1x1 PNG used as an issue attachment; the attachment card shows its filename only. */
const ATTACHMENT_PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
  "base64",
);

interface Fixture {
  api: TestApiClient;
  workspaceSlug: string;
  workspaceName: string;
  shareCode: string;
  parentIssueId: string;
  parentIssueTitle: string;
  projectId: string;
  projectTitle: string;
  agentId: string;
  agentName: string;
  runtimeName: string;
  skillName: string;
  squadName: string;
  autopilotTitle: string;
  inboxItemTitle: string;
  commentText: string;
}

interface PageDefinition {
  /** Page id, matching the "screenshot id" column in docs/redesign/feature-baseline.md */
  id: string;
  /** Page name used in the screenshot filename */
  name: string;
  /** Chinese page name used in the test title */
  label: string;
  route: (fixture: Fixture) => string;
  /** Public pages (login / register / join) get no auth state */
  authenticated?: boolean;
  /**
   * The page is considered rendered once this text is on screen. Prefer a
   * title from the seeded data so the assertion does not depend on the UI
   * language; fixed copy is only for pages without their own data.
   */
  readyText?: (fixture: Fixture) => string;
  /** Visible selector used when there is no reliable text */
  readySelector?: string;
  /** Interaction before the screenshot (scroll, reveal a block, ...) */
  prepare?: (page: Page, fixture: Fixture) => Promise<void>;
  /** Extra localStorage state (for example the issue view mode) */
  storage?: (fixture: Fixture) => Record<string, string>;
  /** Page 12 screenshots the viewport scrolled to the comments; everything else is full page */
  viewportOnly?: boolean;
}

const workspacePath = (fixture: Fixture, path: string) => `/${fixture.workspaceSlug}${path}`;

/**
 * The four /issues view modes live in the surface-view-store persistence
 * registry, keyed by "<storage name>:<workspace slug>", with the surface key
 * `workspace:all` (the default scope tab).
 */
function issueSurfaceStorage(workspaceSlug: string, viewMode: string): Record<string, string> {
  const storageKey = `multica_issue_surface_views:${workspaceSlug}`;
  return {
    [storageKey]: JSON.stringify({
      state: {
        surfaces: {
          "workspace:all": { state: { viewMode }, updatedAt: new Date().toISOString() },
        },
      },
      version: 1,
    }),
  };
}

const PAGES: PageDefinition[] = [
  {
    id: "01",
    name: "join",
    label: "加入页",
    route: (fixture) => `/join?code=${fixture.shareCode}`,
    authenticated: false,
    readyText: (fixture) => fixture.workspaceName,
  },
  {
    id: "02",
    name: "login",
    label: "登录页",
    route: () => "/login",
    authenticated: false,
    readySelector: 'input[type="email"], input[placeholder]',
  },
  {
    id: "03",
    name: "register",
    label: "注册页",
    route: () => "/register",
    authenticated: false,
    readySelector: 'input[type="email"], input[placeholder]',
  },
  {
    id: "04",
    name: "inbox",
    label: "收件箱-列表",
    route: (fixture) => workspacePath(fixture, "/inbox"),
    readyText: (fixture) => fixture.inboxItemTitle,
  },
  {
    id: "05",
    name: "inbox-item",
    label: "收件箱-选中一条",
    route: (fixture) => workspacePath(fixture, `/inbox?issue=${fixture.parentIssueId}`),
    readyText: (fixture) => fixture.inboxItemTitle,
  },
  {
    id: "06",
    name: "my-issues",
    label: "我的任务",
    route: (fixture) => workspacePath(fixture, "/my-issues"),
    readyText: (fixture) => fixture.parentIssueTitle,
  },
  {
    id: "07",
    name: "issues-board",
    label: "任务-看板",
    route: (fixture) => workspacePath(fixture, "/issues"),
    readyText: (fixture) => fixture.parentIssueTitle,
    storage: (fixture) => issueSurfaceStorage(fixture.workspaceSlug, "board"),
  },
  {
    id: "08",
    name: "issues-list",
    label: "任务-列表",
    route: (fixture) => workspacePath(fixture, "/issues"),
    readyText: (fixture) => fixture.parentIssueTitle,
    storage: (fixture) => issueSurfaceStorage(fixture.workspaceSlug, "list"),
  },
  {
    id: "09",
    name: "issues-table",
    label: "任务-表格",
    route: (fixture) => workspacePath(fixture, "/issues"),
    readyText: (fixture) => fixture.parentIssueTitle,
    storage: (fixture) => issueSurfaceStorage(fixture.workspaceSlug, "table"),
  },
  {
    id: "10",
    name: "issues-swimlane",
    label: "任务-泳道",
    route: (fixture) => workspacePath(fixture, "/issues"),
    readyText: (fixture) => fixture.parentIssueTitle,
    storage: (fixture) => issueSurfaceStorage(fixture.workspaceSlug, "swimlane"),
  },
  {
    id: "11",
    name: "issue-detail-top",
    label: "任务详情-顶部",
    route: (fixture) => workspacePath(fixture, `/issues/${fixture.parentIssueId}`),
    readyText: (fixture) => fixture.parentIssueTitle,
  },
  {
    id: "12",
    name: "issue-detail-comments",
    label: "任务详情-评论区",
    route: (fixture) => workspacePath(fixture, `/issues/${fixture.parentIssueId}`),
    readyText: (fixture) => fixture.commentText,
    prepare: async (page) => {
      await page.getByTestId("comment-composer-shell").scrollIntoViewIfNeeded();
      await page.waitForTimeout(300);
    },
    viewportOnly: true,
  },
  {
    id: "13",
    name: "projects",
    label: "项目列表",
    route: (fixture) => workspacePath(fixture, "/projects"),
    readyText: (fixture) => fixture.projectTitle,
  },
  {
    id: "14",
    name: "project-detail",
    label: "项目详情",
    route: (fixture) => workspacePath(fixture, `/projects/${fixture.projectId}`),
    readyText: (fixture) => fixture.projectTitle,
  },
  {
    id: "15",
    name: "agents",
    label: "智能体列表",
    route: (fixture) => workspacePath(fixture, "/agents"),
    readyText: (fixture) => fixture.agentName,
  },
  {
    id: "16",
    name: "agent-detail",
    label: "智能体详情",
    route: (fixture) => workspacePath(fixture, `/agents/${fixture.agentId}`),
    readyText: (fixture) => fixture.agentName,
  },
  {
    id: "17",
    name: "agent-new",
    label: "新建智能体",
    route: (fixture) => workspacePath(fixture, "/agents/new"),
    readyText: () => "创建智能体",
  },
  {
    id: "18",
    name: "squads",
    label: "小队",
    route: (fixture) => workspacePath(fixture, "/squads"),
    readyText: (fixture) => fixture.squadName,
  },
  {
    id: "19",
    name: "skills",
    label: "Skills",
    route: (fixture) => workspacePath(fixture, "/skills"),
    readyText: (fixture) => fixture.skillName,
  },
  {
    id: "20",
    name: "runtimes",
    label: "运行时",
    route: (fixture) => workspacePath(fixture, "/runtimes"),
    readyText: (fixture) => fixture.runtimeName,
  },
  {
    id: "21",
    name: "autopilots",
    label: "自动化",
    route: (fixture) => workspacePath(fixture, "/autopilots"),
    readyText: (fixture) => fixture.autopilotTitle,
  },
  {
    id: "22",
    name: "usage",
    label: "统计",
    route: (fixture) => workspacePath(fixture, "/usage"),
    readyText: () => "暂无消耗数据",
  },
  {
    id: "23",
    name: "settings-profile",
    label: "设置-个人",
    route: (fixture) => workspacePath(fixture, "/settings?tab=profile"),
    readyText: () => "个人资料",
  },
  {
    id: "24",
    name: "settings-workspace",
    label: "设置-工作区",
    route: (fixture) => workspacePath(fixture, "/settings?tab=workspace"),
    readyText: () => "常规",
  },
  {
    id: "25",
    name: "settings-channels",
    label: "设置-连接",
    route: (fixture) => workspacePath(fixture, "/settings?tab=channels"),
    readyText: () => "消息渠道",
  },
];

let fixture: Fixture;
let visual: VisualBaselineData;

async function waitForReady(
  page: Page,
  definition: PageDefinition,
  f: Fixture,
): Promise<void> {
  await page.waitForLoadState("domcontentloaded");
  await page.waitForFunction(
    () => (document.body?.innerText ?? "").trim().length > 0,
    undefined,
    { timeout: 30_000 },
  );
  if (definition.authenticated !== false) {
    await page
      .locator('[data-slot="sidebar"]')
      .first()
      .waitFor({ state: "visible", timeout: 30_000 });
  }
  if (definition.readySelector) {
    await page
      .locator(definition.readySelector)
      .first()
      .waitFor({ state: "visible", timeout: 30_000 });
  }
  if (definition.readyText) {
    const text = definition.readyText(f);
    await page.waitForFunction(
      (expected) => document.body.innerText.includes(expected),
      text,
      { timeout: 30_000 },
    );
  }
  await page.waitForFunction(() => document.fonts.status === "loaded").catch(() => undefined);
  // React may still settle one more layout pass after the data arrives; give animations and lazy content a moment.
  await page.waitForTimeout(500);
}

async function captureDefinition(
  browser: Browser,
  definition: PageDefinition,
  f: Fixture,
): Promise<void> {
  for (const viewport of VIEWPORTS) {
    for (const theme of THEMES) {
      const filename = `${definition.id}-${definition.name}-${viewport.width}-${theme}.png`;
      const context = await browser.newContext({
        baseURL: BASE_URL,
        viewport,
        deviceScaleFactor: 1,
        locale: "zh-CN",
        colorScheme: theme,
      });
      try {
        await context.addCookies([
          { name: "multica-locale", value: LOCALE, url: LOCALE_ORIGIN },
        ]);
        const page = await context.newPage();
        const storage = definition.storage?.(f) ?? {};
        const token = definition.authenticated === false ? null : (f.api.getToken() ?? null);
        await page.addInitScript(
          ({ theme, token, storage }) => {
            localStorage.setItem("theme", theme);
            // Web stores the locale in a cookie; the desktop adapter reads this key, so writing it is harmless.
            localStorage.setItem("multica-locale", "zh-Hans");
            if (token) {
              localStorage.setItem("multica_token", token);
              localStorage.setItem("multica:chat:isOpen", "false");
            }
            for (const [key, value] of Object.entries(storage)) {
              localStorage.setItem(key, value);
            }
          },
          { theme, token, storage } as { theme: Theme; token: string | null; storage: Record<string, string> },
        );
        await page.goto(definition.route(f), { waitUntil: "domcontentloaded" });
        await waitForReady(page, definition, f);
        if (definition.prepare) {
          await definition.prepare(page, f);
        }
        await page.screenshot({
          path: join(OUTPUT_DIR, filename),
          fullPage: !definition.viewportOnly,
          animations: "disabled",
        });
      } finally {
        await context.close();
      }
    }
  }
}

test.describe("R0 视觉基线截图", () => {
  test.beforeAll(async () => {
    mkdirSync(OUTPUT_DIR, { recursive: true });

    const runId = `${Date.now().toString(36)}-${process.pid.toString(36)}`;
    const api = new TestApiClient();
    await api.login(`r0-visual-${runId}@multica.ai`, "R0 视觉基线");
    await api.markUserOnboarded();
    const workspaceName = "R0 视觉基线";
    const workspace = await api.createWorkspace(workspaceName, `r0-visual-${runId}`);
    visual = new VisualBaselineData(api, { id: workspace.id, slug: workspace.slug });

    const parentIssueTitle = "统一发布流程";
    const parent = await api.createIssue(parentIssueTitle, {
      status: "in_progress",
      priority: "high",
      description: [
        "**目标**：把服务端、客户端和文档的发布收敛成一条流水线。",
        "",
        "- 版本号只在打 tag 时确定",
        "- 差量包只从上一个已发布版本起步",
        "- 先校验 SHA-256，再切 channels/stable/latest.json",
      ].join("\n"),
    });
    const project = await api.createProject("UWorkers 前端改版", {
      description: "改版 A′ 方向的功能基线留档与逐项核对。",
    });

    await api.createIssue("补齐差量升级校验", {
      parent_issue_id: parent.id,
      status: "todo",
      priority: "medium",
    });
    const otherIssues = [
      { title: "产品需求评审", status: "backlog", priority: "low" },
      { title: "设计令牌梳理", status: "in_progress", priority: "high", projectId: project.id },
      { title: "看板拖拽性能", status: "todo", priority: "medium", projectId: project.id },
      { title: "评论折叠交互", status: "in_review", priority: "medium" },
      { title: "智能体运行时接入", status: "in_progress", priority: "high" },
      { title: "发布说明文案", status: "done", priority: "low" },
      { title: "数据统计口径", status: "in_review", priority: "medium" },
      { title: "登录页视觉", status: "blocked", priority: "urgent" },
      { title: "旧版首页清理", status: "done", priority: "none" },
      { title: "移动端适配", status: "backlog", priority: "low" },
      { title: "无效需求（示例）", status: "cancelled", priority: "none" },
    ];
    for (const issue of otherIssues) {
      await api.createIssue(issue.title, {
        status: issue.status,
        priority: issue.priority,
        ...(issue.projectId ? { project_id: issue.projectId } : {}),
      });
    }

    await api.createProject("发布与分发", {
      description: "便携包、差量升级包与 Client-Kit 的发布记录。",
    });

    const commentText = "评审结论：按 A 方案推进，分六个阶段落地。";
    await api.createComment(parent.id, commentText);
    await api.createComment(
      parent.id,
      ["**阶段 0 基线**", "", "1. 功能清单落成文件", "2. 主要页面截图存档", "3. 补齐关键流程的 e2e"].join("\n"),
    );
    await api.createComment(
      parent.id,
      "差量包只适用于对应的起始版本，不能跨版本升级；公司电脑先 Stop.cmd 再换 Client-Kit。",
    );
    await visual.uploadIssueAttachment(
      parent.id,
      "upgrade-plan.png",
      new Uint8Array(ATTACHMENT_PNG),
      "image/png",
    );

    const agentName = "视觉基线助手";
    const runtimeName = "R0 视觉基线机器";
    const { agentId } = await visual.seedAgent(agentName, {
      description: "用于 R0 功能基线截图的智能体。",
      runtimeName,
    });
    const skillName = "发布检查清单";
    await visual.createSkill(skillName, "上线前逐项核对", "# 发布检查清单\n- 版本号\n- SHA-256");
    const squadName = "发布小组";
    await visual.createSquad(squadName, agentId, "负责版本发布与验收。");
    const autopilotTitle = "每周发布摘要";
    await visual.createAutopilot(autopilotTitle, agentId);

    const inboxItemTitle = "请评审统一发布流程";
    await visual.seedInboxItems([
      {
        title: inboxItemTitle,
        body: "统一发布流程进入评审，等你确认差量包的起始版本。",
        issueId: parent.id,
        severity: "action_required",
        type: "issue_assigned",
      },
      {
        title: "设计令牌梳理更新",
        body: "444 个组件里只有 22 个写死颜色，令牌换肤可以覆盖九成以上。",
        severity: "attention",
        type: "issue_comment",
      },
      {
        title: "发布说明文案已完成",
        body: "v0.2.7 的发布说明已归档。",
        severity: "info",
        type: "issue_status_changed",
        read: true,
      },
    ]);

    const shareLink = await visual.createShareLink();

    fixture = {
      api,
      workspaceSlug: workspace.slug,
      workspaceName,
      shareCode: shareLink.code,
      parentIssueId: parent.id,
      parentIssueTitle,
      projectId: project.id,
      projectTitle: project.title,
      agentId,
      agentName,
      runtimeName,
      skillName,
      squadName,
      autopilotTitle,
      inboxItemTitle,
      commentText,
    };
  });

  test.afterAll(async () => {
    if (!fixture || !visual) return;
    try {
      // Clean up in dependency order: visual-only rows first (a squad's leader
      // is a RESTRICT reference to its agent), then the client's issues and
      // projects, then the workspace itself.
      await visual.cleanup();
      await fixture.api.cleanup();
      await fixture.api.deleteWorkspace();
    } catch (error) {
      // A cleanup failure must not hide the screenshots, but log it so a leftover workspace can be removed by hand.
      console.warn("视觉基线清理失败，请手动删除工作区", fixture.workspaceSlug, error);
    }
  });

  for (const definition of PAGES) {
    test(`${definition.id} ${definition.label}`, async ({ browser }) => {
      await captureDefinition(browser, definition, fixture);
    });
  }
});
