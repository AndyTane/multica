# UWorkers 可选主题评审

按 CLAWSTA-134 更新后的主题接入方案实施。运行时新增 `packages/ui/styles/themes/uworkers.css`，Web、Desktop 各加一行 import；上游令牌、组件、页面、布局和现有测试保持 main 原样。分支 `feat/redesign-r1-theme`，基线 `012edaa3c87fcda5913de2bbb4d7d06328660f93`。

## 接入与关闭

两个 globals.css 均在现有 import 最后引用共享主题。Desktop 使用仓库相对路径，避免修改 UI 包的 exports。移除该端的一行 import 即恢复上游外观。阅读区用现有 `.rich-text-editor`、`.markdown-content` 类名，增加根选择器的优先级，避免路由 CSS 加载先后影响覆盖；只调整阅读字号和段落/列表行高。

控件、表格、标题和代码使用原有字号，字体栈、全局 14px 阶梯、圆角、侧栏、面板外环、间距和 DOM 均保留。任务描述及评论正文、Markdown 段落使用 15px/24px。不接入 UI Lab 的上游令牌编辑器；关闭主题后仍完整恢复它编辑的原始主题。

## 数值判断

- 浅色次要文字从目标 L=0.54 调为 0.51，色度与色相保持目标值。目标在白底/画布通过，但在 8% 蓝色选中底叠加 hover 后最低约 3.98；最终最低 4.54，满足小号文字 4.5。
- 深色亮蓝按钮使用炭灰文字，满足 4.5。深色选中底使用固定实色，避免透明蓝叠加 raised 底后使链接低于 4.5。
- 成功、警告、危险、信息保留 main 各主题的色度与色相，只调明度；小号文字及 10% 同色底均核对 4.5。
- 图表保留蓝色色相和五阶色度，仅调整明度，所有标记达到 3。图表文字继续使用正文角色。
- 页面标题字重、写死在 TSX 中的颜色、看板列和状态定义均不在本批。

## 全部主题令牌

表内是主题文件实际覆盖的每个变量。背景、卡片与弹出层保留变量别名；其余颜色是 OKLCH 字面值。没有覆盖的动效、尺寸、滚动条和搜索高亮变量继承上游。

### 浅色

| 令牌 | main | 主题最终值 |
|---|---|---|
| `--app-shell` | `oklch(0.964435 0.001327 286.375)` | `oklch(1 0 0)` |
| `--page-canvas` | `oklch(0.988087 0 0)` | `oklch(0.971 0.003 286)` |
| `--surface` | `oklch(1 0 0)` | `oklch(1 0 0)` |
| `--surface-foreground` | `oklch(0.141 0.005 285.823)` | `oklch(0.232 0.004 286)` |
| `--surface-raised` | `oklch(1 0 0)` | `oklch(1 0 0)` |
| `--surface-hover` | `oklch(0.967 0.001 286.375)` | `oklch(0.955 0.003 286)` |
| `--surface-selected` | `oklch(0.95 0.002 286.375)` | `oklch(0.51 0.2 260 / 8%)` |
| `--surface-selected-foreground` | `oklch(0.141 0.005 285.823)` | `oklch(0.51 0.2 260)` |
| `--surface-border` | `oklch(0.92 0.004 286.32)` | `oklch(0.932 0.007 286)` |
| `--surface-shadow` | `0 1px 2px rgb(15 23 42 / 0.04), 0 1px 1px rgb(15 23 42 / 0.03)` | `0 1px 3px rgb(0 0 0 / 0.06), 0 0 0 0.5px rgb(0 0 0 / 0.04)` |
| `--floating-shadow` | `0 16px 40px rgb(15 23 42 / 0.14), 0 3px 10px rgb(15 23 42 / 0.08)` | `0 16px 40px rgb(15 23 42 / 0.14), 0 3px 10px rgb(15 23 42 / 0.08)` |
| `--menu-shadow` | `0 8px 24px rgb(15 23 42 / 0.08), 0 2px 6px rgb(15 23 42 / 0.05)` | `0 8px 24px rgb(15 23 42 / 0.08), 0 2px 6px rgb(15 23 42 / 0.05)` |
| `--background` | `var(--page-canvas)` | `var(--page-canvas)` |
| `--foreground` | `oklch(0.141 0.005 285.823)` | `oklch(0.232 0.004 286)` |
| `--card` | `var(--surface)` | `var(--surface)` |
| `--card-foreground` | `oklch(0.141 0.005 285.823)` | `oklch(0.232 0.004 286)` |
| `--popover` | `var(--surface-raised)` | `var(--surface)` |
| `--popover-foreground` | `oklch(0.141 0.005 285.823)` | `oklch(0.232 0.004 286)` |
| `--primary` | `oklch(0.21 0.006 285.885)` | `oklch(0.51 0.2 260)` |
| `--primary-foreground` | `oklch(0.985 0 0)` | `oklch(1 0 0)` |
| `--secondary` | `oklch(0.967 0.001 286.375)` | `oklch(0.955 0.003 286)` |
| `--secondary-foreground` | `oklch(0.21 0.006 285.885)` | `oklch(0.232 0.004 286)` |
| `--muted` | `oklch(0.967 0.001 286.375)` | `oklch(0.955 0.003 286)` |
| `--muted-foreground` | `oklch(0.505 0.016 285.938)` | `oklch(0.51 0.008 286)` |
| `--faint-foreground` | `oklch(0.606 0.016 285.938)` | `oklch(0.60 0.008 286)` |
| `--accent` | `oklch(0.967 0.001 286.375)` | `oklch(0.955 0.003 286)` |
| `--accent-foreground` | `oklch(0.21 0.006 285.885)` | `oklch(0.232 0.004 286)` |
| `--destructive` | `oklch(0.577 0.245 27.325)` | `oklch(0.50 0.245 27.325)` |
| `--border` | `oklch(0.945 0.003 286.32)` | `oklch(0.932 0.007 286)` |
| `--input` | `oklch(0.92 0.004 286.32)` | `oklch(0.932 0.007 286)` |
| `--ring` | `oklch(0.705 0.015 286.067)` | `oklch(0.51 0.2 260)` |
| `--chart-1` | `oklch(0.55 0.16 255)` | `oklch(0.46 0.16 255)` |
| `--chart-2` | `oklch(0.66 0.13 255)` | `oklch(0.495 0.13 255)` |
| `--chart-3` | `oklch(0.76 0.10 255)` | `oklch(0.53 0.10 255)` |
| `--chart-4` | `oklch(0.85 0.06 255)` | `oklch(0.565 0.06 255)` |
| `--chart-5` | `oklch(0.92 0.03 255)` | `oklch(0.60 0.03 255)` |
| `--sidebar` | `oklch(0.964435 0.001327 286.375)` | `oklch(1 0 0)` |
| `--sidebar-foreground` | `oklch(0.141 0.005 285.823)` | `oklch(0.232 0.004 286)` |
| `--sidebar-primary` | `oklch(0.21 0.006 285.885)` | `oklch(0.51 0.2 260)` |
| `--sidebar-primary-foreground` | `oklch(0.985 0 0)` | `oklch(1 0 0)` |
| `--sidebar-accent` | `oklch(0.935 0.003 286.375)` | `oklch(0.51 0.2 260 / 8%)` |
| `--sidebar-accent-foreground` | `oklch(0.21 0.006 285.885)` | `oklch(0.51 0.2 260)` |
| `--sidebar-border` | `oklch(0.92 0.004 286.32)` | `oklch(0.932 0.007 286)` |
| `--sidebar-ring` | `oklch(0.705 0.015 286.067)` | `oklch(0.51 0.2 260)` |
| `--brand` | `oklch(0.56079497 0.19449888 256.41258418)` | `oklch(0.51 0.2 260)` |
| `--brand-foreground` | `oklch(0.985 0 0)` | `oklch(1 0 0)` |
| `--success` | `oklch(0.55 0.16 145)` | `oklch(0.49 0.16 145)` |
| `--warning` | `oklch(0.75 0.16 85)` | `oklch(0.49 0.16 85)` |
| `--info` | `oklch(0.55 0.18 250)` | `oklch(0.49 0.18 250)` |

### 深色

| 令牌 | main | 主题最终值 |
|---|---|---|
| `--app-shell` | `oklch(0.155 0.005 285.823)` | `oklch(0.227 0.004 286)` |
| `--page-canvas` | `oklch(0.18 0.005 285.823)` | `oklch(0 0 0)` |
| `--surface` | `oklch(0.21 0.006 285.885)` | `oklch(0.227 0.004 286)` |
| `--surface-foreground` | `oklch(0.985 0 0)` | `oklch(0.985 0 0)` |
| `--surface-raised` | `oklch(0.235 0.007 285.885)` | `oklch(0.294 0.004 286)` |
| `--surface-hover` | `oklch(0.274 0.006 286.033)` | `oklch(0.294 0.004 286)` |
| `--surface-selected` | `oklch(0.3 0.006 286.033)` | `oklch(0.273 0.031 252)` |
| `--surface-selected-foreground` | `oklch(0.985 0 0)` | `oklch(0.669 0.181 252)` |
| `--surface-border` | `oklch(1 0 0 / 10%)` | `oklch(0.36 0.007 286)` |
| `--surface-shadow` | `0 1px 2px rgb(0 0 0 / 0.2), 0 1px 1px rgb(0 0 0 / 0.16)` | `0 1px 2px rgb(0 0 0 / 0.2), 0 1px 1px rgb(0 0 0 / 0.16)` |
| `--floating-shadow` | `0 20px 48px rgb(0 0 0 / 0.46), 0 4px 12px rgb(0 0 0 / 0.28)` | `0 20px 48px rgb(0 0 0 / 0.46), 0 4px 12px rgb(0 0 0 / 0.28)` |
| `--menu-shadow` | `0 10px 28px rgb(0 0 0 / 0.3), 0 2px 8px rgb(0 0 0 / 0.18)` | `0 10px 28px rgb(0 0 0 / 0.3), 0 2px 8px rgb(0 0 0 / 0.18)` |
| `--background` | `var(--page-canvas)` | `var(--page-canvas)` |
| `--foreground` | `oklch(0.985 0 0)` | `oklch(0.985 0 0)` |
| `--card` | `var(--surface)` | `var(--surface)` |
| `--card-foreground` | `oklch(0.985 0 0)` | `oklch(0.985 0 0)` |
| `--popover` | `var(--surface-raised)` | `var(--surface)` |
| `--popover-foreground` | `oklch(0.985 0 0)` | `oklch(0.985 0 0)` |
| `--primary` | `oklch(0.92 0.004 286.32)` | `oklch(0.669 0.181 252)` |
| `--primary-foreground` | `oklch(0.21 0.006 285.885)` | `oklch(0.227 0.004 286)` |
| `--secondary` | `oklch(0.274 0.006 286.033)` | `oklch(0.294 0.004 286)` |
| `--secondary-foreground` | `oklch(0.985 0 0)` | `oklch(0.985 0 0)` |
| `--muted` | `oklch(0.274 0.006 286.033)` | `oklch(0.294 0.004 286)` |
| `--muted-foreground` | `oklch(0.705 0.015 286.067)` | `oklch(0.68 0.008 286)` |
| `--faint-foreground` | `oklch(0.60 0.015 286.067)` | `oklch(0.60 0.008 286)` |
| `--accent` | `oklch(0.274 0.006 286.033)` | `oklch(0.294 0.004 286)` |
| `--accent-foreground` | `oklch(0.985 0 0)` | `oklch(0.985 0 0)` |
| `--destructive` | `oklch(0.704 0.191 22.216)` | `oklch(0.75 0.191 22.216)` |
| `--border` | `oklch(1 0 0 / 6%)` | `oklch(0.36 0.007 286)` |
| `--input` | `oklch(1 0 0 / 15%)` | `oklch(0.36 0.007 286)` |
| `--ring` | `oklch(0.552 0.016 285.938)` | `oklch(0.669 0.181 252)` |
| `--chart-1` | `oklch(0.72 0.16 255)` | `oklch(0.72 0.16 255)` |
| `--chart-2` | `oklch(0.62 0.13 255)` | `oklch(0.68 0.13 255)` |
| `--chart-3` | `oklch(0.52 0.10 255)` | `oklch(0.64 0.10 255)` |
| `--chart-4` | `oklch(0.42 0.06 255)` | `oklch(0.61 0.06 255)` |
| `--chart-5` | `oklch(0.32 0.03 255)` | `oklch(0.58 0.03 255)` |
| `--sidebar` | `oklch(0.21 0.006 285.885)` | `oklch(0.227 0.004 286)` |
| `--sidebar-foreground` | `oklch(0.985 0 0)` | `oklch(0.985 0 0)` |
| `--sidebar-primary` | `oklch(0.488 0.243 264.376)` | `oklch(0.669 0.181 252)` |
| `--sidebar-primary-foreground` | `oklch(0.985 0 0)` | `oklch(0.227 0.004 286)` |
| `--sidebar-accent` | `oklch(0.274 0.006 286.033)` | `oklch(0.273 0.031 252)` |
| `--sidebar-accent-foreground` | `oklch(0.985 0 0)` | `oklch(0.669 0.181 252)` |
| `--sidebar-border` | `oklch(1 0 0 / 10%)` | `oklch(0.36 0.007 286)` |
| `--sidebar-ring` | `oklch(0.552 0.016 285.938)` | `oklch(0.669 0.181 252)` |
| `--brand` | `oklch(0.65 0.16 255)` | `oklch(0.669 0.181 252)` |
| `--brand-foreground` | `oklch(0.985 0 0)` | `oklch(0.227 0.004 286)` |
| `--success` | `oklch(0.65 0.15 145)` | `oklch(0.70 0.15 145)` |
| `--warning` | `oklch(0.70 0.16 85)` | `oklch(0.75 0.16 85)` |
| `--info` | `oklch(0.65 0.18 250)` | `oklch(0.71 0.18 250)` |

## 对比度

WCAG 2.x 相对亮度，按 8-bit sRGB 计算；半透明选中底先与真实底面合成。新增独立测试 `apps/web/app/uworkers-theme.test.ts`，原来的对比度测试及所有现有测试均未改动。

| 角色 | 浅色白底 | 浅色画布 | 深色内容面 | 深色黑底 | 深色 raised | 浅色/深色最差内容底 |
|---|---:|---:|---:|---:|---:|---:|
| `--foreground` | 16.83 | 15.46 | 16.30 | 20.12 | 13.35 | 13.19 / 13.35 |
| `--muted-foreground` | 5.79 | 5.32 | 5.93 | 7.31 | 4.85 | 4.54 / 4.85 |
| `--primary` | 5.97 | 5.49 | 5.65 | 6.97 | 4.63 | 4.68 / 4.63 |
| `--brand` | 5.97 | 5.49 | 5.65 | 6.97 | 4.63 | 4.68 / 4.63 |
| `--success` | 5.83 | 5.35 | 6.73 | 8.31 | 5.51 | 4.57 / 5.51 |
| `--warning` | 6.30 | 5.79 | 7.58 | 9.35 | 6.21 | 4.94 / 6.21 |
| `--info` | 6.11 | 5.62 | 6.51 | 8.03 | 5.33 | 4.79 / 5.33 |
| `--destructive` | 5.98 | 5.49 | 6.45 | 7.96 | 5.28 | 4.69 / 5.28 |
| `--chart-1` | 7.22 | 6.63 | 6.74 | 8.32 | 5.52 | 5.66 / 5.52 |
| `--chart-2` | 6.16 | 5.66 | 5.88 | 7.25 | 4.81 | 4.83 / 4.81 |
| `--chart-3` | 5.30 | 4.87 | 5.08 | 6.27 | 4.16 | 4.15 / 4.16 |
| `--chart-4` | 4.54 | 4.17 | 4.49 | 5.54 | 3.68 | 3.56 / 3.68 |
| `--chart-5` | 3.96 | 3.64 | 4.00 | 4.94 | 3.28 | 3.11 / 3.28 |

链接与主色/brand 共用蓝色。主按钮文字与填色分别为浅色 5.97:1、深色 5.65:1。

| 状态文字 / 10% 同色底，取最差背景 | 浅色 | 深色 |
|---|---:|---:|
| `--success` | 4.64 | 4.66 |
| `--warning` | 5.03 | 5.19 |
| `--info` | 4.86 | 4.57 |
| `--destructive` | 4.60 | 4.55 |

## 暂留的写死颜色

这批不改 TSX，也不通过全局重定义 Tailwind 色板来间接改变用户数据或品牌色。以下界面反馈在新底色上仍沿用旧的黄/绿/蓝，可在下一批单独评估：

- `agents/components/agent-detail-page.tsx`：未绑定运行时的 amber 警告横幅。
- `chat/components/offline-banner.tsx`：离线横幅的 amber 背景、边框和文字。
- `issues/components/source-context-viewer.tsx`、`source-context-comment-list.tsx`：来源变化的 amber 底色。
- `issues/components/pull-request-list.tsx`：PR 状态、检查结果与增删计数的 emerald/amber/blue。
- `skills/components/runtime-local-skill-import-panel.tsx`：导入结果统计的绿/红底色。

以上路径相对于 `packages/views/`。标签色、取色器、供应商品牌色和搜索命中高亮也原样保留。

## 验证与截图

PR 的 How to Test 记录本次最终命令和结果；遇到现有测试失败会保留原样并报告，不调整它们来适配主题。

同一隔离数据库、浏览器、1440×900 视口与 100% 缩放，采集 main/主题的看板、任务详情顶部、评论区、收件箱、智能体列表、登录页，各含浅色与深色，共 24 张原始 PNG。ZIP 附在 CLAWSTA-134 的最终回复，附离线对照页、采集脚本、源码校验、计算样式与验证记录。

主题关闭的看板另作本地开关验证，不收入 PR 截图。关闭时同时核对根变量、阅读/控件计算样式和所有既有运行时源码与 main 的一致性。
