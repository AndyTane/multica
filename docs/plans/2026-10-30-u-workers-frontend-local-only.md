# U Workers 前端与隐私改造计划

> For implementers: 使用 executing-plans 按任务执行，遵守本仓库 AGENTS.md。本文件沿用初稿文件名，范围以本文为准：模型请求不受限制，不实施全局断网。

**Goal:** 产品显示名统一为 U Workers，移除 Multica 官方云站关联及非用户行为触发的数据外发，保留模型请求和用户授权的联网操作。

**Architecture:** 沿用 views → core + ui 和各平台适配层；清理浏览器、Node/Go、Electron 主进程、守护进程的隐性外发及官方云服务装配。业务数据默认本地存储，模型与集成按用户配置运行。

**Tech Stack:** React、Next.js、Electron、TanStack Query、Zustand、Go、PostgreSQL、本地文件存储。

---

## 1. 用户要求与交付范围

- 独立副本：D:/01_Projects/u-workers；来源：D:/01_Projects/multica-fork。
- 复制提交、原有工作区修改校验见同目录 2026-10-30-u-workers-copy-receipt.json。
- 页面、Logo、窗口、PWA、托盘、菜单、通知、安装显示名统一 U Workers。
- 移除 multica.ai 及官方云登录、云运行时、计费、营销、帮助跳转、默认地址和更新渠道的关联模块。
- 用户最新澄清：**模型请求不受限制，要限制的是非用户行为的外发数据。**
- 本版取代初稿的“禁用公网模型、所有 Agent 子进程断网、删除全部第三方集成”要求。
- 业务数据默认本地保存；公网模型请求会发送任务所需上下文，这是允许的工作方式，不宣称所有数据绝不离开本机。
- 原项目、现有服务、数据库、DSH 配置和运行中任务保持原状。
- 本次完成独立源码复制和计划，不实施页面改动、不安装依赖、不启动部署、不调用真实模型。

## 2. 外发行为边界

### 2.1 保留的能力

| 行为 | 处理原则 |
| --- | --- |
| DeepSeek、OpenAI、Anthropic 等模型 API | 保留，不因本次改造新增域名、地域、模型地址或公网请求白名单限制 |
| DSH、Claude、Codex、Hermes 等运行时 | 保留，不要求本地模型、离线容器或全局断网 |
| 流式连接、模型认证、SDK 重试 | 正常模型请求的一部分，不逐次增加授权弹窗 |
| 用户任务中的搜索、MCP、Git fetch/push、上传 | 在现有任务授权和权限范围内保留，不因使用子进程联网而一律拒绝 |
| 用户主动配置的第三方登录、邮件、代码托管、Composio、渠道或存储 | 保留对应能力，不默认绑定上游运营账号，不扩大现有授权范围 |
| 用户建立的定时任务、持续订阅或集成 | 在已经授权的目标、数据范围和生命周期内执行，不每次轮询重新询问 |
| 用户点击外链、加载资源、检查更新或安装工具 | 可主动执行，目的地和作用清楚；Multica 官方服务关联仍须移除 |

后台执行本身不等于非用户行为。已授权的持续工作与正常重试仍然属于用户工作流，不为隐私改造重新限制既有模型请求。

### 2.2 移除的行为

- PostHog、产品统计、行为跟踪、自动崩溃/异常上传、自托管遥测、注册来源与用户画像。
- 启动、登录、切页、定时器触发的上游 heartbeat、营销、版本/设备上报及使用情况同步。
- 缺配置时回退 api.multica.ai、官方云登录、云网关、云计费或官方 Cloud Runtime。
- 未经用户配置或启用的第三方 worker、渠道、webhook、模型探测、远程上传和同步。
- 页面自动加载的追踪像素、第三方头像、图片、iframe 和链接预览；需要时提供明确的用户加载操作。
- 启动或后台定时检查/下载更新、自动拉取 release 或安装脚本；保留用户主动检查与安装。

用户点击“运行任务”不构成向统计、广告或 Multica 运营端发送任务/设备/账号数据的授权。遥测不能混入模型请求；环境变量不能重新启用已关闭的上报。

### 2.3 判定与技术原则

- 依据既有会话、task/run、集成配置、订阅、定时任务和调用来源判断归属，不信任任意客户端 userInitiated 标志。
- 已授权集成可持续执行，配置和停用方式可见；撤销后停止其后台工作。
- 无法归属的产品自动外发应删除触发、清理装配并补回归。
- 不使用覆盖全部模型 SDK、Agent 工具或自定义服务地址的全局出站拒绝策略。
- 分别观察浏览器、Node/Go、Electron 主进程和 Agent 请求，不能只测浏览器。
- 仍保留现有认证、授权和外链安全措施；此改造不扩大任务权限。
- 普通回归均使用假协议/本地假服务，不擅自启动真实 Agent 或消耗模型额度。

## 3. 已核实入口

以下路径相对于新项目根目录；是已核实的首批入口，并非穷尽清单。

| 范围 | 文件/目录 | 改造方向 |
| --- | --- | --- |
| 隐私策略 | packages/core/analytics/privacy.ts；server/internal/privacy/reporting.go | 当前已在编译期关闭外部上报，保留 gate 并清理死依赖，不误称现在仍在发送 |
| Desktop 地址回退 | apps/desktop/src/shared/runtime-config.ts；apps/desktop/src/main/runtime-config-loader.ts | 缺配置不连官方云；使用用户的自托管配置或明确提示 |
| Web API/文档代理 | apps/web/config/runtime-urls.ts；apps/web/next.config.ts；apps/web/proxy.ts | 不默认代理官方云，保留用户配置服务与鉴权，不限于 loopback |
| 品牌与元数据 | apps/web/app/layout.tsx；apps/web/app/manifest.ts；apps/web/platform/document-title.ts | 替换品牌、官方 canonical/social 关联，字体随包提供 |
| Logo/应用壳 | packages/ui/components/common/multica-icon.tsx；packages/views/layout/app-sidebar.tsx；workspace-loader.tsx | 新共用 U Workers 标识 |
| 官方云节点 | packages/views/runtimes/components/runtimes-page.tsx；cloud-runtime-dialog.tsx；packages/core/runtimes/cloud-runtime.ts | 移除官方云产品入口，保留普通运行时和公网模型 |
| 登录与引导 | packages/views/auth/login-page.tsx；packages/views/onboarding/steps/step-platform-fork.tsx；components/cloud-waitlist-expand.tsx | 去官方云登录/候补名单；保留主动配置的认证 |
| 帮助与外链 | packages/views/layout/help-launcher.tsx；join-discord-card.tsx；packages/views/platform/open-external.ts | 去官方运营入口，内置帮助，普通主动外链仍可用 |
| 更新与安装 | apps/desktop/src/main/updater.ts；cli-bootstrap.ts；server/internal/daemon/auto_update.go；server/internal/cli/update.go | 停止自动检查/下载，保留真实来源的主动操作 |
| 后端云/集成 | server/cmd/server/router.go；server/internal/cloudruntime/；server/internal/handler/cloud_runtime.go；cloud_billing.go；server/pkg/composio/ | 去官方云装配，保留用户集成，无配置不探测 |
| 存储 | server/internal/storage/local.go；s3.go；server/internal/auth/cloudfront.go | 默认本地，去官方存储依赖，不一刀切删除用户配置外部存储 |
| 营销页 | apps/web/features/landing/components/；apps/web/features/landing/utils/github-release.ts | U Workers 入口，去官方价格、销售、候补名单、社交及自动 release 查询 |

## 4. 页面方案

保留现有高密度工作台，首轮不重写任务交互。

| 页面 | 目标 | 验收重点 |
| --- | --- | --- |
| 根页面 / | 未登录进入自托管登录，已登录进入工作区 | 不显示原 SaaS 营销页，不硬编码工作区 slug |
| 登录 | 登录 U Workers，使用用户配置的认证 | 不回退官方云，保留 Google/邮件等主动配置能力，不删除后留下不可用登录 |
| 上手引导 | 服务 → 工作区 → 运行时/模型 → 任务 | 去官方候补名单，本地或公网模型均可配置 |
| 侧边栏 | 新字标，保留任务、项目、智能体、小队、收件箱、运行时、用量、设置 | 去官方云、购买、升级与运营推广 |
| 任务列表/详情 | 保留看板、评论、附件、运行记录与审阅 | 不改状态语义，不自动加载第三方追踪资源 |
| 智能体/运行时 | 保留 DSH/Claude/Codex/Hermes 与自定义模型配置 | 不强制本地模型或断网，不删除公网地址 |
| 聊天 | 保留所选模型、流式、重试、恢复 | 模型请求不受本次策略限制，不偷偷切换模型 |
| 设置 | 保留自托管、用户集成、模型、用量、备份、关于 | 去官方云计费/节点/账号绑定，不批量删除 Apps/Channels |
| 帮助/关于 | 内置帮助、版本、本地诊断导出 | 不自动上报/查更新，保留主动操作 |
| Desktop | 窗口、托盘、菜单、错误页、安装名同步 | renderer/主进程都覆盖，和原部署数据隔离 |

旧官方云/计费深链显示本地不可用说明与返回工作区操作，不白屏、不跳官网。普通外链、用户集成深链按原有权限处理。

### 4.1 视觉与文案

- 新增 packages/ui/components/common/product-mark.tsx，使用代码原生 SVG 的 UW 标识和 U Workers 字标，适配 16/24/32px 与浅深色。
- 纯品牌常量放 packages/core/config/brand.ts；core 不引入 UI/环境变量/浏览器 API，ui 不依赖 core，由应用 props/slots 装配。
- 沿用 packages/ui/styles/tokens.css 的角色字号、语义色和 4/8/12/16/24/32px 间距，不在此轮大改全局几何。
- 优先本地 Inter + 系统中文字体、系统等宽代码字体；备选全系统 UI 字体或本地 Geist，不增加页面在线字体依赖。
- 按 1440、1024、390px 检查长名、折叠、滚动与浮层；覆盖 hover、选中+hover、focus、disabled、pending、空与失败状态。
- 保留键盘、可访问名称、焦点恢复、reduced-motion，遵守 conventions 与 Button/Dialog 文档，description 默认省略。
- 不显示“完全离线”“所有数据绝不出本机”。必要时说明“业务数据默认保存在本地；模型与集成按你的配置运行”，不在每页重复。

## 5. 品牌与迁移边界

替换所有用户可见品牌、官方域名生成的分享地址、官方运营服务入口及所有支持语言文案。

不全仓替换 @multica/*、Go module、DB/API 标识、storage key、CLI 字面命令、旧数据、提交历史或来源声明。副本使用独立 DB、userData、daemon profile 和端口，不因保留内部名称读写原部署。

安装指引用真实二进制或用户选择的来源，不编造 u-workers CLI，也不把原官方发布渠道伪装成 U Workers 更新服务。

## 6. 分批实施任务

新增行为先写失败回归，文案/资源替换用现有测试和视觉检查。每批只提交自己的文件，不夹带复制来的未提交修改。

### Task 1：用户行为与隐性外发边界（P0）

**Files:** AGENTS.md；packages/core/analytics/privacy.ts、privacy.test.ts、privacy-network.test.ts；server/internal/privacy/reporting.go 与 analytics/privacy 测试。Create docs/outbound-data-policy.md。

1. 清点每项请求的触发、目标、数据与用户授权归属。
2. 保留现有编译期 gate，测试旧环境变量不能重开上报。
3. 增加模型流式/SDK 重试、用户任务工具调用、已授权集成仍可用的回归。
4. 更新新项目 AGENTS：禁隐性上报，保留模型与用户联网，不能强制断网或逐次模型授权。

**Verify:** pnpm --filter @multica/core test -- analytics/privacy.test.ts analytics/privacy-network.test.ts；server 目录 go test ./internal/privacy ./internal/analytics -count=1。均使用 fake 服务。

**Commit:** docs(privacy): define user-directed network boundaries。

### Task 2：官方云回退与非用户外发装配（P0）

**Files:** 上述 Desktop runtime-config、Web runtime-urls/next.config/proxy、server/cmd/server/router.go、server/internal/cloudruntime/、selfhosttelemetry/、analytics/ 及对应测试。

1. 为缺配置不连官方云、启动/登录/切页不自动上报写失败测试。
2. 去官方 API/WS/文档/登录回退，保留用户配置的自托管服务与鉴权。
3. 删除官方云控制面、遥测、营销和未授权 worker 的装配与死依赖。
4. 集成/存储无用户配置不初始化/探测，有配置按已授权生命周期运行。
5. 保留已有用户配置，不修改原部署或正式数据库。

**Verify:** 配置、代理、startup/handler 测试，fake 官方云/统计请求为 0，用户配置服务可正常访问。

**Commit:** feat(privacy): remove upstream fallbacks and unsolicited reporting。

### Task 3：登录与上手引导（P1）

**Files:** packages/views/auth/login-page.tsx；apps/web/app/(auth)/login/page.tsx；packages/views/onboarding/onboarding-flow.tsx、steps/step-platform-fork.tsx、steps/step-runtime-connect.tsx、components/cloud-waitlist-expand.tsx、index.ts 及测试。

1. 核实邮件/Google 依赖，保留用户主动配置的认证，不只删按钮。
2. 去官方云绑定、候补名单、自动营销，替换 U Workers 文案。
3. 保留运行时、模型配置、用户主动工具安装，不强制本地模型。
4. 复用现有 session/PAT/成员权限；loopback 不免认证，无需求不另建认证协议。
5. 确有新增 API/路由时再更新 schema、畸形响应测试与 reserved slugs。

**Verify:** 登录/退出、错误码、CLI 回调、无运行时、公网模型配置测试；fake 认证，不发真实邮件。

**Commit:** feat(onboarding): remove upstream cloud and marketing flows。

### Task 4：U Workers 品牌（P1）

**Files:** Create packages/core/config/brand.ts、packages/ui/components/common/product-mark.tsx；Modify sidebar、workspace-loader、Web layout/manifest/document-title/favicon；apps/desktop/package.json、electron-builder.yml、src/main/index.ts、src/main/renderer-recovery.ts；packages/views/locales 下所有支持语言。

1. 接入品牌常量/SVG，遵守包边界。
2. 替换页面、窗口、托盘、菜单、图标、PWA 与安装显示名。
3. 移除官方 canonical/social metadata。
4. 独立安装身份、userData、daemon profile，不自动迁移旧数据。
5. 检查语言插值、长度、换行、可访问名称。

**Verify:** 复用 sidebar/login/manifest/document-title 测试，Web/Desktop 浅深色与窄屏检查；逐项区分品牌残留和内部/来源引用。

**Commit:** feat(brand): present U Workers across web and desktop。

### Task 5：移除官方云页面，保留用户集成（P1）

**Files:** 上述 runtimes-page/cloud-runtime-dialog/core cloud-runtime 与 exports；apps/web/app/[workspaceSlug]/(dashboard)/runtimes/page.tsx、billing/page.tsx、apps/web/app/billing/return/page.tsx；packages/views/settings/components/settings-page.tsx、settings-layout.tsx、settings-navigation.ts、billing-tab.tsx、搜索索引；help-launcher、join-discord-card、Desktop router/菜单及测试。

1. 先测试导航/搜索不含官方云、计费和官网推广。
2. 删除官方云按钮/弹窗/死导出，保留普通运行时与模型配置。
3. 保留 Apps/Channels、用户集成、本地用量，不与遥测一并删除。
4. Web/Desktop 同步处理旧官方云深链，不能触发上游请求。
5. 帮助本地化，普通主动外链使用原有安全处理，检查键盘/焦点。

**Verify:** pnpm --filter @multica/views test -- layout/app-sidebar.test.tsx settings/components/settings-navigation.test.ts settings/components/settings-page.test.tsx；再跑实际修改的运行时/平台路由测试，验证用户集成和模型选择不回归。

**Commit:** feat(ui): remove upstream cloud surfaces and preserve integrations。

### Task 6：隐性资源外联与自动下载（P0）

**Files:** Web layout、landing、next.config/proxy；packages/views/editor/、packages/core/workspace/avatar-url.ts；Desktop external-url/updater/cli-bootstrap/electron-builder；server/internal/daemon/auto_update.go、server/internal/cli/update.go、server/cmd/multica/cmd_update.go 与安装模板。

1. 字体、默认头像、Logo、帮助随包提供；区分构建依赖安装与产品运行时外发。
2. 页面不自动加载第三方追踪/嵌入/预览，保留用户明确加载操作和已授权集成资源。
3. 去启动/定时更新检查、自动安装和 release 查询，保留用户主动检查/下载安装。
4. 移除官方更新关联；没有新发布源时使用本地包或用户指定的真实来源。
5. 覆盖 renderer、主进程、Go，不能全局封禁模型、OAuth、Git、MCP、主动下载或 openExternal。

**Verify:** 浏览时 fake 追踪/更新服务零请求，主动加载、更新、模型/集成 fake 请求可用，不调真实模型或下载真实安装包。

**Commit:** feat(privacy): remove implicit requests and automatic downloads。

### Task 7：移动端发布范围（发布门槛）

**Files:** Read apps/mobile/AGENTS.md；审查 apps/mobile/ 独立品牌、翻译、配置、认证与触发点。

1. 首轮主交付 Web/Windows Desktop，不默认宣称移动端已完成。
2. 发布移动端时同步品牌、官方回退清理、非用户外发关闭，保留模型/授权联网，不复用 Web store/UI。
3. 不发布则排除移动产物并列出未验证项。

**Verify:** mobile AGENTS 的 typecheck/lint/test 与设备验证，根检查不覆盖移动端。

**Commit:** chore(mobile): define privacy release scope，或实际功能提交。

### Task 8：独立环境与网络行为验收（P0）

**Files:** Create e2e/background-network.spec.ts、e2e/u-workers-brand.spec.ts、docs/outbound-data-verification.md；更新 CONTRIBUTING.md、部署说明与实际改变的 AGENTS 流程。

1. 独立数据库/端口/数据目录，不复制原 .env 或部署凭据；需要 worktree 时遵循既有环境脚本。
2. 启动、登录、浏览、切工作区、静置场景：无非用户外发，无官方云/统计请求。
3. 主动 fake 模型流式/重试、Agent 工具、MCP、Git、集成场景：请求成功，可归属用户授权，无附带遥测。
4. 授权订阅/定时任务持续可用，未配置集成不启动，撤销后停止，不能误删所有后台联网。
5. 分别观察浏览器 request/WebSocket/service worker/导航和 Node、Go、Electron 请求。
6. 网络记录只保留必要目标/触发/结果，不保存正文、代码、令牌或真实业务抓包；模型请求不要求零流量。
7. 断网测试本地任务管理/附件；公网模型报网络错误即可，不要求全部流程离线。
8. 真实 Agent 仍需 agentintegration 和显式测试授权，普通回归仅用假服务。

**Commands:** pnpm typecheck、pnpm lint、pnpm test、pnpm test:dsh-web；Go 先测试实际变更包，再按风险 make test；pnpm exec playwright test e2e/background-network.spec.ts e2e/u-workers-brand.spec.ts。SQL 变更执行 make sqlc 并遵守迁移规则。

**Expected:** 非用户行为外发为 0，官方云关联请求为 0；模型请求、正常重试和授权联网不受新增限制，不能通过全局断网取得假通过。

**Commit:** test(privacy): verify no unsolicited traffic and working model requests。

## 7. 最终验收

- [ ] 用户界面与系统显示名统一 U Workers。
- [ ] 官方云产品、购买/升级和 multica.ai 运营服务关联已移除。
- [ ] 非用户行为的统计、崩溃、来源、营销、版本/设备外发为 0，环境变量不能重开。
- [ ] 缺配置不回退官方云，用户模型与自托管服务正常。
- [ ] DSH/Claude/Codex/DeepSeek 等请求、流式、重试与恢复不受新增限制。
- [ ] 用户工具联网、MCP、Git、下载、外链、集成与授权持续任务正常。
- [ ] 不自动加载追踪/嵌入资源，不自动检查或下载更新。
- [ ] 不新增 Agent 全局断网、本地模型强制要求或逐次模型授权弹窗。
- [ ] 默认本地存储/诊断，不宣称所有任务数据绝不离开本机。
- [ ] 原部署不受影响，实际验证端和未验证项清楚。

## 8. 本次完成状态

完成的是复制与计划修订，产品页面和生产配置尚未修改。目标为“消除非用户行为的数据外发，同时保留模型和授权联网能力”，不以完全离线、仅本机模型或所有出站为 0 为标准。
