# 改版 R0 · 功能基线清单

改版（A′「安静画布」方向）之后要一项一项核对功能有没有丢，这份文件就是核对基线：一部分是功能清单，一部分是整页截图脚本。本批只做清单和脚本，没有改任何产品代码。

功能点来源：CLAWSTA-129 方案第 1 节的功能表，加上对 `apps/web/app/**/page.tsx` 全部路由的逐页核对（路由表见下），以及实际检索仓库里的测试文件。

> 「现有自动化覆盖」的口径：只填确实驱动这一功能点（或它所用的同一个组件行为）的测试文件——`e2e/*.spec.ts` 的用例，或 `packages/**/*.test.tsx` 里针对该交互、该组件本身的 describe/it。只是同目录、并未覆盖该行为的间接用例一律写「无」。所以这一列是保守估计：写「无」不代表旁边一定没有测试，而是没找到针对它的测试。


## 怎么跑截图脚本

脚本在 `e2e/visual/baseline.spec.ts`，平时不参与测试：只有 `VISUAL_BASELINE=1` 时才会被 Playwright 收集。

```bash
# 1. 起本地服务（web + api + PostgreSQL）
bash scripts/dev-env.sh up          # 或 make up

# 2. 确认 visual 用例的 gating（普通 suite 不含它们）
pnpm exec playwright test e2e/visual --list                          # Total: 0 tests
VISUAL_BASELINE=1 pnpm exec playwright test e2e/visual --list        # Total: 25 tests

# 3. 截图
VISUAL_BASELINE=1 pnpm exec playwright test e2e/visual/baseline.spec.ts

# 4. 收工（保留数据库，方便下次）
bash scripts/dev-env.sh down        # 或 make down
```

- 输出目录：`e2e/visual/__out__/`，已加进 `.gitignore`，截图不进仓库。
- 文件名：`<编号>-<页面>-<宽>-<主题>.png`，例如 `07-issues-board-1440-light.png`。每页 4 张：1440×900 / 1280×800 × 浅色 / 深色，25 个页面共 100 张。
- 界面固定简体中文。12 号（任务详情-评论区）截的是滚动到评论输入框后的视口图，其余都是整页图；长页面的整页图会很高，这是有意的。
- 数据由现有的 `TestApiClient`（`e2e/fixtures.ts`）在本次运行里新建一个独立工作区生成：各状态任务、一个带父任务/评论/附件的任务、两个项目、一个智能体、一套运行时、一个 Skill、一个小队、一个自动化、几条收件箱通知和一条邀请链接。跑完删掉这个工作区。
- 不要指向真实部署或真实数据库（见 `AGENTS.md` 的 Testing 规则）。
- 只重截某一页：`VISUAL_BASELINE=1 pnpm exec playwright test e2e/visual/baseline.spec.ts -g "07 任务-看板"`。


## 页面与路由对照

### 截图页（25 页 / 100 张）

| 编号 | 页面 | 路由 | 文件名 |
| --- | --- | --- | --- |
| 01 | 加入页 | `/join?code=<邀请码>` | `01-join-<宽>-<主题>.png` |
| 02 | 登录页 | `/login` | `02-login-<宽>-<主题>.png` |
| 03 | 注册页 | `/register` | `03-register-<宽>-<主题>.png` |
| 04 | 收件箱-列表 | `/:workspaceSlug/inbox` | `04-inbox-<宽>-<主题>.png` |
| 05 | 收件箱-选中一条 | `/:workspaceSlug/inbox?issue=<id>` | `05-inbox-item-<宽>-<主题>.png` |
| 06 | 我的任务 | `/:workspaceSlug/my-issues` | `06-my-issues-<宽>-<主题>.png` |
| 07 | 任务-看板 | `/:workspaceSlug/issues（看板视图）` | `07-issues-board-<宽>-<主题>.png` |
| 08 | 任务-列表 | `/:workspaceSlug/issues（列表视图）` | `08-issues-list-<宽>-<主题>.png` |
| 09 | 任务-表格 | `/:workspaceSlug/issues（表格视图）` | `09-issues-table-<宽>-<主题>.png` |
| 10 | 任务-泳道 | `/:workspaceSlug/issues（泳道视图）` | `10-issues-swimlane-<宽>-<主题>.png` |
| 11 | 任务详情-顶部 | `/:workspaceSlug/issues/<id>` | `11-issue-detail-top-<宽>-<主题>.png` |
| 12 | 任务详情-评论区 | `/:workspaceSlug/issues/<id>（滚到评论区）` | `12-issue-detail-comments-<宽>-<主题>.png` |
| 13 | 项目列表 | `/:workspaceSlug/projects` | `13-projects-<宽>-<主题>.png` |
| 14 | 项目详情 | `/:workspaceSlug/projects/<id>` | `14-project-detail-<宽>-<主题>.png` |
| 15 | 智能体列表 | `/:workspaceSlug/agents` | `15-agents-<宽>-<主题>.png` |
| 16 | 智能体详情 | `/:workspaceSlug/agents/<id>` | `16-agent-detail-<宽>-<主题>.png` |
| 17 | 新建智能体 | `/:workspaceSlug/agents/new` | `17-agent-new-<宽>-<主题>.png` |
| 18 | 小队 | `/:workspaceSlug/squads` | `18-squads-<宽>-<主题>.png` |
| 19 | Skills | `/:workspaceSlug/skills` | `19-skills-<宽>-<主题>.png` |
| 20 | 运行时 | `/:workspaceSlug/runtimes` | `20-runtimes-<宽>-<主题>.png` |
| 21 | 自动化 | `/:workspaceSlug/autopilots` | `21-autopilots-<宽>-<主题>.png` |
| 22 | 统计 | `/:workspaceSlug/usage` | `22-usage-<宽>-<主题>.png` |
| 23 | 设置-个人 | `/:workspaceSlug/settings?tab=profile` | `23-settings-profile-<宽>-<主题>.png` |
| 24 | 设置-工作区 | `/:workspaceSlug/settings?tab=workspace` | `24-settings-workspace-<宽>-<主题>.png` |
| 25 | 设置-连接 | `/:workspaceSlug/settings?tab=channels` | `25-settings-channels-<宽>-<主题>.png` |

### 全部路由（`apps/web/app/**/page.tsx`）

| 路由 | 页面 | 截图编号 |
| --- | --- | --- |
| `/` | 根路径，重定向到 /login | 无截图 |
| `/login` | 登录页 | 02 |
| `/register` | 注册页 | 03 |
| `/join` | 加入页（分享链接落地） | 01 |
| `/auth/callback` | 第三方登录回调 | 无截图 |
| `/invitations` | 待处理邀请 | 无截图 |
| `/invite/[id]` | 邀请详情 | 无截图 |
| `/onboarding` | 新手引导 | 无截图 |
| `/workspaces/new` | 新建工作区 | 无截图 |
| `/:workspaceSlug/inbox` | 收件箱 | 04、05 |
| `/:workspaceSlug/my-issues` | 我的任务 | 06 |
| `/:workspaceSlug/issues` | 任务（看板 / 列表 / 表格 / 泳道） | 07、08、09、10 |
| `/:workspaceSlug/issues/[id]` | 任务详情 | 11、12 |
| `/:workspaceSlug/projects` | 项目列表 | 13 |
| `/:workspaceSlug/projects/[id]` | 项目详情 | 14 |
| `/:workspaceSlug/agents` | 智能体列表 | 15 |
| `/:workspaceSlug/agents/[id]` | 智能体详情 | 16 |
| `/:workspaceSlug/agents/new` | 新建智能体（选择方式） | 17 |
| `/:workspaceSlug/agents/new/ai` | AI 创建智能体 | 无截图 |
| `/:workspaceSlug/agents/new/ai/[sessionId]` | AI 创建会话 | 无截图 |
| `/:workspaceSlug/agents/new/manual` | 手动创建智能体 | 无截图 |
| `/:workspaceSlug/squads` | 小队列表 | 18 |
| `/:workspaceSlug/squads/[id]` | 小队详情 | 无截图 |
| `/:workspaceSlug/skills` | Skills 列表 | 19 |
| `/:workspaceSlug/skills/[id]` | Skill 详情 | 无截图 |
| `/:workspaceSlug/runtimes` | 运行时列表 | 20 |
| `/:workspaceSlug/runtimes/[id]` | 运行时（机器）详情 | 无截图 |
| `/:workspaceSlug/runtimes/[id]/runtime/[runtimeId]` | 运行时设置 | 无截图 |
| `/:workspaceSlug/runtimes/cloud` | 云运行时（不可用占位页） | 无截图 |
| `/:workspaceSlug/cloud-runtime` | 云运行时（不可用占位页） | 无截图 |
| `/:workspaceSlug/autopilots` | 自动化 | 21 |
| `/:workspaceSlug/autopilots/[id]` | 自动化详情 | 无截图 |
| `/:workspaceSlug/usage` | 统计 | 22 |
| `/:workspaceSlug/billing` | 计费（不可用占位页） | 无截图 |
| `/:workspaceSlug/chat` | 聊天 | 无截图 |
| `/:workspaceSlug/settings` | 设置（个人 / 工作区 / 连接） | 23、24、25 |
| `/:workspaceSlug/members/[id]` | 成员详情 | 无截图 |
| `/:workspaceSlug/attachments/[id]/preview` | 附件预览 | 无截图 |
| `/dingtalk/bind` | 钉钉绑定落地页 | 无截图 |
| `/lark/bind` | 飞书绑定落地页 | 无截图 |
| `/slack/bind` | Slack 绑定落地页 | 无截图 |
| `/telegram/bind` | Telegram 绑定落地页 | 无截图 |
| `/wecom/bind` | 企业微信绑定落地页 | 无截图 |
| `/billing/return` | 计费回跳（不可用占位页） | 无截图 |


## 功能清单

按页面分组，每行一个可核对的功能点；「截图编号」指上面的页面编号。全局功能在每一页都可见，编号写 01–25。

### 全局外壳（所有页面共用）

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 侧边栏入口：收件箱（含未读数） | e2e/navigation.spec.ts, packages/views/layout/app-sidebar.test.tsx | 01–25 |
| 侧边栏入口：我的任务 | 无 | 01–25 |
| 侧边栏入口：聊天（含未读数） | packages/views/layout/app-sidebar.test.tsx | 01–25 |
| 侧边栏入口：任务 | e2e/navigation.spec.ts | 01–25 |
| 侧边栏入口：项目 | 无 | 01–25 |
| 侧边栏入口：自动化 | 无 | 01–25 |
| 侧边栏入口：智能体 | e2e/navigation.spec.ts | 01–25 |
| 侧边栏入口：小队 | 无 | 01–25 |
| 侧边栏入口：Skills | 无 | 01–25 |
| 侧边栏入口：运行时 | 无 | 01–25 |
| 侧边栏入口：统计 | 无 | 01–25 |
| 侧边栏入口：设置 | e2e/navigation.spec.ts | 01–25 |
| 侧边栏分组标题（固定 / 工作 / AI 团队）与底部工具区 | 无 | 01–25 |
| 工作区切换菜单：当前用户、工作区列表与当前勾选、未读圆点 | packages/views/layout/app-sidebar.test.tsx | 01–25 |
| 工作区切换菜单：创建工作区 | 无 | 01–25 |
| 工作区切换菜单：待处理邀请的就地「加入 / 拒绝」 | packages/views/layout/app-sidebar.test.tsx | 01–25 |
| 工作区切换菜单：退出登录 | e2e/auth.spec.ts | 01–25 |
| 产品标识与浏览器标签标题（页面名 + U Workers） | e2e/u-workers-brand.spec.ts, apps/web/platform/document-title.test.tsx | 01–25 |
| 新建任务按钮（带 C 快捷键提示、有草稿时的小圆点） | e2e/auth.spec.ts | 01–25 |
| 全局搜索入口（侧边栏「搜索...」）与 Mod+K 打开 | packages/views/search/search-command.test.tsx | 01–25 |
| 快捷键默认绑定（搜索 Mod+K、新建任务 C 等）与各平台保留键策略 | packages/core/shortcuts/definitions.test.ts | 01–25 |
| 命令面板：按关键词筛页面、成员、最近任务 | packages/views/search/search-command.test.tsx | 01–25 |
| 命令面板命令：新建任务、新建项目、复制任务链接、复制标识符、收起 / 展开全部评论 | packages/views/search/search-command.test.tsx | 01–25 |
| 命令面板：切换到浅色 / 深色 / 跟随系统主题 | packages/views/search/search-command.test.tsx | 01–25 |
| 命令面板：⌘/Ctrl+回车或 ⌘/Ctrl+点击在新标签页打开结果 | packages/views/search/search-command.test.tsx | 01–25 |
| 置顶区：折叠展开、置顶项高亮、取消固定、「再显示 N 项」 | packages/views/layout/app-sidebar.test.tsx | 01–25 |
| 置顶区：拖动调整置顶顺序 | 无 | 01–25 |
| 帮助菜单：文档、反馈、服务器版本 | packages/views/layout/help-launcher.test.tsx | 01–25 |
| 侧边栏折叠 / 展开（Mod+B） | 无 | 01–25 |
| 输入框里不误触发全局快捷键（IME 组合、已处理的按键、长按重复） | packages/views/layout/global-shortcuts.test.ts | 01–25 |
| 侧边栏拖拽调宽 | packages/views/layout/sidebar-resize.test.tsx | 01–25 |
| 宽度不足时侧边栏自动收起 / 变成抽屉，路由切换后自动关掉 | packages/views/layout/sidebar-auto-collapse.test.tsx, packages/views/layout/app-sidebar.test.tsx | 01–25 |
| 聊天浮窗：悬浮按钮与窗口（新对话、还原、展开、最小化、历史会话、智能体筛选） | 无 | 01–25 |
| 聊天浮窗打开时，正在看的会话不计入侧边栏未读 | packages/views/layout/app-sidebar.test.tsx | 01–25 |
| 聊天浮窗只在非聊天页显示，关掉开关后整个消失 | packages/views/chat/floating-chat-visibility.test.ts | 01–25 |
| 聊天浮窗开关快捷键（Mod+J，输入框里也生效） | packages/views/layout/global-shortcuts.chat.test.tsx | 01–25 |
| 后退 / 前进快捷键（Mod+[ / Mod+]） | packages/views/layout/global-shortcuts.history.test.tsx | 01–25 |
| 主题：浅色 / 深色 / 跟随系统（偏好设置里切换，本设备生效） | packages/views/settings/components/preferences-tab.test.tsx | 01–25 |
| 语言：中文 / English / 한국어 / 日本語 / Français，保存失败时提示未同步 | packages/views/settings/components/preferences-tab.test.tsx | 01–25 |
| 固定系统快捷键（打开设置、关闭标签页、切换第 1–8 个标签、放大 / 缩小 / 重置缩放、关闭对话框） | packages/views/settings/components/keyboard-shortcuts-tab.test.tsx | 01–25 |
| 窄屏（<1024px）差异：设置页导航变下拉、聊天与收件箱变列表 / 详情两态 | packages/views/settings/components/settings-page.test.tsx, packages/views/chat/chat-page.test.tsx | 01–25 |
| 桌面端差异：设置里多一组「本设备」（守护进程、更新），以及标签页与缩放的窗口级快捷键 | packages/views/settings/components/settings-page.test.tsx | 01–25 |
| 移动端：独立的移动 App（Expo/React Native），不共用本页的 Web 外壳 | 无 | 01–25 |

### 01 加入页

路由：`/join?code=<邀请码>`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 打开分享链接先看到工作区名称与邀请人姓名（未登录也能看） | apps/web/app/join/page.test.tsx | 01 |
| 加入后将以什么身份进入：管理员 / 成员 徽章 | apps/web/app/join/page.test.tsx | 01 |
| 未登录时的提示与按钮文案（登录并加入） | apps/web/app/join/page.test.tsx | 01 |
| 「加入工作区」按钮：未登录先去登录，登录后回到本页 | apps/web/app/join/page.test.tsx | 01 |
| 加入成功后进入该工作区的任务列表 | apps/web/app/join/page.test.tsx | 01 |
| 已经是成员时直接进入工作区，不重复加入 | apps/web/app/join/page.test.tsx | 01 |
| 加入成功后同步账号状态失败时的提示与「重试同步」按钮 | apps/web/app/join/page.test.tsx | 01 |
| 链接无效或已过期时的错误页与「返回首页」按钮 | apps/web/app/join/page.test.tsx | 01 |
| 链接里没有邀请码时的提示 | 无 | 01 |
| 席位已满 / 席位无法校验时的错误提示 | 无 | 01 |

### 02 登录页

路由：`/login`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 邮箱输入框与「继续」按钮（发送登录验证码） | packages/views/auth/login-page.test.tsx | 02 |
| 6 位验证码输入框（填满自动提交） | packages/views/auth/login-page.test.tsx | 02 |
| 验证码页的「重新发送」与 N 秒倒计时 | packages/views/auth/login-page.test.tsx | 02 |
| 验证码页的「返回」按钮 | packages/views/auth/login-page.test.tsx | 02 |
| 账号 + 密码登录表单（账号名、密码） | packages/views/auth/login-page.test.tsx | 02 |
| 账号或密码错误时的提示（不把密码回显出来） | packages/views/auth/login-page.test.tsx | 02 |
| 邮箱验证码 / 账号密码两种登录方式互相切换（服务端两种都开时） | packages/views/auth/login-page.test.tsx | 02 |
| 「注册 / 设置密码」入口 | 无 | 02 |
| 「使用 Google 登录」按钮（部署配置了 Google 才出现） | packages/views/auth/login-page.test.tsx | 02 |
| 登录已过期的提示 | packages/views/auth/login-page.test.tsx | 02 |
| 密码刚修改完的一次性提示 | 无 | 02 |
| 已登录访客打开 /login 时按 ?next 或默认落点自动跳走 | apps/web/app/(auth)/login/page.test.tsx | 02 |
| 桌面端 platform=desktop 的登录交接（换取 token 后唤起桌面应用） | apps/web/app/(auth)/login/page.test.tsx | 02 |
| CLI 授权确认页：以当前邮箱授权 / 使用其他账号 | packages/views/auth/login-page.test.tsx | 02 |
| CLI 验证码登录后把 token 回传给本地回调地址 | packages/views/auth/login-page.test.tsx | 02 |
| 未登录访问工作区页面时被送回登录页 | e2e/auth.spec.ts | 02 |
| 登录成功后进入工作区 | e2e/auth.spec.ts | 02 |
| 登录页的品牌标识与自带静态资源 | e2e/u-workers-brand.spec.ts | 02 |

### 03 注册页

路由：`/register`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 「我有邀请 / 直接注册」模式切换（实例开放注册时才出现） | packages/views/auth/registration-page.test.tsx | 03 |
| 粘贴邀请链接或邀请码（自动从链接里取出 token） | packages/views/auth/registration-page.test.tsx | 03 |
| 带 #token 的邀请链接打开时自动填入，并把 token 从地址栏抹掉 | packages/views/auth/registration-page.test.tsx | 03 |
| 邮箱输入框（直接注册模式） | 无 | 03 |
| 显示名输入框 | 无 | 03 |
| 登录名输入框与命名规则提示 | 无 | 03 |
| 密码 + 确认密码（至少 12 位提示） | 无 | 03 |
| 「设置密码并进入」提交 | packages/views/auth/registration-page.test.tsx | 03 |
| 表单校验：缺邀请、缺显示名、缺登录名、两次密码不一致（并聚焦到出错的字段） | packages/views/auth/registration-page.test.tsx | 03 |
| 服务端拒绝后只清空密码，保留邀请、显示名、登录名 | packages/views/auth/registration-page.test.tsx | 03 |
| 服务端错误的本地化文案 | packages/views/auth/registration-page.test.tsx | 03 |
| 「返回登录」 | packages/views/auth/registration-page.test.tsx | 03 |
| 本实例未开放注册时的提示 | packages/views/auth/registration-page.test.tsx | 03 |

### 04 收件箱-列表

路由：`/:workspaceSlug/inbox`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 页头标题「收件箱」与未读数 | packages/views/inbox/components/inbox-page.test.tsx | 04 |
| 页头筛选按钮上的「N 个筛选条件」计数 | packages/views/inbox/components/inbox-filter-menu.test.tsx | 04 |
| 筛选：只看未读 | packages/views/inbox/components/inbox-filter-menu.test.tsx | 04 |
| 筛选：来自（按通知里的成员筛） | packages/views/inbox/components/inbox-filter-menu.test.tsx | 04 |
| 筛选：状态 | packages/views/inbox/components/inbox-filter-menu.test.tsx | 04 |
| 筛选：优先级 | packages/views/inbox/components/inbox-filter-menu.test.tsx | 04 |
| 筛选：清除筛选 | packages/views/inbox/components/inbox-filter-menu.test.tsx, packages/views/inbox/components/inbox-page.test.tsx | 04 |
| 筛选后一条不剩时的空态与「清除筛选」按钮 | packages/views/inbox/components/inbox-page.test.tsx | 04 |
| 批量菜单：全部标为已读 | 无 | 04 |
| 批量菜单：归档全部 | 无 | 04 |
| 批量菜单：归档全部已读 | 无 | 04 |
| 批量菜单：归档已完成 | 无 | 04 |
| 已归档视图里不出现批量菜单 | packages/views/inbox/components/inbox-page.test.tsx | 04 |
| 通知行：未读圆点与加粗标题 | packages/views/inbox/components/inbox-list-item.test.tsx | 04 |
| 通知行：动作副标题（状态设为 / 优先级设为 / 分配给谁 / 日期变更 / 表情回应 / 子任务结束等） | packages/views/inbox/components/inbox-detail-label.test.tsx | 04 |
| 通知行：右侧状态图标（自定义状态用自身颜色，悬停显示状态名） | packages/views/inbox/components/inbox-list-item.test.tsx | 04 |
| 通知行：相对时间（刚刚 / N 分钟 / N 小时 / N 天） | 无 | 04 |
| 通知行：悬停出现的归档按钮（已归档视图里为取消归档） | packages/views/inbox/components/inbox-list-item.test.tsx | 04 |
| 通知行：「…」行内菜单（标为已读、标为未读、归档、取消归档、在新标签页打开） | packages/views/inbox/components/inbox-context-menu.test.tsx | 04 |
| 通知行：右键菜单（同上，并屏蔽浏览器原生菜单） | packages/views/inbox/components/inbox-context-menu.test.tsx | 04 |
| 单击通知行选中它，不跳转页面 | packages/views/inbox/components/inbox-list-item.test.tsx | 04 |
| ⌘/Ctrl+点击、中键在新标签页打开该通知对应的任务 | packages/views/inbox/components/inbox-list-item.test.tsx | 04 |
| 上下方向键切换选中并把它滚进可视区 | packages/views/inbox/components/inbox-list.test.tsx | 04 |
| 打开通知后自动标为已读（手动标未读则保持） | packages/views/inbox/components/inbox-page.test.tsx | 04 |
| 归档 / 取消归档的键盘快捷键 | packages/views/inbox/components/inbox-page.test.tsx | 04 |
| 列表底部「已归档」入口 | packages/views/inbox/components/inbox-list.test.tsx | 04 |
| 已归档视图：顶部返回收件箱 | packages/views/inbox/components/inbox-page.test.tsx | 04 |
| 已归档视图：加载更多、加载失败重试 | packages/views/inbox/components/inbox-list.test.tsx, packages/views/inbox/components/inbox-page.test.tsx | 04 |
| 已归档视图：深链指向尚未加载的通知时单独补查（加载中提示、失败重试） | packages/views/inbox/components/inbox-page.test.tsx | 04 |
| 空列表提示（暂无通知 / 暂无已归档通知） | 无 | 04 |
| 通知行上的智能体活动标记 | packages/views/inbox/components/inbox-list-item.test.tsx | 04 |

### 05 收件箱-选中一条

路由：`/:workspaceSlug/inbox?issue=<id>`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 右栏：通知关联任务时展开完整任务详情（标题、正文、属性、评论区等） | packages/views/inbox/components/inbox-page.test.tsx | 05 |
| 从通知进入时自动滚到对应评论并高亮 | packages/views/inbox/components/inbox-page.test.tsx | 05 |
| 重复点击已打开的通知时重新播放上面的定位 | packages/views/inbox/components/inbox-page.test.tsx | 05 |
| 切换选中时先保留上一条详情，直到新详情就绪 | packages/views/inbox/components/inbox-page.test.tsx | 05 |
| 筛选把当前选中项过滤掉时清空选中 | 无 | 05 |
| 没有关联任务的普通通知：标题、类型、时间、正文 | packages/views/inbox/components/inbox-page.test.tsx | 05 |
| 普通通知的「原始输入」区块 | 无 | 05 |
| 普通通知的「保留上下文重试」按钮 | packages/views/inbox/components/inbox-page.test.tsx | 05 |
| 普通通知的「在完整表单中编辑」按钮 | 无 | 05 |
| 自动化执行次数用完的通知卡片与「查看限制详情」 | packages/views/inbox/components/autopilot-quota-notice.test.tsx, packages/views/inbox/components/inbox-page.test.tsx | 05 |
| 详情底部的归档 / 取消归档按钮 | 无 | 05 |
| 左右两栏之间可拖拽调整宽度 | packages/views/inbox/components/inbox-page.test.tsx | 05 |
| 窄屏下详情铺满全屏并提供返回列表 | packages/views/inbox/components/inbox-page.test.tsx | 05 |
| 未选中时的占位文案（选择一条通知查看详情 / 收件箱为空） | 无 | 05 |

### 06 我的任务

路由：`/:workspaceSlug/my-issues`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 页头标题「我的任务」与刷新状态 | packages/views/layout/issue-page-refresh.test.tsx | 06 |
| 范围页签：全部 | 无 | 06 |
| 范围页签：已分配 | 无 | 06 |
| 范围页签：我创建的 | 无 | 06 |
| 范围页签：我的智能体和小队 | 无 | 06 |
| 页签悬停说明（一句话解释每个范围包含什么） | 无 | 06 |
| 视图栏：保存的视图页签、新增视图、管理视图、拖拽排序、固定到侧栏、删除 | 无 | 06 |
| 筛选：状态 | packages/views/issues/components/issues-header.status-filter.test.tsx | 06 |
| 筛选：优先级 | 无 | 06 |
| 筛选：负责人（含小队、无负责人、搜索） | 无 | 06 |
| 筛选：创建者 | 无 | 06 |
| 筛选：项目 / 无项目 | 无 | 06 |
| 筛选：项目状态 | e2e/issue-project-status-filter.spec.ts | 06 |
| 筛选：标签 | 无 | 06 |
| 筛选：自定义属性（等于 / 包含 / 早于 / 晚于 / 无值） | packages/views/issues/components/issues-header.property-filter.test.tsx | 06 |
| 筛选：重置全部筛选 | 无 | 06 |
| 筛选条件胶囊条（每个条件一个可移除胶囊，带清除与保存） | packages/views/issues/components/filter-chips-bar.test.ts | 06 |
| 显示设置：分组（状态 / 负责人 / 项目） | 无 | 06 |
| 显示设置：排序字段 + 升序/降序切换 | e2e/issue-display-popover.spec.ts | 06 |
| 显示设置：显示子任务开关 | 无 | 06 |
| 显示设置：卡片属性开关（不含描述，含自定义属性） | packages/views/issues/components/list-view.test.tsx | 06 |
| 视图切换：看板 / 列表 / 表格 / 泳道 | e2e/issues.spec.ts | 06 |
| 智能体工作中筛选芯片与工作清单 | packages/views/issues/components/workspace-agent-working-chip.test.tsx | 06 |
| 批量操作工具栏（已选 N、状态、优先级、负责人、删除） | packages/views/issues/components/batch-action-toolbar.test.tsx | 06 |
| 保存为视图对话框 | packages/views/issues/components/save-view-dialog.test.tsx | 06 |
| 空态：没有分配给你的任务 | 无 | 06 |

### 07 任务-看板

路由：`/:workspaceSlug/issues（看板视图）`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 视图切换（看板 / 列表 / 表格 / 泳道） | e2e/issues.spec.ts | 07 |
| 范围页签：全部 | packages/views/issues/components/issues-page.test.tsx | 07 |
| 范围页签：成员 | packages/views/issues/components/issues-page.test.tsx | 07 |
| 范围页签：智能体 | packages/views/issues/components/issues-page.test.tsx | 07 |
| 页签悬停说明（每个范围包含哪些任务） | 无 | 07 |
| 筛选按钮上的「N 个筛选」计数 | e2e/issues.spec.ts | 07 |
| 筛选：状态（含自定义状态，带各自任务数） | packages/views/issues/components/issues-header.status-filter.test.tsx | 07 |
| 筛选：优先级 | 无 | 07 |
| 筛选：日期（按创建/更新时间；今天、最近 3 天、最近 7 天、自定义范围、清除日期） | e2e/issues.spec.ts | 07 |
| 筛选：负责人（成员 / 智能体 / 小队 / 无负责人，可搜索） | 无 | 07 |
| 筛选：创建者 | 无 | 07 |
| 筛选：项目 / 无项目 | 无 | 07 |
| 筛选：项目状态 | e2e/issue-project-status-filter.spec.ts | 07 |
| 筛选：标签 | 无 | 07 |
| 筛选：自定义属性（等于 / 包含 / 早于 / 晚于 / 无值） | packages/views/issues/components/issues-header.property-filter.test.tsx | 07 |
| 筛选：重置全部筛选 | 无 | 07 |
| 筛选条件胶囊条（每个条件一个可移除胶囊，带清除与保存） | packages/views/issues/components/filter-chips-bar.test.ts | 07 |
| 显示设置：分组（状态 / 负责人 / 项目 / 自定义下拉属性） | packages/views/issues/components/board-view-project-grouping.test.tsx | 07 |
| 显示设置：排序字段 + 升序/降序切换 | e2e/issue-display-popover.spec.ts | 07 |
| 显示设置：卡片属性开关（优先级、描述、负责人、开始日期、截止日期、项目、标签、子任务进度、自定义属性） | packages/views/issues/components/list-view.test.tsx | 07 |
| 显示设置：显示子任务开关 | packages/views/issues/components/swimlane-view.test.tsx | 07 |
| 保存为视图 / 编辑已保存的视图 | packages/views/issues/components/save-view-dialog.test.tsx | 07 |
| 视图栏：内置页签、保存的视图、拖拽排序、右键编辑/删除/固定到侧栏、更多视图、管理视图、新增视图 | 无 | 07 |
| 智能体工作中筛选芯片（含工作清单） | packages/views/issues/components/workspace-agent-working-chip.test.tsx | 07 |
| ⌘K 全局搜索面板（跨页面） | packages/views/search/search-command.test.tsx | 07 |
| 页头刷新状态（切视图/改筛选时标题旁转圈） | packages/views/layout/issue-page-refresh.test.tsx | 07 |
| 空态：还没有任务 | 无 | 07 |
| 空态：没有符合筛选条件的任务 + 清除筛选按钮 | 无 | 07 |
| 看板按状态分列（含自定义状态列） | packages/views/issues/components/issues-page.test.tsx | 07 |
| 列头显示状态名 | packages/views/issues/components/issues-page.test.tsx | 07 |
| 列头「+」在该列新建任务（自动带入该列的状态或分组值；已归档的状态没有「+」） | packages/views/issues/components/create-renderers.test.tsx | 07 |
| 列头「…」菜单：隐藏列 | 无 | 07 |
| 右端「隐藏的列」侧栏：列出被隐藏的状态与数量，可「显示列」 | 无 | 07 |
| 把卡片拖到另一列改状态 | 无 | 07 |
| 同一列内拖拽调整卡片顺序（仅排序方式为「手动」时） | 无 | 07 |
| 非手动排序下拖拽时提示切到「手动」才能调序 | 无 | 07 |
| 在看板空白处按住拖动横向平移 | packages/views/issues/components/use-board-drag-pan.test.tsx | 07 |
| 卡片上直接改优先级 / 负责人 / 开始日期 / 截止日期 | packages/views/issues/components/board-card-assignee-picker.test.tsx | 07 |
| 卡片显示内容（编号、标题、描述摘要、标签、项目、自定义状态标签、子任务进度、更新时间） | packages/views/issues/components/board-card-assignee-picker.test.tsx | 07 |
| Shift+点击卡片打开侧边预览（空格预览、J/K 上下切换、H/L 跨列切换） | packages/views/issues/components/board-card-peek.test.tsx, packages/views/issues/components/issue-peek.test.tsx | 07 |
| 右键卡片弹出任务操作菜单 | packages/views/issues/actions/__tests__/issue-actions-menu.test.tsx | 07 |
| 按负责人 / 项目 / 自定义属性分组时的列，以及「无负责人 / 无项目 / 未设置」列 | packages/views/issues/components/board-view-project-grouping.test.tsx, packages/views/issues/components/issues-page.test.tsx | 07 |
| 分组列的计数取服务端总数（不只算已加载的卡片） | packages/views/issues/components/board-view-project-grouping.test.tsx | 07 |
| 分组列加载更多 / 加载失败重试 | 无 | 07 |

### 08 任务-列表

路由：`/:workspaceSlug/issues（列表视图）`

> 与 07 共用的页头功能（视图切换、范围页签、筛选、显示设置、排序、保存视图、批量操作、⌘K）在四种视图下完全相同，逐项见 07 任务-看板；本页只列列表视图特有的功能点。

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 显示设置：卡片属性开关（优先级、负责人、开始日期、截止日期、项目、标签、子任务进度、自定义属性） | packages/views/issues/components/list-view.test.tsx | 08 |
| 批量操作工具栏（已选 N、改状态、改优先级、改负责人、删除与删除确认） | packages/views/issues/components/batch-action-toolbar.test.tsx, packages/views/issues/components/batch-action-toolbar.confirm.test.tsx | 08 |
| 按状态分组的分节与节头（状态名 + 任务总数） | packages/views/issues/components/list-view.test.tsx | 08 |
| 点节头折叠 / 展开该状态下的任务 | packages/views/issues/components/list-view.test.tsx | 08 |
| 节头全选复选框（选中该状态全部任务） | 无 | 08 |
| 节头「+」在该状态下新建任务（已归档的状态没有「+」） | packages/views/issues/components/create-renderers.test.tsx | 08 |
| 行内容（编号、标题、自定义状态标签、重复标记、子任务进度、标签、自定义属性、项目、开始日期、截止日期、负责人） | packages/views/issues/components/list-view.test.tsx | 08 |
| 悬停行时优先级图标换成勾选框以多选 | 无 | 08 |
| 把行拖到另一个状态分组改状态 | 无 | 08 |
| 同一状态内拖拽调整顺序（仅「手动」排序时） | 无 | 08 |
| 隐藏状态提示与「显示列」恢复入口 | packages/views/issues/components/list-view.test.tsx | 08 |
| 空状态分节显示「无任务」 | 无 | 08 |
| 分节加载更多 / 没有更多了 / 加载失败重试 | 无 | 08 |
| Shift+点击或（设为默认时）单击打开侧边预览 | packages/views/issues/components/list-row-peek.test.tsx | 08 |

### 09 任务-表格

路由：`/:workspaceSlug/issues（表格视图）`

> 与 07 共用的页头功能逐项见 07 任务-看板；本页只列表格视图特有的功能点。

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 显示设置：层级开关（子任务嵌在父任务下方显示） | packages/views/issues/components/table-view-virtualized-hierarchy.test.tsx | 09 |
| 表头「分组」按钮（不分组 / 状态 / 负责人 / 项目 / 自定义属性） | packages/views/issues/components/table-view-project-grouping.test.tsx | 09 |
| 表头「列」按钮（搜索列、勾选显示或隐藏列、系统列与自定义属性分组） | packages/views/issues/components/table-column-picker.test.tsx | 09 |
| 批量操作工具栏（已选 N、改状态、改优先级、改负责人、删除与删除确认） | packages/views/issues/components/batch-action-toolbar.test.tsx, packages/views/issues/components/batch-action-toolbar.confirm.test.tsx | 09 |
| 表头点开排序菜单（升序 / 降序） | 无 | 09 |
| 表头拖动调整列顺序（拖把手或键盘） | 无 | 09 |
| 拖动列边界调整列宽、双击自适应内容宽度 | packages/views/issues/components/data-table-resize.test.tsx | 09 |
| 表头菜单「隐藏列」 | 无 | 09 |
| 表头「+」直接挑选要添加的列 | packages/views/issues/components/table-column-picker.test.tsx | 09 |
| 分组行显示分组名与条数 | packages/views/issues/components/table-view-project-grouping.test.tsx | 09 |
| 点分组行折叠 / 展开该组 | 无 | 09 |
| 层级开启后父任务下方缩进显示子任务 | packages/views/issues/components/table-view-virtualized-hierarchy.test.tsx | 09 |
| 父行箭头展开 / 折叠子任务 | 无 | 09 |
| 行内改状态（会启动智能体的改动先弹确认） | packages/views/issues/components/table-view-editing.test.tsx | 09 |
| 行内改优先级 / 负责人 / 项目 / 开始日期 / 截止日期 / 标签 / 自定义属性 | 无 | 09 |
| 行内改标题（铅笔按钮进入，Enter 提交、Esc 取消、点别处提交） | packages/views/issues/components/table-inline-title.test.tsx | 09 |
| 标题行「+」直接建子任务（继承父任务的项目） | packages/views/issues/components/table-inline-title.test.tsx, packages/views/issues/components/table-view-editing.test.tsx | 09 |
| 表格上方搜索框（按标题或编号搜，可清空） | packages/views/issues/components/table-issue-search.test.tsx | 09 |
| 导出：全部 / 导出已选择 N 个（下载 CSV） | packages/views/issues/components/table-view-model.test.ts | 09 |
| 表头全选复选框 | 无 | 09 |
| Shift 点击连选一段行 | packages/views/issues/components/table-view-model.test.ts | 09 |
| 点击行进入任务详情（Shift 打开预览、⌘/Ctrl 或中键在新标签页打开） | packages/views/issues/components/table-view-editing.test.tsx | 09 |
| 滚动加载更多行 / 分组分页 / 加载失败重试 | e2e/issue-table.spec.ts | 09 |
| 表尾计算（计数 / 总和 / 平均值）与行内快速新建「添加任务…」：zh-Hans 文案已定义，当前界面无入口 | 无 | 09 |

### 10 任务-泳道

路由：`/:workspaceSlug/issues（泳道视图）`

> 与 07 共用的页头功能逐项见 07 任务-看板；本页只列泳道视图特有的功能点。

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 显示设置：分组（父任务 / 项目 / 负责人） | packages/views/issues/components/swimlane-view.test.tsx | 10 |
| 显示设置：卡片属性开关（同看板） | packages/views/issues/components/list-view.test.tsx | 10 |
| 按父任务 / 项目 / 负责人分成泳道 | packages/views/issues/components/swimlane-view.test.tsx | 10 |
| 顶部状态列头（每列总数）与列头「…」隐藏列 | packages/views/issues/components/swimlane-view.test.tsx | 10 |
| 泳道行头（折叠箭头、父任务状态图标与编号、任务总数） | packages/views/issues/components/swimlane-view.test.tsx | 10 |
| 拖动泳道行头调整泳道顺序（固定泳道不可拖） | packages/views/issues/components/swimlane-view.test.tsx | 10 |
| 「无父级 / 无项目 / 未分配」固定泳道 | packages/views/issues/components/swimlane-view.test.tsx | 10 |
| 「其他父级」只读泳道（不能拖入拖出、没有新建按钮） | packages/views/issues/components/swimlane-view.test.tsx | 10 |
| 打开父级任务 | packages/views/issues/components/swimlane-view.test.tsx | 10 |
| 单元格「+」新建任务（带入该列状态与所在泳道） | packages/views/issues/components/swimlane-view.test.tsx | 10 |
| 拖拽卡片改状态，同时改父任务 / 项目 / 负责人 | packages/views/issues/components/swimlane-view.test.tsx | 10 |
| 隐藏列侧栏与各列任务数 | packages/views/issues/components/swimlane-view.test.tsx | 10 |
| 单元格空态显示「—」 | 无 | 10 |
| 按父任务分组时批量拉取子任务填满空格子 | packages/views/issues/components/swimlane-view.test.tsx | 10 |
| 「显示子任务」开关对泳道生效 | packages/views/issues/components/swimlane-view.test.tsx | 10 |

### 11 任务详情-顶部

路由：`/:workspaceSlug/issues/<id>`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 面包屑（工作区 / 项目）与返回出口 | packages/views/issues/components/issue-detail.test.tsx | 11 |
| 面包屑末端的「编号 + 标题」链接 | packages/views/issues/components/issue-detail.test.tsx | 11 |
| 标题就地编辑（离开焦点即保存） | packages/views/issues/components/issue-detail.test.tsx | 11 |
| 标题并发冲突时对比两个版本（保留我的 / 使用最新） | packages/views/issues/components/issue-detail.test.tsx | 11 |
| 正文（描述）编辑器，编辑后自动保存 | packages/views/issues/components/issue-detail.test.tsx | 11 |
| 正文保存失败或并发冲突时的处理 | packages/views/issues/components/issue-detail.test.tsx | 11 |
| 正文下方的表情回应 | 无 | 11 |
| 正文下方的附件上传按钮、把文件拖进正文 | 无 | 11 |
| 标题下的「属于父任务 XXX」一行（含父任务完成进度） | 无 | 11 |
| 属性胶囊（状态、优先级、负责人、项目、开始日期、截止日期、标签；出现在侧边预览里） | packages/views/issues/components/issue-detail.test.tsx | 11 |
| 置顶 / 取消置顶到侧边栏 | packages/views/issues/actions/__tests__/use-issue-actions.test.tsx | 11 |
| 标记为已完成 | 无 | 11 |
| 任务已完成时的归档按钮 | 无 | 11 |
| 「更多」菜单：状态、优先级、负责人、开始日期、截止日期、在新标签页打开、固定到侧边栏、复制链接、复制本地 workdir / 项目目录路径、关系（创建子任务、设置父任务、移除父任务、标记为重复）、删除任务 | packages/views/issues/actions/__tests__/issue-actions-menu.test.tsx | 11 |
| 切换右侧边栏的显示 / 隐藏 | 无 | 11 |
| 页面内查找（⌘F：输入、上一个/下一个、计数、无匹配、关闭） | packages/views/issues/hooks/use-in-page-find.test.ts | 11 |
| 子任务区块（行内容、显示设置、按阶段分组、加号新建） | packages/views/issues/components/issue-detail.test.tsx | 11 |
| 子任务区块的折叠与「全选子任务」 | 无 | 11 |
| 重复标记横幅与「不是重复」 | packages/views/issues/components/issue-duplicates.test.tsx | 11 |
| 来源上下文徽标（从评论创建时的上下文快照，可查看 / 放大 / 关闭） | packages/views/issues/components/source-context-viewer.test.tsx | 11 |
| 头部智能体工作中芯片（悬停看正在跑的运行） | packages/views/issues/components/issue-agent-header-chip.test.tsx | 11 |
| 头部唤醒芯片（点开跳到右栏唤醒） | packages/views/issues/components/issue-wakeup-header-chip.test.tsx | 11 |
| 右栏：属性（状态、负责人、项目 + 已设置的优先级/阶段/开始日期/截止日期/标签 + 自定义属性 + 「添加字段」） | packages/views/issues/components/issue-detail.test.tsx | 11 |
| 右栏：快捷操作（一键触发智能体、显示其余 N 条、无权限时的说明弹窗） | packages/views/issues/components/quick-actions-section.test.tsx | 11 |
| 右栏：唤醒（规则列表、新建唤醒、立即唤醒、开关、删除、触发记录、系统规则） | packages/views/issues/components/wakeups-section.test.tsx, e2e/issue-wakeups.spec.ts | 11 |
| 右栏：插件面板（插件挂在任务页上的自定义区块） | packages/views/plugins/plugin-panel-section.test.tsx | 11 |
| 右栏：父任务（跳转父任务、移除父任务） | 无 | 11 |
| 右栏：重复项（列出被标记为重复的任务） | packages/views/issues/components/issue-duplicates.test.tsx | 11 |
| 右栏：Pull Request（关联 PR、CI 与合并状态、行菜单移除关联、合并后改状态的设置） | packages/views/issues/components/pull-requests-section.test.tsx, packages/views/issues/components/pull-request-list.test.tsx | 11 |
| 右栏：产物（每个文件的最新版本、查看全部 N 个产物、产物总览与按类型筛选） | packages/views/issues/components/deliverables/deliverables.test.tsx | 11 |
| 右栏：执行日志（进行中的运行、最近三次历史运行、费用与耗时、费用趋势条、打开运行时间线） | packages/views/issues/components/execution-log-section.test.tsx | 11 |
| 运行时间线对话框（按天分组、总花费与耗时、费用明细、缺价模型提示） | packages/views/issues/components/issue-runs-dialog.test.tsx | 11 |
| 右栏：详情（创建者、创建时间、更新时间） | packages/views/issues/components/issue-detail.test.tsx | 11 |
| 右栏：元数据（键数量、打开 JSON 弹窗；无元数据时不显示） | packages/views/issues/components/issue-detail.test.tsx | 11 |

### 12 任务详情-评论区

路由：`/:workspaceSlug/issues/<id>（滚到评论区）`

> 与 11 共用的右栏区块（属性、快捷操作、唤醒、插件、父任务、重复任务、PR、产物、执行日志、详情、元数据）逐项见 11 任务详情-顶部；本页只列评论区特有的交互。

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 「动态」标题与订阅 / 取消订阅 | packages/views/issues/components/issue-detail.test.tsx | 12 |
| 取消订阅菜单（取消订阅此任务 / 此任务及其子任务） | packages/views/issues/components/issue-detail.test.tsx | 12 |
| 订阅者头像组与点开更改订阅者 | packages/views/issues/components/issue-detail.test.tsx | 12 |
| 「由智能体代你关注」徽标与说明 | 无 | 12 |
| 评论卡片：作者、时间、正文（Markdown、代码高亮、@提及卡片、表情） | packages/views/issues/components/issue-detail.test.tsx | 12 |
| 评论附件卡片（图片、文件、HTML 不内嵌、重传标 v2 版本） | packages/views/issues/components/comment-card.test.tsx | 12 |
| 给评论加表情回应 | 无 | 12 |
| 评论「…」菜单（编辑、解决该讨论、以此评论解决讨论、重新打开讨论、复制、复制链接、创建子任务、删除） | packages/views/issues/components/comment-card-actions-menu.test.tsx, packages/views/issues/components/comment-card-copy-link.test.tsx | 12 |
| 行内编辑评论（保存 / 取消 / 附件上传中禁用保存） | packages/views/issues/components/comment-card-edit-gate.test.tsx | 12 |
| 删除评论的确认（是否连带删除回复的说明） | packages/views/issues/components/comment-card-deleted.test.tsx | 12 |
| 已删除评论的占位与保留的回复 | packages/views/issues/components/comment-card-deleted.test.tsx | 12 |
| 回复某条评论（回复框、回复数量、回复对象提示、回复内容预览） | packages/views/issues/components/comment-composers.test.tsx | 12 |
| 收起 / 展开某条讨论 | 无 | 12 |
| 已解决讨论折叠条（N 条已解决评论、展开、折叠、结论徽标） | 无 | 12 |
| 更早的动态折叠与逐段展开 | packages/views/issues/components/issue-detail.test.tsx | 12 |
| 底部评论框（发送、⌘/Ctrl+Enter、空内容时发送禁用） | packages/views/issues/components/comment-composers.test.tsx | 12 |
| 评论框附件上传（上传未完成时禁用发送） | packages/views/issues/components/comment-composers.test.tsx | 12 |
| 发送失败时保留草稿并可重试 | packages/views/issues/components/comment-composers.test.tsx | 12 |
| 接收方芯片：谁会因为这条评论开始工作，点击可改为「插话到当前运行 / 本轮结束后开始 / 停止并重新开始 / 开始时一并处理」 | packages/views/issues/components/comment-trigger-chips.test.tsx, packages/views/issues/components/comment-composers.test.tsx | 12 |
| 插话回执（等待 X 读取 / 已读取 / 未收到并给出原因 / 作为新一轮发送） | packages/views/issues/components/steer-receipts.test.tsx | 12 |
| 被阻塞的 @ 提及（说明不会触发的原因） | packages/views/issues/components/comment-trigger-chips.test.tsx | 12 |
| @all 成员广播提示（不会启动智能体） | packages/views/issues/components/comment-trigger-chips.test.tsx | 12 |
| 划选评论文本添加批注（添加批注、批注条数、移除、原文已变提示） | packages/views/issues/components/reply-annotations.test.tsx, packages/views/issues/components/comment-selection-bubble.test.tsx | 12 |
| 评论里内联的智能体运行块（状态、步骤预览、停止、查看记录、打开完整日志） | packages/views/issues/components/inline-comment-run.test.tsx | 12 |
| 运行失败的原因与重试按钮 | packages/views/issues/components/issue-detail.test.tsx, packages/views/issues/components/inline-comment-run.test.tsx | 12 |
| 右侧讨论快速跳转轨（刻度、悬停看标题、点击跳转、已解决标记、每条回复一个刻度） | packages/views/issues/components/thread-minimap.test.tsx | 12 |
| 底部评论框吸底 / 跟随内容滚动 | packages/views/issues/components/issue-detail.test.tsx | 12 |
| 「本地目录」提示 | packages/views/projects/components/local-directory-hint.test.tsx | 12 |
| 从通知深链进来时定位到某条评论并高亮 | packages/views/issues/components/issue-detail.test.tsx | 12 |

### 13 项目列表

路由：`/:workspaceSlug/projects`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 页面标题「项目」与项目总数 | 无 | 13 |
| 「新建项目」按钮（打开新建项目弹窗） | 无 | 13 |
| 空状态「还没有项目」与「创建第一个项目」按钮 | 无 | 13 |
| 搜索项目（按标题，支持拼音匹配） | 无 | 13 |
| 当前结果 / 全部项目 计数（搜索或筛选生效时出现） | 无 | 13 |
| 筛选：状态（计划中、进行中、已暂停、已完成、已取消） | 无 | 13 |
| 筛选：优先级（紧急、高、中、低、无优先级） | 无 | 13 |
| 筛选：负责人（成员与智能体分组，带数量） | 无 | 13 |
| 清除全部筛选 | 无 | 13 |
| 显示：排序依据（名称、优先级、状态、进度、创建时间） | 无 | 13 |
| 显示：升序 / 降序切换 | 无 | 13 |
| 显示：表格视图下的列开关（优先级、进度、负责人、任务数、创建时间） | 无 | 13 |
| 视图切换：表格 / 卡片 | 无 | 13 |
| 表格表头点击排序（名称、状态、优先级、进度、创建时间） | 无 | 13 |
| 表格全选 / 取消全选 | 无 | 13 |
| 表格行内直接改项目状态 | 无 | 13 |
| 表格行内直接改项目优先级 | 无 | 13 |
| 表格行内直接改负责人 | 无 | 13 |
| 表格行内的进度环与完成数/任务数 | 无 | 13 |
| 表格列：任务数、创建时间（相对时间） | 无 | 13 |
| 点击行打开项目详情 | packages/views/projects/components/projects-page.test.tsx | 13 |
| 行上中键或修饰键在新标签页打开 | packages/views/projects/components/projects-page.test.tsx | 13 |
| 行操作菜单：在新标签页打开、固定到侧边栏、取消钉选、删除 | 无 | 13 |
| 删除项目确认弹窗（取消 / 删除） | 无 | 13 |
| 卡片视图：状态徽章、负责人、优先级、进度、创建时间 | 无 | 13 |
| 批量工具栏：已选 N 项、清除选择、固定到侧边栏、删除 | 无 | 13 |
| 批量删除确认弹窗 | 无 | 13 |
| 无匹配提示（搜索或筛选无结果） | 无 | 13 |
| 加载中的骨架屏 | 无 | 13 |
| 新建项目弹窗：选择图标 | 无 | 13 |
| 新建项目弹窗：项目标题与描述 | 无 | 13 |
| 新建项目弹窗：指派负责人（成员、智能体、无负责人） | 无 | 13 |
| 新建项目弹窗：来源 GitHub 仓库（搜索、勾选工作区仓库、粘贴 URL 添加） | 无 | 13 |
| 新建项目弹窗：来源 本地目录（选择目录、更换、清除、仅本机可用提示） | 无 | 13 |
| 新建项目弹窗：更多选项（开始日期、截止日期）与「创建项目」 | 无 | 13 |

### 14 项目详情

路由：`/:workspaceSlug/projects/<id>`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 面包屑返回项目列表 | 无 | 14 |
| 右栏：更换项目图标（表情选择） | 无 | 14 |
| 右栏：编辑项目标题 | 无 | 14 |
| 右栏属性区展开 / 收起 | 无 | 14 |
| 属性：状态（计划中、进行中、已暂停、已完成、已取消） | 无 | 14 |
| 属性：优先级（紧急、高、中、低、无优先级） | 无 | 14 |
| 属性：负责人（搜索指派、成员与智能体分组、无负责人） | 无 | 14 |
| 属性：开始日期 | packages/views/projects/components/project-date-pickers.test.tsx | 14 |
| 属性：截止日期 | packages/views/projects/components/project-date-pickers.test.tsx | 14 |
| 进度区（完成比例条与完成数/任务数） | 无 | 14 |
| 描述区：Markdown 编辑与「会作为项目上下文提供给智能体」提示 | 无 | 14 |
| 资源区：区块标题与空状态「还没有关联资源」 | 无 | 14 |
| 资源区：添加资源弹窗，搜索 GitHub 仓库与「已关联」标记 | 无 | 14 |
| 资源区：粘贴仓库 URL 关联 | packages/views/projects/components/project-resources-github-ref.test.tsx | 14 |
| 资源区：设置任务工作的起始分支 | packages/views/projects/components/project-resources-github-ref.test.tsx | 14 |
| 资源区：移除已关联资源 | packages/views/projects/components/project-resources-local-directory.test.tsx | 14 |
| 资源区：添加本地目录 | 无 | 14 |
| 资源区：本地目录提示（仅注册它的那台电脑可用） | packages/views/projects/components/local-directory-hint.test.tsx | 14 |
| 资源区：执行方式选择（直接修改文件夹 / 并行隔离运行） | packages/views/projects/components/local-directory-mode-dialog.test.tsx | 14 |
| 顶栏：固定到侧边栏 / 取消固定 | 无 | 14 |
| 顶栏更多菜单：复制链接 | packages/views/projects/components/project-detail.test.tsx | 14 |
| 顶栏更多菜单：删除项目 | packages/views/projects/components/project-detail.test.tsx | 14 |
| 删除项目确认弹窗（取消 / 删除） | packages/views/projects/components/project-detail.test.tsx | 14 |
| 顶栏：切换右侧边栏显示 | 无 | 14 |
| 主体：项目内任务的视图切换（看板、列表、表格、泳道、甘特） | 无 | 14 |
| 主体：项目暂无任务时的空状态与「新建任务」 | 无 | 14 |

### 15 智能体列表

路由：`/:workspaceSlug/agents`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 页面标题「智能体」、数量与说明文案 | 无 | 15 |
| 「新建智能体」按钮 | 无 | 15 |
| 「了解更多」文档链接 | packages/views/agents/components/agents-page.test.tsx | 15 |
| 搜索智能体（按名称） | 无 | 15 |
| 范围切换：我的 / 全部 / 已归档（各带数量） | 无 | 15 |
| 当前结果 / 范围内全部 计数 | 无 | 15 |
| 筛选：可用性（全部、在线、不稳定、离线、已归档） | 无 | 15 |
| 筛选：访问权限（工作区、指定成员、仅所有者） | packages/views/agents/components/agents-page-filter-predicate.test.ts | 15 |
| 筛选：运行时（按运行时名称，带数量） | 无 | 15 |
| 筛选：owner | 无 | 15 |
| 筛选：模型 | 无 | 15 |
| 清除筛选 | 无 | 15 |
| 显示：排序（最近活跃、名称、运行最多、最近创建） | packages/views/agents/components/agents-page.test.tsx | 15 |
| 显示：升序 / 降序 | 无 | 15 |
| 显示：列开关（状态、owner、访问权限、运行时、最近活跃、运行次数、模型、创建时间） | 无 | 15 |
| 行：头像、名称、描述、私有锁标记、「你」标记 | 无 | 15 |
| 状态列：在线 / 不稳定 / 离线，并附处理中、排队中、空闲与运行数 | 无 | 15 |
| 访问权限列：工作区 / 指定成员 / 仅所有者 | packages/views/agents/components/agents-page-access-cell.test.tsx | 15 |
| 运行时列 | 无 | 15 |
| 运行次数列 | 无 | 15 |
| 最近活跃列（今天、N 天前、30 天内无活动） | 无 | 15 |
| 模型列 | 无 | 15 |
| 创建时间列 | 无 | 15 |
| 「需绑定运行时」提示 | 无 | 15 |
| 「已归档」标记 | 无 | 15 |
| 点击行打开智能体详情 | 无 | 15 |
| 行操作菜单：在新标签页打开 | 无 | 15 |
| 行操作菜单：取消全部运行（含运行数确认弹窗） | 无 | 15 |
| 行操作菜单：复制智能体 | 无 | 15 |
| 行操作菜单：归档（确认弹窗） | 无 | 15 |
| 行操作菜单：恢复（已归档时） | 无 | 15 |
| 批量工具栏：已选 N 项、清除选择 | 无 | 15 |
| 批量工具栏：设置访问范围（应用于 N 个、跳过数提示、应用） | packages/views/agents/components/agent-batch-toolbar.test.tsx, packages/views/agents/components/access-picker-bulk-commit.test.tsx | 15 |
| 批量工具栏：归档所选 | packages/views/agents/components/agent-batch-toolbar.test.tsx | 15 |
| 批量工具栏：恢复所选 | 无 | 15 |
| 空状态与无匹配提示 | 无 | 15 |
| 列表加载失败与「重试」 | 无 | 15 |

### 16 智能体详情

路由：`/:workspaceSlug/agents/<id>`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 面包屑返回智能体列表 | 无 | 16 |
| 头部：头像、名称、状态指示、描述 | 无 | 16 |
| 头部：模型、运行时、可见性徽章、更新时间 | 无 | 16 |
| 「私信」按钮（无权限时提示而不是跳转） | packages/views/agents/components/agent-detail-page.test.tsx | 16 |
| 「分配工作」按钮 | 无 | 16 |
| 更多操作菜单：归档智能体 | packages/views/agents/components/agent-detail-page.test.tsx | 16 |
| 归档确认弹窗（取消 / 归档） | 无 | 16 |
| 已归档横幅与「恢复」 | 无 | 16 |
| 需绑定运行时横幅与「绑定运行时」 | packages/views/agents/components/agent-detail-page.test.tsx | 16 |
| 顶层标签：概览 / 工作 / 能力 / 设置 | packages/views/agents/components/agent-overview-pane.test.tsx | 16 |
| 概览：当前运行 | packages/views/agents/components/tabs/activity-tab.render.test.tsx | 16 |
| 概览：近 30 天表现（成功率、平均耗时、失败） | packages/views/agents/components/tabs/activity-tab.render.test.tsx | 16 |
| 概览：最近工作列表与「查看更多」 | packages/views/agents/components/tabs/activity-tab.render.test.tsx | 16 |
| 概览右栏：所有者、访问权限、运行时、模型、并发 | 无 | 16 |
| 概览右栏：已分配的 Skills 标签 | 无 | 16 |
| 工作：已分配 / 已创建 切换 | 无 | 16 |
| 工作：搜索任务 | 无 | 16 |
| 能力 / 设置下的二级标签导航（窄屏横向排列，宽屏左侧列表） | packages/views/agents/components/agent-overview-pane.test.tsx | 16 |
| 能力 → 指令：编辑并保存 System Prompt | packages/views/agents/components/tabs/instructions-tab.test.tsx | 16 |
| 能力 → 指令：查看 / 收起「由 U Workers 维护」的系统层 | 无 | 16 |
| 能力 → 指令：工作区补充说明 | 无 | 16 |
| 能力 → Skills：分配给智能体的 skill 开关（关闭后暂停使用但保留分配） | packages/views/agents/components/tabs/skills-tab.test.tsx | 16 |
| 能力 → Skills：移除已分配的 skill | packages/views/agents/components/tabs/skills-tab.test.tsx | 16 |
| 能力 → Skills：从运行时继承的 skill 列表与「刷新」 | packages/views/agents/components/tabs/skills-tab.test.tsx | 16 |
| 能力 → Skills：添加 skill 弹窗（搜索、多选、添加） | 无 | 16 |
| 能力 → MCP：添加 MCP Server（可视化配置 / JSON、STDIO 与 Streamable HTTP） | packages/views/agents/components/tabs/mcp-config-tab.test.tsx | 16 |
| 能力 → MCP：编辑、重命名、删除 MCP Server | packages/views/agents/components/tabs/mcp-config-tab.test.tsx | 16 |
| 能力 → MCP：工作区 MCP Server 的分配、启用开关与移除 | packages/views/agents/components/tabs/mcp-config-tab.test.tsx | 16 |
| 能力 → MCP：从运行时继承的 MCP Server 与「刷新」 | packages/views/agents/components/tabs/mcp-config-tab.test.tsx | 16 |
| 能力 → MCP 应用：勾选允许该智能体使用的应用 | packages/views/agents/components/tabs/agent-mcp-tab.test.tsx, e2e/agent-mcp.spec.ts | 16 |
| 能力 → 集成：消息平台连接状态与绑定入口 | packages/views/agents/components/tabs/integrations-tab.test.tsx | 16 |
| 设置 → 通用：更换头像 | 无 | 16 |
| 设置 → 通用：名称 | 无 | 16 |
| 设置 → 通用：描述 | 无 | 16 |
| 设置 → 通用：运行时 | packages/views/agents/components/inspector/runtime-picker.test.tsx | 16 |
| 设置 → 通用：模型 | packages/views/agents/components/inspector/model-picker.test.tsx | 16 |
| 设置 → 通用：思考等级 | packages/views/agents/components/inspector/thinking-picker.test.tsx, packages/views/agents/components/inspector/thinking-prop-row.test.tsx | 16 |
| 设置 → 通用：速度（服务等级） | packages/views/agents/components/inspector/service-tier-setting-field.test.tsx | 16 |
| 设置 → 通用：最大并行运行数 | 无 | 16 |
| 设置 → 访问权限：仅自己 / 整个工作区 / 指定成员 | packages/views/agents/components/inspector/access-picker.test.tsx | 16 |
| 设置 → 环境变量：解锁并编辑、逐条或批量编辑、显示/隐藏值、移除变量、保存 | packages/views/agents/components/tabs/env-tab.test.tsx | 16 |
| 设置 → 自定义参数：添加、编辑、移除参数与命令预览 | packages/views/agents/components/tabs/custom-args-tab.test.tsx | 16 |
| 设置 → 路由：Local / Gateway 模式，Host、端口、鉴权 Token、TLS | 无 | 16 |
| 设置 → 上下文压缩：开关与压缩阈值 | packages/views/agents/components/tabs/context-compaction-tab.test.tsx | 16 |
| 切换标签时有未保存修改的提示（继续编辑 / 放弃修改） | 无 | 16 |
| 页面态：不存在、无访问权限、加载失败与重试 | packages/views/agents/components/agent-detail-page.test.tsx | 16 |

### 17 新建智能体

路由：`/:workspaceSlug/agents/new`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 页面标题「创建智能体」与步骤「选择起点」 | 无 | 17 |
| 「返回」回到智能体列表 | 无 | 17 |
| 方式卡：从空白开始（进入手动创建） | 无 | 17 |
| 方式卡：通过 AI 创建（带「推荐」标记，进入对话创建） | 无 | 17 |
| 从小队带参进入时标题变为「为小队创建智能体」 | 无 | 17 |

### 18 小队

路由：`/:workspaceSlug/squads`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 页面标题「小队」与小队总数 | 无 | 18 |
| 「新建小队」按钮 | 无 | 18 |
| 空状态「暂无小队」 | 无 | 18 |
| 范围切换：我的 / 全部（各带数量） | 无 | 18 |
| 筛选：队长 | 无 | 18 |
| 筛选：创建者 | 无 | 18 |
| 清除筛选 | 无 | 18 |
| 显示：排序（小队名、成员、创建时间）与升序 / 降序 | 无 | 18 |
| 显示：列开关（成员、创建者、创建时间） | 无 | 18 |
| 表格列：小队（头像、名称、描述）、队长、成员数、创建者、创建时间 | 无 | 18 |
| 点击行进入小队详情 | 无 | 18 |
| 行操作菜单：在新标签页打开、归档 | 无 | 18 |
| 归档确认弹窗（承接中的任务会转交给队长） | 无 | 18 |
| 无匹配提示 | 无 | 18 |

### 19 Skills

路由：`/:workspaceSlug/skills`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 页面标题「Skills」与说明文案 | 无 | 19 |
| 「了解更多」文档链接 | packages/views/skills/components/skills-page.source-link.test.tsx | 19 |
| 「新建 skill」按钮（打开方式选择） | 无 | 19 |
| 介绍横幅：在工作区内共享、本地运行时里的 skill 默认私有 | 无 | 19 |
| 搜索 skill | 无 | 19 |
| 当前结果 / 全部 skill 计数 | 无 | 19 |
| 筛选：使用状态（使用中、未使用） | 无 | 19 |
| 筛选：标签（筛选搜索、无标签提示、多选） | packages/views/skills/components/skills-page-filter-predicate.test.ts | 19 |
| 「管理标签」与 Skill 标签弹窗 | 无 | 19 |
| 清除筛选 | 无 | 19 |
| 显示：排序（名称、被谁使用、更新时间、创建时间）与升序 / 降序 | 无 | 19 |
| 显示：列开关 | 无 | 19 |
| 表格列：名称（只读锁标记）、被谁使用、来源、添加者、更新时间、创建时间 | 无 | 19 |
| 来源标注：手动创建、来自某运行时、来自 ClawHub、来自 Skills.sh、来自 GitHub | 无 | 19 |
| 点击行打开 Skill 详情 | packages/views/skills/components/skills-page.source-link.test.tsx | 19 |
| 点击来源链接（不触发行跳转） | packages/views/skills/components/skills-page.source-link.test.tsx | 19 |
| 行操作菜单：添加到智能体（我的智能体 / 其他智能体、搜索、添加） | 无 | 19 |
| 行操作菜单：从来源更新（确认弹窗） | packages/views/skills/components/refresh-skill-dialog.test.tsx | 19 |
| 行操作菜单：删除 | 无 | 19 |
| 批量工具栏：已选 N 项、清除选择 | 无 | 19 |
| 批量：更新所选（可更新与跳过统计） | packages/views/skills/components/skill-list-actions.test.tsx | 19 |
| 批量：删除所选（确认弹窗） | 无 | 19 |
| 新建 skill 方式选择：手动创建、从本地导入、从 URL 导入、从运行时复制 | 无 | 19 |
| 手动创建：名称、描述与「创建 skill」 | 无 | 19 |
| 从本地导入：选择文件夹或 .skill/.zip 压缩包 | packages/views/skills/components/create-skill-dialog.test.tsx | 19 |
| 从 URL 导入：Skill URL 与支持的来源 | 无 | 19 |
| 从运行时复制：选择运行时、搜索本地 skill、单个或批量导入 | packages/views/skills/components/runtime-local-skill-import-panel.test.tsx | 19 |
| 从运行时复制：同名冲突处理（覆盖、重命名、跳过） | packages/views/skills/components/runtime-local-skill-import-panel.test.tsx | 19 |
| 空状态与无匹配提示 | 无 | 19 |
| 列表加载失败与「重试」 | 无 | 19 |

### 20 运行时

路由：`/:workspaceSlug/runtimes`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 页面标题「运行时」、机器数与说明文案、「了解更多」 | 无 | 20 |
| 「添加电脑」按钮 | 无 | 20 |
| 机器行：图标、名称、副标题、健康状态标签 | 无 | 20 |
| 机器行：运行时数量与各 provider 图标（超出折叠为 +N） | 无 | 20 |
| 机器行：工作负载（N 个运行中、N 个排队中、全部空闲） | 无 | 20 |
| 机器行：最近活跃时间 | 无 | 20 |
| 机器行：「本机」标记与名称来源 | packages/views/runtimes/components/runtime-machines.test.ts | 20 |
| 点击机器行进入机器详情 | 无 | 20 |
| 尚未连到机器的自定义运行时区块与行操作 | packages/views/runtimes/components/pending-runtime.test.ts | 20 |
| Mika 引导卡片（运行时可用时出现）与选择电脑弹窗 | packages/views/runtimes/components/mika-runtime-choice.test.tsx | 20 |
| 添加电脑弹窗：步骤 1 构建 CLI，选择系统（macOS/Linux、Windows）并复制命令 | packages/views/runtimes/components/connect-remote-dialog.test.tsx | 20 |
| 添加电脑弹窗：步骤 2 启动守护进程并复制命令 | packages/views/runtimes/components/connect-remote-dialog.test.tsx | 20 |
| 添加电脑弹窗：等待电脑上线、「取消」 | packages/views/runtimes/components/connect-remote-dialog.test.tsx | 20 |
| 添加电脑弹窗：排障（改用 token 登录、检查状态、查看日志） | packages/views/runtimes/components/connect-remote-dialog.test.tsx | 20 |
| 添加电脑弹窗：连接成功后的「查看运行时」「创建智能体」 | packages/views/runtimes/components/connect-remote-dialog.test.tsx | 20 |
| 空状态「还没有运行时」与说明 | 无 | 20 |
| 运行中时的「正在启动本地运行时」提示 | 无 | 20 |

### 21 自动化

路由：`/:workspaceSlug/autopilots`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 页面标题「自动化」与自动化总数 | 无 | 21 |
| 标签页：自动化 / 任务唤醒（标签写入 URL） | packages/views/autopilots/components/autopilots-page.test.tsx | 21 |
| 「新建自动化」按钮 | 无 | 21 |
| 空状态：6 个模板卡片（每日新闻摘要、PR review 提醒、Bug 分诊、每周进度报告、依赖审计、文档检查） | 无 | 21 |
| 空状态：「从空白开始」按钮 | 无 | 21 |
| 范围切换：全部 / 启用 / 已暂停（带数量） | 无 | 21 |
| 筛选：执行者 | 无 | 21 |
| 筛选：触发器（时间表、Webhook、API） | 无 | 21 |
| 筛选：模式（创建任务、仅运行） | 无 | 21 |
| 筛选：创建者 | 无 | 21 |
| 清除筛选 | 无 | 21 |
| 显示：排序（名称、上次运行、下次运行、创建时间）与升序 / 降序 | 无 | 21 |
| 显示：列开关（执行者、触发器、上次运行、下次运行、模式、创建者、创建时间） | 无 | 21 |
| 表格列：名称、执行者、触发器、上次运行、下次运行、模式、创建者、创建时间 | 无 | 21 |
| 点击行进入自动化详情 | 无 | 21 |
| 行操作菜单：在新标签页打开、暂停、恢复、删除 | 无 | 21 |
| 删除确认弹窗（连带触发器与运行历史） | 无 | 21 |
| 批量工具栏：已选 N 项、清除选择、暂停、恢复、删除 | 无 | 21 |
| 列表加载失败与「重试」 | 无 | 21 |
| 无匹配提示 | 无 | 21 |
| 新建 / 编辑弹窗：名称 | packages/views/autopilots/components/autopilot-dialog.validation.test.tsx | 21 |
| 新建 / 编辑弹窗：Runbook（智能体每次运行读取） | 无 | 21 |
| 新建 / 编辑弹窗：智能体或小队（执行方）选择 | packages/views/autopilots/components/autopilot-dialog.validation.test.tsx | 21 |
| 新建 / 编辑弹窗：关联项目（同时决定运行时的执行环境） | packages/views/autopilots/components/autopilot-dialog.project.test.tsx | 21 |
| 新建 / 编辑弹窗：输出模式（创建任务 / 仅运行） | 无 | 21 |
| 新建 / 编辑弹窗：订阅者（添加、搜索、移除） | 无 | 21 |
| 新建 / 编辑弹窗：触发方式（时间表 / Webhook） | 无 | 21 |
| 新建 / 编辑弹窗：时间表编辑器（定点或按间隔、每天/每周特定几天/每月、时区、下一批运行预览） | packages/views/autopilots/components/schedule-editor/schedule-editor.test.tsx | 21 |
| 新建 / 编辑弹窗：Cron 文本编辑与校验 | packages/views/autopilots/components/schedule-editor/schedule-editor.test.tsx | 21 |
| 新建 / 编辑弹窗：Webhook 事件过滤（事件、动作、添加、移除） | packages/views/autopilots/components/webhook-event-filter-section.test.tsx | 21 |
| 新建 / 编辑弹窗：管理访问（已授权成员、添加、移除） | 无 | 21 |
| 创建 / 保存结果提示（含时间表保存失败的部分成功） | packages/views/autopilots/components/autopilot-dialog-i18n.test.ts | 21 |
| 任务唤醒：范围切换（待唤醒或执行中、全部、已关闭、已结束、已暂停） | packages/views/autopilots/components/workspace-wakeups.test.tsx | 21 |
| 任务唤醒：筛选 来源 / 触发条件 / 唤醒对象 | packages/views/autopilots/components/workspace-wakeups.test.tsx | 21 |
| 任务唤醒：搜索任务或智能体 | packages/views/autopilots/components/workspace-wakeups.test.tsx | 21 |
| 任务唤醒：表格列（任务、触发条件、唤醒对象、来源、有效期、近 7 天运行、开启） | packages/views/autopilots/components/workspace-wakeups.test.tsx | 21 |
| 任务唤醒：逐行开启 / 关闭开关 | packages/views/autopilots/components/workspace-wakeups.test.tsx | 21 |
| 任务唤醒：选行、关闭所选（确认弹窗与批量结果） | packages/views/autopilots/components/workspace-wakeups.test.tsx | 21 |
| 任务唤醒：分页（上一页 / 下一页、第 N 页共 M 项） | packages/views/autopilots/components/workspace-wakeups.test.tsx | 21 |
| 任务唤醒：「新建唤醒」选择任务后创建 | packages/views/autopilots/components/workspace-wakeups.test.tsx | 21 |
| 任务唤醒：自动暂停横幅与「查看」 | packages/views/autopilots/components/workspace-wakeups.test.tsx | 21 |
| 任务唤醒：只读规则不可选与清单加载失败提示 | packages/views/autopilots/components/workspace-wakeups.test.tsx | 21 |

### 22 统计

路由：`/:workspaceSlug/usage`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 页面标题「统计」、时区与更新时间 | 无 | 22 |
| 「刷新」按钮 | 无 | 22 |
| 标签页：用量 / 错误（标签写入 URL，可直接链接） | packages/views/dashboard/components/dashboard-page.test.tsx | 22 |
| 时间范围切换（1d、7d、30d、90d、180d） | 无 | 22 |
| 项目筛选（全部项目 / 指定项目） | 无 | 22 |
| 用量：KPI（费用、Token 含输入输出、耗时含运行次数、运行数含失败数） | packages/views/dashboard/components/dashboard-page.test.tsx | 22 |
| 用量：趋势卡维度切换（按天 / 按周） | 无 | 22 |
| 用量：趋势卡指标切换（Token、费用、耗时、运行数）与图例 | 无 | 22 |
| 用量：排行榜排序（Token、费用、耗时、运行数） | packages/views/dashboard/components/dashboard-page.test.tsx | 22 |
| 用量：排行榜展开全部 / 只看前 N 个 | packages/views/dashboard/components/dashboard-page.test.tsx | 22 |
| 用量：排行榜列（智能体、Token、费用、耗时、运行数）与已删除智能体、其他智能体分桶 | packages/views/dashboard/components/dashboard-page.test.tsx | 22 |
| 用量：未上报用量的运行提示 | packages/views/dashboard/components/dashboard-page.test.tsx | 22 |
| 用量：无数据空状态 | 无 | 22 |
| 错误：KPI（失败运行、失败率、受影响智能体） | packages/views/dashboard/components/dashboard-page.test.tsx | 22 |
| 错误：失败构成（认证、限流、超时、模型服务、运行时、智能体、其他） | packages/views/dashboard/components/dashboard-page.test.tsx | 22 |
| 错误：错误码展开 / 收起、展开全部与只看前 N 个 | packages/views/dashboard/components/dashboard-page.test.tsx | 22 |
| 错误：问题最多的智能体列表与排序（失败数、失败率）、样本不足提示 | packages/views/dashboard/components/dashboard-page.test.tsx | 22 |
| 错误：每日 / 每周错误图 | 无 | 22 |
| 错误：无失败运行时的空状态 | 无 | 22 |

### 23 设置-个人

路由：`/:workspaceSlug/settings?tab=profile`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 从侧边栏进入设置页 | e2e/navigation.spec.ts | 23 |
| 左侧导航按「个人 / 工作区」分组、子组标题与每页入口 | packages/views/settings/components/settings-page.test.tsx | 23 |
| 成员只能查看的页面在导航里带锁标记 | packages/views/settings/components/settings-page.test.tsx | 23 |
| 设置页搜索框：输入即筛、↑↓ 选择、回车打开、Esc 清空 | packages/views/settings/components/settings-page.test.tsx, packages/views/settings/components/settings-search.test.ts | 23 |
| 窄屏时左侧导航换成下拉选择器 | packages/views/settings/components/settings-page.test.tsx | 23 |
| 打开旧书签（如 ?tab=issue）时落到对应的新页面 | packages/views/settings/components/settings-page.test.tsx | 23 |
| 个人资料：上传 / 更换头像 | packages/views/common/avatar-upload-control.test.tsx | 23 |
| 个人资料：修改姓名 | 无 | 23 |
| 个人资料：编辑「关于你」（每次运行提供给为你工作的智能体，有长度上限） | 无 | 23 |
| 个人资料：修改密码（当前密码 / 新密码 / 确认新密码，修改后重新登录） | packages/views/settings/components/password-change-card.test.tsx | 23 |
| 偏好设置：主题——浅色 / 深色 / 跟随系统 | packages/views/settings/components/preferences-tab.test.tsx | 23 |
| 偏好设置：语言——English / 中文 / 한국어 / 日本語 / Français（同步失败提示） | packages/views/settings/components/preferences-tab.test.tsx | 23 |
| 偏好设置：时区选择（可搜索、可清空，影响所有工作区的日期与图表） | packages/views/settings/components/preferences-tab.test.tsx | 23 |
| 偏好设置：固定底部评论栏开关 | packages/views/settings/components/preferences-tab.test.tsx | 23 |
| 偏好设置：回复正在运行的智能体时默认「插话」还是「本轮结束后开始」 | packages/views/settings/components/preferences-tab.test.tsx | 23 |
| 偏好设置：创建任务时显示哪些字段（智能体创建 / 手动创建两列开关） | packages/views/settings/components/preferences-tab.test.tsx | 23 |
| 偏好设置：点击任务卡片或行时打开详情页还是悬浮预览（Shift 反向） | packages/views/settings/components/preferences-tab.test.tsx | 23 |
| 偏好设置：悬浮聊天窗开关 | packages/views/settings/components/preferences-tab.test.tsx | 23 |
| 偏好设置：每组设置标注生效范围（账号同步 / 仅本设备 / 仅本工作区） | packages/views/settings/components/preferences-tab.test.tsx | 23 |
| 通知：六类收件箱通知开关（分配、状态变更、评论、提及、优先级与截止日期、智能体活动） | 无 | 23 |
| 通知：系统通知开关 | 无 | 23 |
| 通知：浏览器通知启用（含被浏览器拦截后的说明） | 无 | 23 |
| 快捷键：操作列表（常规 / 导航两类，含固定的系统快捷键说明） | packages/views/settings/components/keyboard-shortcuts-tab.test.tsx | 23 |
| 快捷键：点一下再按键即录制新组合，立即生效 | packages/views/settings/components/keyboard-shortcuts-tab.test.tsx | 23 |
| 快捷键：单个动作「重置」与「停用」 | packages/views/settings/components/keyboard-shortcuts-tab.test.tsx | 23 |
| 快捷键：冲突、被浏览器占用、会干扰输入等按键的拒绝提示 | packages/views/settings/components/keyboard-shortcuts-tab.test.tsx | 23 |
| 快捷键：按操作名筛选 | 无 | 23 |
| 快捷键：「恢复默认设置」并二次确认 | packages/views/settings/components/keyboard-shortcuts-tab.test.tsx | 23 |
| 已连接应用：应用卡片上的连接状态与「最近使用」 | packages/views/settings/components/composio-tab.test.tsx | 23 |
| 已连接应用：连接 / 断开（断开需确认） | packages/views/settings/components/composio-tab.test.tsx | 23 |
| 已连接应用：令牌过期后的「重新连接」 | packages/views/settings/components/composio-tab.test.tsx | 23 |
| 已连接应用：连接完成后回到设置页的提示与列表刷新 | packages/views/settings/components/composio-tab.test.tsx, e2e/settings.spec.ts | 23 |
| 已连接应用：搜索应用（toolkit） | 无 | 23 |
| 已连接应用：实例未启用 Composio 时整页不出现 | packages/views/settings/components/connected-apps-tab.test.tsx | 23 |
| API Token：创建（名称 + 有效期 30 天 / 90 天 / 1 年 / 永不过期） | 无 | 23 |
| API Token：新建后只显示一次 token，复制 token 与复制 CLI 登录命令 | 无 | 23 |
| API Token：列表里的前缀、创建时间、最后使用、过期时间 | 无 | 23 |
| API Token：吊销（确认对话框） | 无 | 23 |
| API Token：「使用方法」说明与安全提示 | 无 | 23 |
| 用户与注册（仅实例管理员可见）：新增用户并生成邀请链接、复制邀请链接 | packages/views/settings/components/instance-users-tab.test.tsx | 23 |

### 24 设置-工作区

路由：`/:workspaceSlug/settings?tab=workspace`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 常规：上传 / 更换工作区图标 | packages/views/common/avatar-upload-control.test.tsx | 24 |
| 常规：修改工作区名称（保存后侧边栏立即更新） | packages/views/settings/components/workspace-tab.test.tsx, e2e/settings.spec.ts | 24 |
| 常规：修改工作区描述 | packages/views/settings/components/workspace-tab.test.tsx | 24 |
| 常规：编辑「智能体上下文」（每次运行提供给该工作区的智能体） | packages/views/settings/components/workspace-tab.test.tsx | 24 |
| 常规：查看工作区 slug（只读，创建后不可改） | packages/views/settings/components/workspace-tab.test.tsx | 24 |
| 常规：复制 slug | 无 | 24 |
| 常规：修改任务前缀（独立对话框、预览新编号、提示外链旧编号会失效） | packages/views/settings/components/workspace-tab.test.tsx | 24 |
| 常规：普通成员看到只读值并可看到该联系谁 | packages/views/settings/components/workspace-tab.test.tsx | 24 |
| 常规：离开工作区（确认框；唯一所有者 / 唯一成员时给出原因并禁用） | 无 | 24 |
| 常规：删除工作区（输入名称确认） | packages/views/settings/components/delete-workspace-dialog.test.tsx | 24 |
| 成员：成员列表（角色徽章、本人标记） | packages/views/settings/components/members-tab.test.tsx | 24 |
| 成员：按姓名或邮箱筛选成员 | packages/views/settings/components/members-tab.test.tsx | 24 |
| 成员：修改成员角色 | 无 | 24 |
| 成员：移除成员（确认框） | 无 | 24 |
| 成员：邀请成员对话框（邮箱 + 角色 + 有效期 24 小时 / 7 天 / 30 天 / 永不过期） | packages/views/settings/components/members-tab.test.tsx | 24 |
| 成员：待处理邀请列表（发送时间、到期时间） | packages/views/settings/components/members-tab.test.tsx | 24 |
| 成员：撤销待处理的邀请 | 无 | 24 |
| 成员：复制分享链接（公开地址，带 /join?code=） | packages/views/settings/components/members-tab.test.tsx | 24 |
| 成员：创建分享链接（角色、使用次数上限、过期时间） | 无 | 24 |
| 成员：撤销分享链接 | 无 | 24 |
| 成员：普通成员看不到邀请与分享链接控件 | packages/views/settings/components/members-tab.test.tsx | 24 |
| 任务状态：按未开始 / 已开始 / 已完成 / 已关闭分组列出状态 | packages/views/settings/components/issue-statuses-tab.test.tsx | 24 |
| 任务状态：拖动或用行菜单上移 / 下移调整顺序（同步看板、列表与菜单） | packages/views/settings/components/issue-statuses-tab.test.tsx | 24 |
| 任务状态：新建状态（名称、分类、描述、颜色、图标形状） | packages/views/settings/components/issue-statuses-tab.test.tsx | 24 |
| 任务状态：编辑自定义状态（分类创建后不可改） | packages/views/settings/components/issue-statuses-tab.test.tsx | 24 |
| 任务状态：归档状态；仍有任务在用时的拦截与「查看任务」 | packages/views/settings/components/issue-statuses-tab.test.tsx | 24 |
| 任务状态：「显示已归档」开关 | packages/views/settings/components/issue-statuses-tab.test.tsx | 24 |
| 任务状态：内置状态不可编辑 / 归档的说明 | packages/views/settings/components/issue-statuses-tab.test.tsx | 24 |
| 任务状态：「PR 合并后自动进入」徽章与跳到代码页设置 | packages/views/settings/components/issue-statuses-tab.test.tsx | 24 |
| 任务状态：普通成员看不到新建与行操作 | packages/views/settings/components/issue-statuses-tab.test.tsx | 24 |
| 唤醒：子任务结束时的全局默认规则开关 | packages/views/settings/components/wakeups-tab.test.tsx | 24 |
| 唤醒：默认指令编辑与保存（留空使用内置指令） | packages/views/settings/components/wakeups-tab.test.tsx | 24 |
| 唤醒：成员只读查看默认值 | packages/views/settings/components/wakeups-tab.test.tsx | 24 |
| 标签：「任务」与「Skill」两套标签分类切换（没有智能体标签） | packages/views/settings/components/labels-tab.test.tsx | 24 |
| 标签：新建标签入口 | packages/views/settings/components/labels-tab.test.tsx | 24 |
| 标签：编辑标签（名称、描述、颜色） | 无 | 24 |
| 标签：删除标签（说明会从多少个资源上移除） | 无 | 24 |
| 标签：按名称 / 描述筛选、使用数量列 | 无 | 24 |
| 属性：新建属性（名称、类型、描述、选项、图标） | e2e/property-icons.spec.ts | 24 |
| 属性：编辑属性与更换 / 移除图标 | e2e/property-icons.spec.ts | 24 |
| 属性：选项很多时对话框内滚动、保存按钮始终可点 | e2e/property-editor-overflow.spec.ts | 24 |
| 属性：归档 / 恢复属性 | 无 | 24 |
| 属性：按名称筛选、「显示已归档」、启用中属性数量上限提示 | 无 | 24 |
| 快捷操作：列表（名称、执行者、可见范围、使用次数、更新时间） | 无 | 24 |
| 快捷操作：新建 / 编辑（名称、执行者、谁可以使用、提示词） | 无 | 24 |
| 快捷操作：归档 / 恢复 / 删除 | 无 | 24 |
| 快捷操作：按名称搜索 | 无 | 24 |

### 25 设置-连接

路由：`/:workspaceSlug/settings?tab=channels`

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 代码：连接 GitHub（打开安装流程，成功后回到本页） | packages/views/settings/components/code-tab.test.tsx | 25 |
| 代码：显示已连接账号与「由谁连接」 | packages/views/settings/components/code-tab.test.tsx | 25 |
| 代码：暂停 / 恢复 GitHub 功能（暂停时联动开关不可改） | packages/views/settings/components/code-tab.test.tsx | 25 |
| 代码：断开 GitHub App（确认框，说明已有 PR 镜像会保留） | packages/views/settings/components/code-tab.test.tsx | 25 |
| 代码：三个联动开关（PR 侧栏、Co-authored-by trailer、任务 ↔ PR 自动关联） | packages/views/settings/components/code-tab.test.tsx | 25 |
| 代码：PR 合并后任务状态改为（不改变 / 指定状态） | packages/views/settings/components/code-tab.test.tsx | 25 |
| 代码：代码仓库列表（只读行 + 编辑 / 删除） | packages/views/settings/components/repositories-section.test.tsx | 25 |
| 代码：添加仓库（手填 URL 与描述，或从 GitHub 选择，标记私有 / 已归档） | packages/views/settings/components/repositories-section.test.tsx | 25 |
| 代码：自托管 Git 连接（提供方、实例地址、访问令牌） | packages/views/settings/components/code-vcs.test.tsx | 25 |
| 代码：Webhook 地址与密钥（只显示一次）、重新生成、断开连接 | packages/views/settings/components/code-vcs.test.tsx | 25 |
| 代码：成员只读并提示联系管理员 | packages/views/settings/components/code-tab.test.tsx, packages/views/settings/components/repositories-section.test.tsx, packages/views/settings/components/code-vcs.test.tsx | 25 |
| 消息渠道：渠道列表（名称、一句说明、已连接 / 未连接状态） | packages/views/settings/components/channels-tab.test.tsx | 25 |
| 消息渠道：进入单个渠道后有面包屑返回渠道列表 | packages/views/settings/components/channels-tab.test.tsx | 25 |
| 消息渠道：未启用的渠道不出现在列表，指向它的深链回落到列表 | packages/views/settings/components/channels-tab.test.tsx | 25 |
| 飞书：绑定到飞书（扫码安装，失败一类的终止态提示）、已连接徽章、「在飞书中管理」、断开 | packages/views/settings/components/lark-tab.test.tsx | 25 |
| 飞书：部署未配置飞书时的未启用说明 | packages/views/settings/components/lark-tab.test.tsx | 25 |
| Slack：粘贴 Bot token 与 App token 连接、断开 | packages/views/settings/components/slack-tab.test.tsx | 25 |
| Slack：部署未配置 Slack 时的未启用说明 | packages/views/settings/components/slack-tab.test.tsx | 25 |
| 钉钉：粘贴 AppKey / AppSecret 连接、断开、显示已关联的 Staff ID | packages/views/settings/components/dingtalk-tab.test.tsx | 25 |
| 钉钉：近期钉群列表（活跃时间、被 @ 次数、长期未活跃、加载更多、忘记群） | packages/views/settings/components/dingtalk-tab.test.tsx | 25 |
| 钉钉：部署未配置钉钉时的未启用说明 | packages/views/settings/components/dingtalk-tab.test.tsx | 25 |
| 企业微信：粘贴 bot id 与 secret 连接、断开 | packages/views/settings/components/wecom-tab.test.tsx | 25 |
| 企业微信：部署未配置时的未启用说明 | packages/views/settings/components/wecom-tab.test.tsx | 25 |
| Telegram：粘贴 bot token 连接、在 Telegram 中打开、断开 | packages/views/settings/components/telegram-tab.test.tsx | 25 |
| Telegram：部署未配置时的未启用说明 | packages/views/settings/components/telegram-tab.test.tsx | 25 |
| MCP 服务器：共享服务器列表（传输方式、已禁用标记、被几个智能体使用、未分配给任何智能体） | packages/views/settings/components/mcp-tab.test.tsx | 25 |
| MCP 服务器：添加（名称、STDIO 或 Streamable HTTP、命令 / 参数 / 环境变量，或 URL / 请求头，或直接编辑 JSON） | packages/views/settings/components/mcp-tab.test.tsx | 25 |
| MCP 服务器：重命名（只改名称，不替换配置） | packages/views/settings/components/mcp-tab.test.tsx | 25 |
| MCP 服务器：替换完整配置（改完不再回显） | packages/views/settings/components/mcp-tab.test.tsx | 25 |
| MCP 服务器：移除（确认框，说明从服务库与所有已分配智能体里移除） | packages/views/settings/components/mcp-tab.test.tsx | 25 |
| MCP 服务器：三种接入方式的说明区块（MCP / 已连接应用 / 插件） | 无 | 25 |
| MCP 服务器：普通成员看不到任何写操作 | packages/views/settings/components/mcp-tab.test.tsx | 25 |
| 插件：安装前的权限清单同意页（新增权限单独标出，可升级） | packages/views/settings/components/plugins-tab.test.tsx | 25 |
| 插件：已安装列表（版本、作者、升级到新版本） | packages/views/settings/components/plugins-tab.test.tsx | 25 |
| 插件：启用 / 停用、卸载（连同其存储与密钥一起删除） | packages/views/settings/components/plugins-tab.test.tsx | 25 |
| 插件：配置表单（密钥保存后不回显，输入新值即替换） | packages/views/settings/components/plugins-tab.test.tsx | 25 |
| 插件：MCP 工具批准（选择允许智能体调用的工具） | 无 | 25 |
| 插件：自动运行计划（下次运行时间、最近投递记录） | 无 | 25 |
| 插件：发布插件（上传 zip 产物包、删除已发布的插件） | packages/views/settings/components/plugins-tab.test.tsx | 25 |
| 插件：普通成员无法管理插件 | packages/views/settings/components/plugins-tab.test.tsx | 25 |
| 插件：功能开关关闭时整页从导航与直链里消失 | packages/views/settings/components/settings-page.test.tsx | 25 |

## 其余路由（清单覆盖，无截图）

### 18 小队详情（/squads/<id>，无截图）

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 面包屑返回小队列表 | 无 | 无截图 |
| 更换小队头像 | 无 | 无截图 |
| 重命名小队 | 无 | 无截图 |
| 编辑小队描述 | 无 | 无截图 |
| 详情：队长、成员数、创建者、创建时间、更新时间 | 无 | 无截图 |
| 「归档」按钮与确认（归档后不可恢复） | 无 | 无截图 |
| 标签页：成员 / 指引 | 无 | 无截图 |
| 成员：添加成员弹窗（成员或智能体、搜索、角色） | 无 | 无截图 |
| 成员：创建智能体（带入当前小队） | 无 | 无截图 |
| 成员行：编辑角色 | 无 | 无截图 |
| 成员行：设为队长 | 无 | 无截图 |
| 成员行：从小队移除 | 无 | 无截图 |
| 成员行：打开智能体详情 | 无 | 无截图 |
| 成员行：状态、进行中任务、最近活动 | 无 | 无截图 |
| 指引：编辑并保存队长指令 | 无 | 无截图 |
| 未保存修改提示（继续编辑 / 放弃） | 无 | 无截图 |

### 19 Skill 详情（/skills/<id>，无截图）

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 面包屑返回全部 skill | 无 | 无截图 |
| 头部：来源、文件数、被多少个智能体使用、更新时间 | 无 | 无截图 |
| 来源标签链到原站（无法安全链接时降级为纯文本） | packages/views/skills/components/skill-detail-page.test.tsx | 无截图 |
| 「更新」按钮：从来源重新下载并替换（确认弹窗） | packages/views/skills/components/refresh-skill-dialog.test.tsx | 无截图 |
| 「删除 skill」与确认（会从使用它的智能体上移除） | 无 | 无截图 |
| 只读权限提示（非创建者、非管理员） | packages/views/skills/components/skill-detail-page.test.tsx | 无截图 |
| 标签页：概览 / 文件数 | packages/views/skills/components/skill-detail-page.test.tsx | 无截图 |
| 概览：名称 | 无 | 无截图 |
| 概览：描述与字符数 | packages/views/skills/components/skill-detail-page.test.tsx | 无截图 |
| 概览：标签、被哪些智能体使用 | 无 | 无截图 |
| 文件：文件树（主文件与附属文件） | packages/views/skills/components/file-tree.test.tsx | 无截图 |
| 文件：新建文件（路径校验） | packages/views/skills/components/skill-file-path.test.tsx | 无截图 |
| 文件：文件行操作（编辑、重命名、删除，右键同样可用） | packages/views/skills/components/file-tree.test.tsx | 无截图 |
| 文件：视图模式切换（预览 / 编辑 / 纯文本） | packages/views/skills/components/skill-detail-page.test.tsx | 无截图 |
| 文件：删除当前文件 | 无 | 无截图 |
| 保存条：已修改项、丢弃、保存修改 | packages/views/skills/components/skill-detail-page.test.tsx | 无截图 |
| 他人同时修改时的冲突提示（保留我的修改 / 拉取对方版本） | packages/views/skills/components/skill-detail-page.test.tsx | 无截图 |

### 20 运行时详情（/runtimes/<id>，无截图）

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 面包屑返回运行时列表 | 无 | 无截图 |
| 机器头部：名称、健康状态、「本机」标记、副标题 | 无 | 无截图 |
| 机器头部：运行时数量、工作负载、最近活跃 | 无 | 无截图 |
| 机器头部：CLI 版本与「更新」按钮（只读、本地构建、桌面端管理三种提示） | packages/views/runtimes/components/machine-cli-section.test.tsx, packages/views/runtimes/components/update-section.test.tsx | 无截图 |
| 「重命名机器」与重命名弹窗（留空恢复默认名称） | 无 | 无截图 |
| 「添加自定义运行时」 | packages/views/runtimes/components/runtime-profiles-dialog.test.tsx | 无截图 |
| 这台机器上的运行时列表与表头（运行时、健康度、所有者、智能体、费用 · 7 天、CLI） | 无 | 无截图 |
| 运行时行操作菜单：在新标签页打开、编辑自定义运行时、删除、从工作区删除 | packages/views/runtimes/components/runtime-row-menu.test.tsx | 无截图 |
| 点击运行时行进入该运行时的设置页 | 无 | 无截图 |
| 机器暂无运行时时的空状态与说明 | 无 | 无截图 |
| 机器不存在时的提示与「全部运行时」 | 无 | 无截图 |

### 20 运行时设置（/runtimes/<id>/runtime/<runtimeId>，无截图）

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 面包屑：运行时 / 机器名 / 运行时名，非所有者显示「只读」 | 无 | 无截图 |
| 主卡：provider 图标、运行时名称、健康徽章、最后活跃 | packages/views/runtimes/components/runtime-detail-visibility.test.tsx | 无截图 |
| 主卡：所有者、设备、运行时三项信息 | 无 | 无截图 |
| 主卡：「技术详情」展开（守护进程 CLI、守护进程 ID） | packages/views/runtimes/components/runtime-detail-visibility.test.tsx | 无截图 |
| 用量区块：时间范围切换 | packages/views/runtimes/components/usage-section.test.tsx | 无截图 |
| 用量区块：KPI（费用、缓存节省及命中率、Token） | packages/views/runtimes/components/usage-section.test.tsx | 无截图 |
| 用量区块：按天 / 按周 / 热力图切换 | packages/views/runtimes/components/usage-section.test.tsx | 无截图 |
| 用量区块：指标切换（费用、Token）与图例 | 无 | 无截图 |
| 用量区块：按智能体 / 按模型分摊 | 无 | 无截图 |
| 用量区块：每日明细表与列（日期、模型、输入、输出、缓存读、缓存写） | 无 | 无截图 |
| 用量区块：「设置自定义价格」弹窗（输入、输出、缓存读写，美元/百万 token） | packages/views/runtimes/components/usage-section.test.tsx | 无截图 |
| 右栏「服务中」：绑定到该运行时的智能体及其状态、负载 | 无 | 无截图 |
| 右栏「诊断」：可见性切换（私有 / 公开） | packages/views/runtimes/components/runtime-detail-visibility.test.tsx | 无截图 |
| 右栏「诊断」：「删除运行时」与删除弹窗 | packages/views/runtimes/components/delete-runtime-dialog.test.tsx | 无截图 |
| 删除弹窗：需解绑 N 个智能体时的清单、勾选确认与「解绑并删除」 | packages/views/runtimes/components/delete-runtime-dialog.test.tsx | 无截图 |
| 删除弹窗：自愈提示与自定义运行时配置提示 | packages/views/runtimes/components/delete-runtime-dialog.test.tsx | 无截图 |
| 运行时不存在时的提示 | 无 | 无截图 |

### 20 不可用占位页（/runtimes/cloud、/cloud-runtime/*，无截图）

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 当前为不可用占位页：标题「此页面不可用」、说明「U Workers 不提供这个云服务或计费页面。」 | 无 | 无截图 |
| 「返回工作区」按钮（回到工作区任务页） | 无 | 无截图 |

### 21 自动化详情（/autopilots/<id>，无截图）

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 面包屑返回自动化列表 | 无 | 无截图 |
| 标题旁的启用 / 暂停开关与状态文案 | 无 | 无截图 |
| 「编辑」按钮（打开编辑弹窗） | 无 | 无截图 |
| 「立即运行」按钮（暂停或归档时不可用） | 无 | 无截图 |
| 立即运行的结果提示（成功、跳过、失败）与被拦截原因 | packages/views/autopilots/components/run-now-toast.test.ts | 无截图 |
| 执行智能体失去运行时时的暂停横幅与「绑定运行时」 | 无 | 无截图 |
| 属性：智能体、创建者、输出模式、关联项目 | 无 | 无截图 |
| 属性：订阅者（创建任务模式下显示） | 无 | 无截图 |
| 属性：Prompt 只读内容 | 无 | 无截图 |
| 触发器列表：标签、下次运行、已禁用 / 已弃用标记 | packages/views/autopilots/components/trigger-row.test.tsx | 无截图 |
| 触发器：Webhook URL 的隐藏 / 显示、复制、重新生成（确认弹窗） | packages/views/autopilots/components/webhook-url-field.test.tsx | 无截图 |
| 触发器：编辑时间表（标签、启用、时间表） | packages/views/autopilots/components/edit-schedule-trigger-dialog.test.tsx | 无截图 |
| 触发器：删除（确认弹窗） | 无 | 无截图 |
| 「添加触发器」：类型（时间表 / Webhook）、标签与添加 | 无 | 无截图 |
| Webhook 投递：列表与状态（排队中、已分发、已拒绝、已忽略、失败） | 无 | 无截图 |
| Webhook 投递详情（接收时间、尝试次数、响应状态、去重 Key、原始 Body、保留的请求头） | 无 | 无截图 |
| Webhook 投递：重放（签名失败或已拒绝时不可用） | 无 | 无截图 |
| Webhook payload 预览与复制（超长截断） | packages/views/autopilots/components/webhook-payload-preview.test.tsx | 无截图 |
| 运行历史：状态、来源（时间表、手动、Webhook、API）、已关联任务、查看执行日志、跳过的运行分组 | 无 | 无截图 |
| 危险操作：删除自动化（确认弹窗） | 无 | 无截图 |
| 自动化不存在时的提示 | 无 | 无截图 |

### /auth/callback

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| Google 授权回来后的落地判断（已上手 → 工作区；未上手 → 引导页或待处理邀请） | apps/web/app/auth/callback/page.test.tsx | 无截图 |
| 尊重 ?next 目标，非站内地址则忽略 | apps/web/app/auth/callback/page.test.tsx | 无截图 |
| 错误页：拒绝授权、缺少授权码、账号停用、注册被禁、邮箱不允许、授权码失效 | apps/web/app/auth/callback/page.test.tsx, e2e/auth-callback-locale.spec.ts | 无截图 |
| 错误文案跟随当前语言 | apps/web/app/auth/callback/page.test.tsx, e2e/auth-callback-locale.spec.ts | 无截图 |
| 错误页「返回登录」按钮 | 无 | 无截图 |
| 带合法 CLI 回调参数时把 token 转发回本地端口 | apps/web/app/auth/callback/page.test.tsx | 无截图 |
| 桌面端 platform=desktop 的交接 | apps/web/app/auth/callback/page.test.tsx | 无截图 |

### /invitations

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 待处理邀请列表与勾选（工作区名、邀请人、角色） | packages/views/invitations/invitations-page.test.tsx | 无截图 |
| 「加入 N 个工作区」批量接受 | packages/views/invitations/invitations-page.test.tsx | 无截图 |
| 「跳过，创建我自己的工作区」 | packages/views/invitations/invitations-page.test.tsx | 无截图 |
| 没有待处理邀请时的空态与继续按钮 | packages/views/invitations/invitations-page.test.tsx | 无截图 |
| 退出登录 | 无 | 无截图 |

### /invite/[id]

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 邀请详情（工作区名、邀请人以什么身份邀请） | 无 | 无截图 |
| 「接受并加入」 | 无 | 无截图 |
| 「拒绝」 | 无 | 无截图 |
| 已接受 / 已拒绝 / 已过期的状态提示 | 无 | 无截图 |
| 找不到邀请时的说明与「前往主页」 | 无 | 无截图 |
| 页头的返回与退出登录 | 无 | 无截图 |

### /onboarding

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 欢迎页：服务地址、「继续创建工作区」、退出登录 | packages/views/onboarding/steps/step-welcome.test.tsx | 无截图 |
| 欢迎页「我已经用过了」跳过（有工作区时才出现） | packages/views/onboarding/steps/step-welcome.test.tsx | 无截图 |
| 步骤侧栏：当前步骤标记、已完成步骤可点回、进行中不可跳步 | packages/views/onboarding/components/step-shell.test.tsx | 无截图 |
| 窄屏顶部进度条 + 返回 + 退出登录 | packages/views/onboarding/components/step-shell.test.tsx | 无截图 |
| 工作区步骤：名称、URL、任务前缀、随机名 | packages/views/onboarding/steps/step-workspace.test.tsx | 无截图 |
| 工作区步骤：复用已有工作区（打开）或新建一个 | packages/views/onboarding/onboarding-flow-mode.test.tsx | 无截图 |
| 工作区步骤：实例禁用创建工作区时的提示与退出登录 | packages/views/onboarding/steps/step-workspace.test.tsx | 无截图 |
| 运行时步骤：扫描本机运行时、在线 / 离线、刷新、选择后继续、暂时跳过 | packages/views/onboarding/steps/step-runtime-connect.test.tsx | 无截图 |
| 运行时步骤：CLI 安装说明弹窗（按系统切换命令、复制） | packages/views/onboarding/steps/cli-install-instructions.test.tsx | 无截图 |
| 任务步骤：「与 Siman 开始」或「进入工作区」 | packages/views/onboarding/onboarding-local-flow.test.tsx | 无截图 |

### /workspaces/new

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 从工作区切换菜单进入「创建工作区」 | 无 | 无截图 |
| 直接停在「工作区」步骤（不播产品介绍），已有工作区时可取消返回 | packages/views/onboarding/onboarding-flow-mode.test.tsx | 无截图 |

### /members/[id]

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 成员身份卡（头像、姓名、角色徽章、邮箱） | 无 | 无截图 |
| 面包屑（工作区 › 成员 › 姓名） | 无 | 无截图 |
| 该成员的任务面板：搜索、已分配 / 已创建切换 | packages/views/common/actor-issues-panel.test.tsx | 无截图 |
| 该成员的任务面板：显示设置与筛选条件 | 无 | 无截图 |
| 成员已不在工作区时的空态 | 无 | 无截图 |

### /attachments/[id]/preview

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 独立整页预览（复用附件查看器，没有关闭按钮，标签页标题用文件名） | packages/views/attachments/attachment-preview-page.test.tsx | 无截图 |
| 图片、CSV 表格等按类型渲染 | packages/views/attachments/attachment-preview-page.test.tsx, packages/views/editor/attachment-preview-modal.test.tsx | 无截图 |
| HTML 文件：在地址栏里改路径跳转 | packages/views/attachments/attachment-preview-page.test.tsx | 无截图 |
| HTML 文件：视口宽度切换（自适应 / 桌面 / 平板 / 手机）与缩放比例标注 | packages/views/editor/html-viewport-frame.test.tsx | 无截图 |
| 文本 / 日志类：自动换行、查看源码 | 无 | 无截图 |
| 文件太大或类型不支持时的提示 | packages/views/editor/attachment-preview-modal.test.tsx | 无截图 |
| 记录加载失败时的提示 | packages/views/attachments/attachment-preview-page.test.tsx | 无截图 |

### /chat

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 左侧会话列表（置顶在前、已归档入口、智能体身份） | packages/views/chat/components/chat-thread-list.test.tsx | 无截图 |
| 新建对话 | packages/views/chat/components/chat-new-chat-button.test.tsx | 无截图 |
| 会话行菜单：重命名、删除、停止当前运行、归档 / 取消归档 | packages/views/chat/components/chat-thread-list.test.tsx, packages/views/chat/components/session-rename-input.test.tsx | 无截图 |
| 会话行键盘操作（方向键、回车打开） | packages/views/chat/components/chat-thread-list.test.tsx | 无截图 |
| 消息列表：工具步骤、耗时、复制回复、加载更早、失败原因说明 | packages/views/chat/components/chat-message-list.test.tsx | 无截图 |
| 输入框：发送 / 停止、图片或文件上传、项目上下文选择、@ 提及、草稿保留 | packages/views/chat/components/chat-input.test.tsx | 无截图 |
| 排队消息：引导到当前运行、编辑、移除、全部清空 | packages/views/chat/components/chat-queue.test.tsx | 无截图 |
| 运行状态胶囊（思考中 / 执行命令 / 读取文件 / 离线 / 重连中） | packages/views/chat/components/task-status-pill.test.ts | 无截图 |
| 各种提示横幅（无智能体、智能体已归档、无权限、未绑定运行时、离线） | 无 | 无截图 |
| 会话与地址栏同步（切换会话、?agent= 深链） | packages/views/chat/chat-page.test.tsx | 无截图 |
| 窄屏下列表 / 对话来回切换 | packages/views/chat/chat-page.test.tsx | 无截图 |
| 首屏开场建议与「自定义开场建议」 | packages/views/chat/components/chat-empty-state.test.tsx | 无截图 |
| 上传的文件绑定到该会话 | e2e/chat-attachments.spec.ts | 无截图 |

### /billing、/cloud-runtime、/runtimes/cloud、/billing/return（已下线）

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 「此页面不可用」说明页与「返回工作区」按钮（无网络请求） | packages/views/common/unavailable-page.test.tsx | 无截图 |
| 设置页里打开已下线的 tab 时给出同样的落地页 | packages/views/settings/components/settings-page.test.tsx | 无截图 |

### /dingtalk/bind

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 「关联钉钉账号」页：正在关联的过渡态 | 无 | 无截图 |
| 未登录时先要求登录，登录后带着 token 回到本页 | 无 | 无截图 |
| 关联成功的说明（可以关掉这个页面） | 无 | 无截图 |
| 链接缺少 token 时的错误说明 | 无 | 无截图 |
| 链接过期 / 已绑定其他账号 / 不是工作区成员 / 未知错误的分类文案 | 无 | 无截图 |

### /lark/bind

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 「绑定飞书账号」页：正在兑换的过渡态 | packages/views/lark/bind-page.test.tsx | 无截图 |
| 未登录时先要求登录，登录后带着 token 回到本页 | packages/views/lark/bind-page.test.tsx | 无截图 |
| 绑定成功的说明 | packages/views/lark/bind-page.test.tsx | 无截图 |
| 链接缺少 token 时的错误说明 | packages/views/lark/bind-page.test.tsx | 无截图 |
| 链接过期 / 已绑定其他账号 / 不是工作区成员 / 未知错误的分类文案 | 无 | 无截图 |
| 反复失败时「让管理员重新发送绑定链接」的提示 | 无 | 无截图 |

### /slack/bind

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 「关联 Slack 账号」页：正在关联的过渡态 | 无 | 无截图 |
| 未登录时先要求登录，登录后带着 token 回到本页 | 无 | 无截图 |
| 关联成功的说明 | 无 | 无截图 |
| 链接缺少 token 时的错误说明 | 无 | 无截图 |
| 链接过期 / 已绑定其他账号 / 不是工作区成员 / 未知错误的分类文案 | 无 | 无截图 |

### /telegram/bind

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 「绑定你的 Telegram 账号」页：正在绑定的过渡态 | packages/views/telegram/bind-page.test.tsx | 无截图 |
| 未登录时先要求登录，登录后带着 token 回到本页 | packages/views/telegram/bind-page.test.tsx | 无截图 |
| 绑定成功的说明 | packages/views/telegram/bind-page.test.tsx | 无截图 |
| 同一个 token 只兑换一次（重复挂载不重复请求） | packages/views/telegram/bind-page.test.tsx | 无截图 |
| 链接缺少 token 时的错误说明 | packages/views/telegram/bind-page.test.tsx | 无截图 |
| 链接过期 / 已绑定其他账号 / 不是工作区成员 / 未知错误的分类文案 | 无 | 无截图 |
| 反复失败时「再给机器人发一条消息」的提示 | 无 | 无截图 |

### /wecom/bind

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 「关联 U Workers 账号」页（企业微信）：正在关联的过渡态 | packages/views/wecom/bind-page.test.tsx | 无截图 |
| 未登录时先要求登录，登录后带着 token 回到本页 | packages/views/wecom/bind-page.test.tsx | 无截图 |
| 关联成功的说明 | packages/views/wecom/bind-page.test.tsx | 无截图 |
| 链接缺少 token 时的错误说明 | packages/views/wecom/bind-page.test.tsx | 无截图 |
| 链接过期 / 已绑定其他账号 / 不是工作区成员 / 未知错误的分类文案 | 无 | 无截图 |
| 反复失败时「重新向机器人发消息」的提示 | 无 | 无截图 |

### /（根路径）

| 功能点 | 现有自动化覆盖 | 截图编号 |
| --- | --- | --- |
| 访问根路径直接重定向到登录页 | e2e/auth.spec.ts | 无截图 |


## 统计

- 功能点总数：**714**
- 有自动化覆盖：**412**（57.7%）
- 无覆盖：**302**（42.3%）

### 「无覆盖」里最高频的 10 项

同一类功能点在多个页面都没有针对性测试，这类缺口最容易在改版里整片丢掉。这 10 项后续补 e2e，本批不写。

| 排名 | 高频缺口（按主题归并） | 无覆盖功能点行数 | 出现的页面 |
| --- | --- | --- | --- |
| 1 | 筛选与筛选菜单 | 40 | 05、06、07、13、15、18、19、21、22、23、24 |
| 2 | 通知、已读与归档 | 26 | 04、05、11、15、16、18、23、24 |
| 3 | 空态与无匹配提示 | 20 | 04、06、07、08、10、13、14、15、18、19、20、21、22 |
| 4 | 搜索 | 16 | 06、07、13、14、15、16、19、21、23、24 |
| 5 | 批量操作（全选、批量菜单、批量归档删除、清除选择） | 15 | 04、08、09、11、13、15、19、21 |
| 6 | 显示设置与列开关 | 13 | 06、13、15、18、19、21 |
| 7 | 排序 | 13 | 06、07、08、09、13、15、18、19、21 |
| 8 | 行/卡片操作菜单（在新标签页打开、复制链接、删除、归档） | 11 | 07、13、15、18、19、21 |
| 9 | 折叠 / 展开 / 收起 | 10 | 09、11、12、14、16、20 |
| 10 | 自动化 / 触发器 / 唤醒 | 8 | 21 |


## 已知边界与值得盯的地方

研究过程中确认的、和「功能一项不能丢」直接相关的边界情况，改版核对时按这里对照：

### 视图与页头

1. 任务页四种视图（看板 / 列表 / 表格 / 泳道）共用同一个页头：视图切换、范围页签、筛选、显示设置、排序、保存视图、批量操作、⌘K。清单里 07 展开一次，08–10 只列各自特有的功能点。
2. 范围页签只有 **全部 / 成员 / 智能体** 三个，没有「项目」页签；项目维度是靠「显示设置·分组=项目」和「筛选·项目」实现的。方案第 1 节里写的「全部/成员/智能体/项目」后半段在代码里不存在，改版不要按四页签设计。
3. 看板的拖拽（跨列改状态、同列排序）是四种视图里 **唯一零测试** 的交互：`drag-utils.test.ts` 只测了 move-payload 计算，`board-view` 的 onDragEnd 没有覆盖；列表和泳道有测试。改版重做卡片时这是首要回归点。
4. 任务详情右栏 12 个区块（属性、快捷操作、唤醒、插件、父任务、重复任务、PR、产物、执行日志、详情、元数据）在 11 展开一次，12 只列评论区。
5. 表格的「表尾计算（计数/总和/平均）」和「行内快速新建（添加任务…）」在中文文案里有表项，但没有任何组件渲染、也没有 UI 入口——这是缺口，不是现状；方案里不要把「计算」当成既有能力。
6. 我的任务（06）没有专属测试文件：范围页签、页签说明、空态、视图栏全是「无」，只有共享页头和 `issue-page-refresh.test.tsx` 能借上。
7. 每页都有几条「用户能看见但确实没测」的：收件箱批量菜单四项、通知相对时间、看板隐藏列侧栏、列表节头全选、表格表头排序/隐藏列/列拖拽、详情页标记完成与归档按钮、右栏父任务区块、评论表情回应与讨论折叠条。重做时最容易悄悄改坏。

### 其余页面

1. 加入页（01）的界面文案全部是硬编码英文：工作区预览、身份徽章、各类错误（含「席位已满」）都没有接 `packages/views/locales/zh-Hans/`，和「站点中文标题、加入页中文化」的遗留待办一致。它的错误文案也不经过统一错误码，只按英文子串判断。
2. 消息渠道（25）默认只有飞书：Slack / 钉钉 / 企业微信 / Telegram 各自受服务端开关控制，开关关闭时该渠道既不出现也不发请求，指向它的 `?integration=` 深链会回落到渠道列表。
3. 个人组的「已连接应用」只在实例启用 Composio 时出现；「用户与注册」只对实例管理员出现；工作区组的「插件」只在插件开关打开时出现。三者都是整行消失，不是置灰。
4. 设置页所有 tab 由 `?tab=` 查询参数驱动，旧值 `?tab=issue`、`?tab=github`、`?tab=repositories`、`?tab=integrations&connected=…` 仍被映射到新页面（GitHub App 与 Composio 的回调依赖它）。
5. 全局快捷键以 `packages/core/shortcuts/definitions.ts` 为准：搜索 Mod+K、新建任务 C、折叠侧边栏 Mod+B、右栏 Mod+/、聊天窗 Mod+J、任务内查找 Mod+F、归档通知 E、发送 Mod+Enter、后退 Mod+[、前进 Mod+]；「前往收件箱 / 聊天 / 我的任务 / 任务 / 项目 / 自动化 / 智能体 / 小队 / 统计 / 运行时 / Skills / 设置」默认没有绑定键。Mod+B 折叠侧边栏没有行为测试。
6. 五个绑定落地页（钉钉 / 飞书 / Slack / Telegram / 企业微信）是同一个五态结构（关联中 → 需登录 → 成功 / 失败），文案按渠道各写一份；钉钉与 Slack 没有对应测试。
7. 小队（18）、新建智能体的方式选择页（17）整页没有任何测试；智能体详情里的「设置 → 路由（Local/Gateway）」「工作区补充」代码存在但零测试；自动化详情里没有访问管理区块（管理访问在新建/编辑弹窗内）。
8. 「运行时详情」「运行时设置」「小队详情」「Skill 详情」「自动化详情」不在截图清单里，但功能清单已覆盖；`/runtimes/cloud`、`/cloud-runtime/*`、`/billing`、`/billing/return` 都是 UnavailablePage 占位页（「此页面不可用」），属于路由覆盖，不做截图。
9. 桌面端在设置里追加「本设备」组（守护进程、更新），并额外提供标签页与缩放的窗口级快捷键；移动端是独立的 Expo App，不复用本文件的 Web 页面。搜索面板里的「切换主题」命令和偏好设置里的主题开关是两条独立入口，改的是同一份本设备偏好。

### 仓库存量问题（不是这次改出来的）

- `e2e/plugin-surface-document.spec.ts` 仍在 import 不存在的 `buildSurfaceDocument`（源码导出的是 `buildSurfaceFrameDocument`），Playwright 加载这个文件就报错，导致当前 main 上 `pnpm exec playwright test --list` 整份列不出来。本次 visual gating 用按文件过滤验证：无 flag 时 `pnpm exec playwright test e2e/visual --list` 为 0 个，`VISUAL_BASELINE=1` 时为 25 个。
- `e2e/onboarding-smoke.spec.ts`、`e2e/onboarding-shell.spec.ts` 断言的是旧版引导流程（「About you」步骤与「Continue on web」按钮），`e2e/navigation.spec.ts` 的标签标题断言仍是「Inbox | Multica」（当前品牌是 U Workers）。这三处按现状会失败，不能当对应功能的覆盖证据。

## 本批没做

- 统计里列出的 10 项高频缺口没有补 e2e（按任务书约定放后续阶段）。
- 截图脚本已写成并通过 `--list` 校验；实际出图需要在起了本地 web + api 的机器上跑，出图后不需要进仓库。

