/**
 * R0 改版功能基线 · 整页截图脚本（CLAWSTA-133）
 *
 * 对照 docs/redesign/feature-baseline.md 的页面编号，把主要页面在
 * 1440×900 / 1280×800 两种宽度、浅色 / 深色两种主题下各截一张图：
 *
 *     <编号>-<页面>-<宽>-<主题>.png   → e2e/visual/__out__/
 *
 * 运行方式（需要一个已经起好的本地 web + api，普通的
 * `pnpm exec playwright test` 不会收集本目录）：
 *
 *     VISUAL_BASELINE=1 pnpm exec playwright test e2e/visual/baseline.spec.ts
 *
 * 数据由 TestApiClient 在本次运行里新建的独立工作区生成，跑完删除该工作区；
 * 不能指向真实部署或真实数据库（AGENTS.md「Testing」）。
 *
 * 页面列表和编号必须与 docs/redesign/feature-baseline.md 一致：增删页面前先改
 * 清单，再改这里。
 */
import { test, type Browser, type Page } from "@playwright/test";
import { mkdirSync } from "fs";
import { join } from "path";
import { TestApiClient } from "../fixtures";

// playwright test 从仓库根目录运行；cfg 的 testDir 是 ./e2e，所以固定拼
// 根目录下的输出目录，避免 ESM 里没有 __dirname。
const OUTPUT_DIR = join(process.cwd(), "e2e", "visual", "__out__");
const BASE_URL =
  process.env.PLAYWRIGHT_BASE_URL ??
  process.env.FRONTEND_ORIGIN ??
  "http://localhost:3000";
const LOCALE_ORIGIN = new URL(BASE_URL).origin;

/** 截图固定用简体中文：清单里的功能点就是按中文界面核对的。 */
const LOCALE = "zh-Hans";

const VIEWPORTS = [
  { width: 1440, height: 900 },
  { width: 1280, height: 800 },
];
const THEMES = ["light", "dark"] as const;
type Theme = (typeof THEMES)[number];

/** 1×1 PNG，用作任务附件；附件卡片按文件名展示，内容不参与截图。 */
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
  /** 页面编号，对应 docs/redesign/feature-baseline.md 的「截图编号」 */
  id: string;
  /** 文件名里的页面名 */
  name: string;
  /** 测试标题里的中文页面名 */
  label: string;
  route: (fixture: Fixture) => string;
  /** 公开页面（登录 / 注册 / 加入）不注入登录态 */
  authenticated?: boolean;
  /**
   * 页面出现这段文字才算渲染完成。优先用本次种子数据的标题，这样断言和
   * 界面语言无关；只有页面没有专属数据时才用固定文案。
   */
  readyText?: (fixture: Fixture) => string;
  /** 没有可靠文案时的可见选择器 */
  readySelector?: string;
  /** 截图前的交互（滚动、点开某个区块等） */
  prepare?: (page: Page, fixture: Fixture) => Promise<void>;
  /** 注入到 localStorage 的额外状态（比如任务视图模式） */
  storage?: (fixture: Fixture) => Record<string, string>;
  /** 12 号任务详情评论区截滚动后的视口，其余都是整页图 */
  viewportOnly?: boolean;
}

const workspacePath = (fixture: Fixture, path: string) => `/${fixture.workspaceSlug}${path}`;

/**
 * /issues 四种视图模式存在 surface-view-store 的持久化注册表里，键是
 * 「存储名:工作区 slug」，surfaceKey 是 `workspace:all`（范围页签默认值）。
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
  // 数据到手后 React 还可能补一帧布局；给动画和懒加载留一点稳定时间。
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
            // Web 用 cookie 记语言；桌面端适配器读这个键，写一份不会有副作用。
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
  // 配置层已经按 VISUAL_BASELINE=1 过滤本目录；这里再挡一层，避免有人直接
  // 换配置把 visual 用例拉进普通 suite。
  test.skip(
    process.env.VISUAL_BASELINE !== "1",
    "整页截图需要本地服务：VISUAL_BASELINE=1 pnpm exec playwright test e2e/visual/baseline.spec.ts",
  );

  test.beforeAll(async () => {
    mkdirSync(OUTPUT_DIR, { recursive: true });

    const runId = `${Date.now().toString(36)}-${process.pid.toString(36)}`;
    const api = new TestApiClient();
    await api.login(`r0-visual-${runId}@multica.ai`, "R0 视觉基线");
    await api.markUserOnboarded();
    const workspaceName = "R0 视觉基线";
    const workspace = await api.createWorkspace(workspaceName, `r0-visual-${runId}`);

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
    await api.uploadIssueAttachment(
      parent.id,
      "upgrade-plan.png",
      new Uint8Array(ATTACHMENT_PNG),
      "image/png",
    );

    const agentName = "视觉基线助手";
    const runtimeName = "R0 视觉基线机器";
    const { agentId } = await api.seedAgent(agentName, {
      description: "用于 R0 功能基线截图的智能体。",
      runtimeName,
    });
    const skillName = "发布检查清单";
    await api.createSkill(skillName, "上线前逐项核对", "# 发布检查清单\n- 版本号\n- SHA-256");
    const squadName = "发布小组";
    await api.createSquad(squadName, agentId, "负责版本发布与验收。");
    const autopilotTitle = "每周发布摘要";
    await api.createAutopilot(autopilotTitle, agentId);

    const inboxItemTitle = "请评审统一发布流程";
    await api.seedInboxItems([
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

    const shareLink = await api.createShareLink();

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
    if (!fixture) return;
    try {
      // 先按依赖顺序清理（squad 的 leader 对 agent 是 RESTRICT），再删工作区。
      await fixture.api.cleanup();
      await fixture.api.deleteWorkspace();
    } catch (error) {
      // 清理失败不能吞掉截图结果，但要在日志里留痕，方便下次手动清工作区。
      console.warn("视觉基线清理失败，请手动删除工作区", fixture.workspaceSlug, error);
    }
  });

  for (const definition of PAGES) {
    test(`${definition.id} ${definition.label}`, async ({ browser }) => {
      await captureDefinition(browser, definition, fixture);
    });
  }
});
