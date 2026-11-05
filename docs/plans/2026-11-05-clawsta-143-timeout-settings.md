# CLAWSTA-143 R1 服务端超时配置 Implementation Plan

**Goal:** 在现有网页设置中管理智能体运行超时，由服务端持久保存、随任务下发并由客户端执行，免去逐台修改客户端环境变量。

**Architecture:** 工作区默认值与智能体覆盖值由服务端校验、合成每轮任务配置；客户端只为该轮采用合成结果，不写本机配置、不修改共享 daemon 配置。无服务端策略时保留既有行为；显式服务端策略不得被旧的本地 OpenCode 十分钟限制悄悄压回去。

**Tech Stack:** Go/Chi/sqlc/PostgreSQL；共享 React views、TanStack Query、Zod；现有 Windows Client-Kit 升级渠道。

维护者：后端架构师。执行者不得改写本任务书；发现契约问题在 CLAWSTA-143 回报，由维护者修订。按现有 executing-plans 工作流分步实施；此任务已获用户开发授权，无需再询问是否开始。

## 授权、基线与交付

- 用户明确要求：超时放到界面，由服务端配置，避免每台客户端自己改。已明确不要在控件旁写“下次运行生效”。
- 本批 R1：完整代码候选、必要测试、可审查 PR；随后后端验收与前端专业评审。专业通过后由现有协调负责人承接发布与用户验收，不把候选当已上线。
- 开始时远端 main = `03f57f4cf5292ef4c190a1ef96eccda50d344ab5`，tag v0.2.13。执行时重新核对最新 main，已有他人改动不得覆盖。
- 仓库 `https://github.com/AndyTane/UWorkers-nice.git`。执行者使用自己的专用 checkout/branch，从 main 开始。本文可由附件获取，或从本次后端工作目录读取后复制入自己的 docs/plans；规范由后端维护。
- 按仓库规范提交候选、推送专用分支并开 PR，标题含 CLAWSTA-143。不得自行合并、打 tag、上传 OSS、切 stable、替换正在运行的二进制或停止用户任务。升版文件按当前规则同步；正式版本号由最新已用版本核对后确定，不复用 v0.2.13，不把旧 package.json 的 0.2.11 当发布基线。
- 不做移动端新页面、心跳/掉线回收重构、运行时协议全面重写、后台编译脱离生命周期。本批无需真实付费模型调用或真实四十分钟业务编译；必要验证见下。

## 方案取舍

选择“随任务下发策略”。只把变量填进网页但不接 daemon 无法满足要求；服务端远程写客户端系统环境会污染不同工作区；实时热改所有在途计时器会引入编译中途终止风险，超出本次需要。每轮开始时固定策略，下一轮自动采用最新值，不需要用户重启客户端。这里只描述实现语义，界面不显示用户禁止的提示文案。

## 数据与优先级契约

优先复用已有 `workspace.settings.execution_timeouts` 与 `agent.runtime_config.execution_timeouts` JSON 容器，不新增通用配置服务。字段统一秒为单位：

| 字段 | 含义 | 0 的含义 |
| --- | --- | --- |
| `idle_seconds` | 通用无消息等待 | 关闭平台这组无消息监测，沿用既有语义 |
| `tool_idle_seconds` | 已识别工具执行期间无消息等待 | 工具执行期间不因静默终止 |
| `run_seconds` | 整轮总时长上限 | 不限 |
| `opencode_idle_seconds` | OpenCode/既有共用适配器的专用无消息等待 | 跟随通用值，不是关闭全部监测 |

- 对未配置的旧数据，保留现有 2h / 2h / 0 / 10m 及本机显式覆盖，不通过迁移静默改变所有人的预算。界面不能把“尚未设服务端策略”显示成“已由服务端控制”。
- 启用工作区策略时提交完整有效对象；智能体默认“使用工作区默认值”，关闭继承时保存完整覆盖对象，恢复继承删除该键。每轮优先级：智能体显式策略 > 工作区策略 > 原客户端配置。JSON key 缺失与数值 0 必须区分。服务器合成策略必须是完整对象，不让老的本地 env/启动参数再次收紧其中任何字段。
- 专用正值仍取通用值与专用值中较小者；UI提供“跟随通用”选项映射 0。因此用户可明确选择 OpenCode 跟随两小时，不再受旧十分钟约束。不得未经用户选择自动放宽现有部署。
- 只接受可安全转换为 Go duration 的非负整数秒；拒绝负数、小数、字符串、溢出及错误对象形状，错误定位到字段。不要创建和需求无关的时长上限。
- 更新仅本组键，不丢失 workspace settings 的 PR 等其他配置，也不丢失 agent runtime_config 的 compaction、gateway 等配置。不能只靠前端 spread 防覆盖；用现有读改写/局部更新模式处理，测试未包含此键的旧客户端更新仍保留已设置策略。
- 沿用工作区管理员与智能体编辑权限；任务 token 或无权成员不能借新字段越权改策略。沿用空间隔离、事件通知、Query cache 失效。
- claim 单任务与 batch 共用同一解析路径，附带有效策略与来源。daemon 只在执行此任务时构造配置副本/ExecOptions，禁止改 d.cfg 导致并发任务串值。失败原因和实际预算日志必须使用该轮配置。
- 运行中的任务继续使用启动快照；修改后无需重启，后续任务自动拿到新值。重试/续跑按一次新执行取值；接口错误不能静默降级成本地短预算继续执行。
- 复用显式客户端 capability（如 execution-timeouts-v1，最终命名可遵循现有规范）识别支持方。新服务端+旧客户端不宣称已应用策略；对明确要求策略的运行，须给出可识别的客户端升级状态，不能领取后忽略。旧服务端+新客户端保持旧行为。不要通过伪造版本号绕过能力检查。

## 引擎自身工具超时

平台 `tool_idle_seconds` 是静默窗口，绝不能标为“工具最长执行时间”。OpenCode 自己的 shell timeout 属于另一层。

本批同时提供 OpenCode 的可选“工具默认执行时长”设置，存入同组策略的独立可选字段 `opencode_shell_seconds`，正整数秒、未设置沿用引擎原值。通过该轮子进程环境映射 `OPENCODE_EXPERIMENTAL_BASH_DEFAULT_TIMEOUT_MS`，校验乘 1000 不溢出；不写用户系统环境，不输出其他 env，不污染其他 runtime。明确这是默认值，工具调用显式 timeout 仍优先；不要声称能强行覆盖引擎所有调用。

该字段只在 OpenCode 能力适用处展示；DSH/Codex 不展示假的统一工具总时长支持。DSH 本批覆盖上述平台策略；Codex 需要沿用已有 idle/tool 与 semantic inactivity 推导关系，避免界面放宽后内部旧较短值仍提前终止；握手、模型网络请求及其他非同义超时不改。本批不修改外部 DSH/OpenCode 项目。

## 界面契约

- 工作区设置 → 常规：新增“运行超时”区域。
- 智能体详情 → 设置 → 通用：新增同名区域，默认继承，可单独覆盖。继续复用现有 SettingsSection/SettingsRow/保存状态及权限表现，不新增一级导航。
- 普通字段白话：“无消息等待”“工具执行时无消息等待”“整轮运行上限”；OpenCode 专用等待与工具默认执行时长归入适用的扩展项，不把运行时名字写进所有智能体通用表单。
- 单位用分钟/小时，保存转换为整秒；0 通过“不限/关闭/跟随通用”等对应语义控件表达，勿让同一个“0”产生歧义。显示继承来源和实际有效值。
- 不在控件旁写“下次运行生效”，也不换成同义冗余提示。旧客户端需要升级是实际未生效状态，应准确反馈，不能显示成保存即已应用。
- 保留语言资源与无障碍标签，复用当前 UWorkers 风格。由前端负责人只审本批界面、交互和数据契约；无需新增一轮通用技术评审。

## 实施路径

### 1. 服务端持久化、校验和读取

检查/修改 `server/internal/handler/workspace.go`、`agent.go`，新增共享 typed policy helper（建议 `server/pkg/agent/execution_timeouts.go`，或现有无依赖协议包）；沿用 `server/pkg/db/queries/workspace.sql`、`agent.sql` 的局部 JSON 更新模式，确需修改 SQL 时运行 sqlc。新增字段级校验、权限/空间隔离与保留其他设置的测试。

### 2. 下发和客户端执行

检查/修改 `server/internal/handler/daemon.go`、`server/internal/daemon/types.go`、`daemon.go`、`client.go` 与已有 capability 定义位置；补齐单/batch claim、实际 exec 配置、watchdog、总超时及诊断的同源取值。`server/pkg/agent/opencode.go`、`agent.go` 中处理运行时适配，保持 DSH `tool_call`/`tool_result` 路径。

### 3. API 类型与界面

检查/修改 `packages/core/types/workspace.ts`、`packages/core/agents/types.ts`、`packages/core/api/schema.ts`/相关 schemas、workspace/agent mutations；实现共用时长编辑组件，接入 `packages/views/settings/components/workspace-tab.tsx` 和 `packages/views/agents/components/agent-detail-inspector.tsx`（当前 general 页），更新各语言资源。服务端 JSON 用 schema 解析，未知/缺失字段兼容。

### 4. 必要验证

先补有意义回归用例，再实现，最后执行仓库要求的相关检查。必须覆盖：

1. 保存、重新读取、继承、清除覆盖、零值和非法输入；无关 JSON 键保留、旧客户端局部更新不抹掉策略；越权拒绝、不同工作区不串数据。
2. 单/batch claim 均下发相同合成策略；两个并发任务各用各的值、不修改 d.cfg；无策略继续兼容。
3. 明确回归：客户端本地 OpenCode 短窗口 + 服务端较长窗口，模拟 OpenCode 工具结束才上报，任务在旧短窗口之后仍完成；也验证超过新的有效窗口确实终止，不能只是禁用所有 watchdog。
4. DSH 有工具开始/结束事件时使用正确窗口；持续消息重置静默计时；整轮硬上限仍会按配置生效；Codex 已有 semantic timer 不偷偷压回旧限制。
5. OpenCode 子进程收到配置映射的工具默认值，别的并发 runtime/env 不受影响；显式工具 timeout 优先的限制如实说明。
6. 新旧服务端/客户端能力组合；“支持配置”的信息与真实执行一致。保存策略后第二轮运行自动采用新值，daemon 无需重启；在途轮不被热改。
7. 真实临时子进程可用缩短时长验证“越过旧窗口仍完成”和“到新窗口终止”，只能使用隔离测试进程，不停止用户 daemon/业务编译。区分此项与真实四十分钟业务编译，不伪称真实长编译已验收。
8. 界面保存与继承链路、错误状态、权限只读、有效来源、OpenCode 扩展显隐；截图核对没有被禁止的提示。测试放在权威层，避免重复镜像测试。

迭代命令：`go test ./internal/handler ./internal/daemon ./pkg/agent`（server目录，按用例先用 -run 缩小）；前端 targeted vitest 使用现有 package scripts。最终按 AGENTS 运行 `make test`、`pnpm typecheck`、`pnpm lint`、`pnpm test` 的必要检查，环境限制如实记录。SQL变更 `make sqlc`；bridge变更才需 `pnpm test:dsh-web`。禁止未经授权真实模型 smoke。GitHub CI 不等待；已知额度限制不是重复轮询理由。

## 回传与阶段出口

完成本批后在 CLAWSTA-143 回复并有效 @ 后端架构师：`68ca5d11-c32c-453c-82bf-ec44dbcf66a8`。报告批号 R1、base..head、PR URL、修改/关闭问题、未关闭阻塞、执行检查与真实结果、证据/截图路径、版本处理。不得只给计划或预检结果。

本批出口：代码候选交付并能证明服务端设置确实改变客户端行为；由后端架构师接收验收，再投递前端专业审查。正式发布/公司落地仍由协调负责人按既有升号与三件套流程接续，不在实施阶段宣称上线完成。

## R1.1 集中修复与专业复审（2026-11-05）

审查对象：PR #45，`03f57f4cf5292ef4c190a1ef96eccda50d344ab5..736aa6b0130b99c16931788d2194c23294730c56`。后端审查结论：需要修改。原阶段目标和授权不变，沿用同一 PR，禁止自行合并或发布。

### B1：保持既有运行配置清除语义

- 新 `server/pkg/db/queries/agent.sql` 将所有 runtime_config 写入从替换改成顶层 JSON 合并。既有 `packages/views/agents/components/tabs/runtime-config-tab.tsx` 和 `packages/core/agents/openclaw-runtime-config.ts` 在网关字段全部清空时发送 `{mode:"gateway"}`，不带 gateway；原实现清除原网关，新实现保留原 gateway。`server/internal/daemon/openclaw_runtime_config.go` 在 gateway 模式继续使用该旧端点和旧 token。这是代码链路已确认的兼容回归，不能作为原有问题放行。
- 修复须同时满足：新超时局部写入不丢其他配置，旧客户端不带 execution_timeouts 时不抹掉策略，原有配置编辑/清空行为仍保持。不得通过要求旧客户端改发 gateway:null 来回避兼容问题，也不能把所有自由 JSON 键不加区分地永久保留。选择最小明确的更新契约，若需改变请求形状同步现有调用者和测试。
- 在隔离数据库增加实际 handler/SQL 回归：先保存虚构 gateway 及 execution_timeouts；按既有 UI 清空后载荷更新，重新读回并验证 gateway 已删除、超时策略仍在；再验证仅保存超时不会改 gateway/context_compaction 等其他键。使用虚构凭据，输出不包含真实 env/token。
- 相同 SQL 合并也改动了 workspace.settings；检查现有 PR 设置写入/删除语义，仅修本批引入的冲突，不重构其他配置系统。

### 验证范围与前端评审

- 后端负责人已独立运行新 daemon/agent 的 `^TestExecutionTimeout` 专项并通过。继续保留这些结果；修复后复跑新增持久化回归、R1 handler 专项和受改动影响的测试，SQL 变化重新生成 sqlc。整仓旧失败日志保留，不能把无对照的失败笼统标成全部基线问题，也不要求为本批重写所有 Windows 不兼容测试。
- 前端负责人仅审 PR #45 的界面、交互和前端数据契约，以本任务书的入口、来源/有效值、继承/保存/错误/只读、OpenCode 显隐、语言及无障碍要求为准。查看附件真实渲染，并检查适用的主题、窄屏和键盘行为；明确哪些亲测、哪些审阅他人证据。不重复后端内部技术验收。
- 前端现有整仓失败中若涉及本批入口/组件，核对冻结后的实际回归结果；无关 worker/Windows 环境失败按影响归类，不把未实测的基线声明当证据。发现阻塞一次集中列清，回复后端负责人，由后端负责人并入本批修复，不另开重复实现委派。
- 高级开发执行专家负责 B1 修复；前端负责人本次只评审、不与其并行修改同一分支。两者均在本 issue 回复并有效 @ 后端架构师。回传写明审查/修复的精确 head、关闭项、剩余阻塞、实际检查与证据。后续复审只查本批新增差异及未关闭项。
