# Web registration and user administration implementation plan

**P5 正式任务：网页邀请注册、密码设置和实例用户管理。范围已授权，P4 已接回验收。**

负责人/验收：后端架构师；执行：DSH；新增界面与契约由前端负责人配合复审。本稿替换同路径 P4，P4 历史附件保留。

## 用户目标与边界

Tane 2026-10-31 13:10 已选择：网页注册先邀请制，管理员可添加/删除用户，保留开放注册开关。默认关闭开放注册。密码在 HTTPS 网页设置；日常注册、邀请、用户管理、修改密码无需桌面脚本。注册独立 PR，验收后已授权部署和合并，无需再问是否执行。

P4 任务 01a19058-c2d6-7464-a8f3-4f875bf79b46 已完成，后端已独立确认只余明确Tane、uworkers-tane及owner成员各1，本地凭据0，审阅清理前后证据。PR6已合并、PR5已关闭；本批基线 main 8c61cdef31aa86cffa616f315540e58fbb82d625。开工重新读取main，若有新变化对齐差异；不重复清理或继续修改已合并PR6。

目标仍为 ssh desktop 的 D:/UWorkers/clawsta80/native，当前 app 065e20ad0。禁止修改承载智能体的 Multica/DSH 实例。保留 Tane 正式身份及 uworkers-tane、其内容/权限/密码，固定码和邮箱验证码关闭，SMTP暂停，企业登录只摸底，不接运行时/启动付费智能体。不重开已验收 HTTPS/防火墙事项。

## 已检查的代码与入口

- server/internal/handler/auth.go：CreateUser、issueJWT、checkSignupAllowed、GetMe/UpdateMe；邮箱/Google 有自动建号与 ALLOW_SIGNUP 规则。
- server/internal/middleware/auth.go：JWT 验签后直接使用 sub，不能假定删除 user 后旧 JWT 自然失效。
- server/internal/handler/invitation.go：现有邀请是工作区成员邀请，要求已登录，不能直接代替账号注册授权。
- server/pkg/db/queries/user.sql：尚无实例管理员/删除用户管理接口；工作区 owner 不是实例管理员。
- server/cmd/server/router.go、server/internal/auth、server/pkg/db/queries/workspace_delete.sql、P1 local-auth 工具与内存限流：复用既有机制。
- packages/views 共享登录/设置页、packages/core/api/schema.ts 和 auth store；Next 路由仅 apps/web，按仓库规则接好桌面导航。

先读 AGENTS.md、UI/文案规范；遵循无FK、每个CONCURRENTLY索引单独迁移，sqlc生成，不手写。账号级新表列 workspace 删除计划 KEEP；新增接口/schema兼容旧客户端。以下为实现契约，具体文件拆分由 DSH 按真实基线落实，不要求额外框架。

## 技术决定

### 实例管理员

增加独立实例管理员权限和服务端 guard。只把目标部署明确的 Tane UUID 初始化为管理员：规范邮箱 trim+lower UTF8 SHA256 = 5983f9551dc3f2370d8f04a7291d1d03e1c7fb96eac1b5112791572728122788，且 pinned UUID/DB邮箱逐项一致。不得按显示名、旧验证码、首个访问者、任意工作区 owner 授权。

普通注册和邀请账户默认普通用户，不能经请求或 UpdateMe 自设管理员。/api/me 返回只读管理能力供UI显示，管理API独立校验真实用户与权限，不接纳 mat_ 任务令牌冒充人。管理员管理账号不等于取得其他工作区内容权限。本批不做角色编辑器；最后管理员不能删除。

### 网页注册、初设与邀请

登录页明确「注册 / 设置密码」入口；邀请模式可粘贴邀请或从链接进入，填写显示名、登录名、密码/确认；绑定身份不可换。成功复用现有登录收尾和正常 onboarding，不直接改 onboarded_at，不自动加入已有工作区。

管理员网页添加待激活用户，生成/撤销/重新生成邀请。邀请绑定 user UUID；Tane初设绑定已有UUID，不重复建号。不能凭邮箱或名字认领已有身份；已有密码不能被初设邀请覆盖。若实现恢复入口，必须独立显式授权并撤销旧会话，不暗用初设覆盖密码；无SMTP不得提供仅填邮箱重置密码。

邀请 crypto/rand 至少256bit，DB只存摘要，有效24小时；撤销/新发使旧邀请失效。凭据写入和邀请消费同事务，条件更新保证并发只能一次；非法输入/唯一性冲突不消耗邀请。首次仅建新凭据。

链接秘密放 fragment，页面初始化立即从地址栏移除，仅存内存、经POST body兑换；不进入 query/path、Referer、storage、分析上报、访问日志、截图、报告或评论。响应NoStore，设置页适当Referrer-Policy；不记录请求体。只在管理员主动生成后的私有页面一次显示，列表不返回原值。

Tane 尚无正式密码时，一次性本机引导沿用桌面快捷方式但改为打开网页：受限宿主辅助程序核对身份后，在内存生成15分钟初设邀请并直接打开HTTPS设置页，或本机私有窗口提供复制按钮。辅助程序不询问/打印/保存密码，不向终端输出邀请，不把邀请写入脚本/文件；有密码则只打开登录页、不重置。邀请在页面立即移除fragment，辅助程序不得留下秘密日志/文件。不开放LAN引导端口，不采用“首人即管理员”，不增加常驻服务。之后日常管理都在网页完成。

### 开放注册开关

实例「用户管理」提供持久保存、重启后保留的「开放注册」开关，默认关闭，只允许管理员修改。公开config仅返回必要能力，不泄露名单。关闭后无邀请注册在服务端拒绝；开启后可自行注册普通身份/密码并正常onboarding，不能覆盖已有邮箱/用户名或获管理权/成员权。

本地密码注册由此策略控制，既有workspace invitation或邮箱白名单不能绕过。明确与原 ALLOW_SIGNUP 的关系；其他建号路径在本部署继续关闭或遵守策略，不无意改变其他部署行为。自填未验证邮箱不能成为合并身份、找回密码或加入工作区的依据，未来企业SSO需独立验证后映射。

### 修改密码与撤销

个人设置用当前密码验证后输入新密码/确认，沿用 bcrypt cost12和P1长度规则，不静默截断。本批不扩展登录名修改。成功后旧密码和旧会话失效，明确引导重登。

可用账号 auth_version + JWT版本声明实现；旧令牌缺省值按迁移兼容版本处理，滑动续签/CLI令牌必须检查并保留版本，不能复活旧会话。删除用户后旧JWT/PAT、鉴权缓存、活动WS不得继续读写受保护数据。检查实际认证入口实现必要撤销/失效，不能只删除密码行或更换全实例JWTsecret。撤销状态重启后有效，不影响其他用户。

### 管理员添加/删除

管理页显示必要用户信息及管理员/待激活状态，提供添加、邀请/撤销、删除。添加不替用户设置明文密码，由本人网页激活。

删除展示明确目标/影响并二次确认；服务端阻止自删、删除最后管理员（含并发），拒绝普通用户/伪造owner操作。只按精确UUID处理。不暗中删除共享工作区/他人数据：目标为唯一owner时给出可操作依赖，先用现有工作区转移/删除流程处理；有运行任务或需处置运行时归属也明确拒绝，不杀进程。

满足条件后事务清理身份、凭据、邀请、成员、个人访问令牌和账号绑定，按真实schema枚举依赖；他人业务内容/历史归属依既有“已删除用户”展示惯例保留。P4测试数据清理不是日常删除的无界级联模板。仅目标独占文件可依现有存储机制清理，路径边界精确校验、失败可重试，不能跨用户删除。

### 请求边界

邀请兑换/注册/当前密码验证在bcrypt前限流，复用P1来源识别和无Redis后备，保留容量上限和过期清理。注册来源5次/分钟，失败兑换来源20次/分钟，当前密码验证账号5次/分钟，429+Retry-After。不能信任客户端伪造XFF。限制body大小，严格校验用户名/邮箱/密码、数据库唯一性，稳定错误码供多语言映射，不输出秘密或用户枚举细节。

cookie管理写操作复用CSRF；未登录注册/兑换验证精确可信Origin并JSON-only，拒绝跨站。全程HTTPS正常证书校验。

## 实施与验证

按四组集中实现并提交，不把每项测试另拆委派：

1. 数据/权限/sqlc：管理员、注册策略、邀请摘要/消费、会话撤销；测试普通用户及工作区owner无管理权、不能自授管理员、最后管理员/并发删除保护。
2. 服务端：真实PG测试邀请一次性并发消费、过期/撤销、重复邮箱/用户名不覆盖、失败不消耗、开关服务端拒绝、删除依赖/令牌失效；覆盖旧cookie续签、CLI/PAT/WS受影响路径，不重跑无关历史全套。
3. 网页：共享登录/注册/个人密码/管理页，现有5语言一致。配置加载前不闪邮箱表单，错误本地化，失败清空密码保留非秘密输入，键盘/autocomplete/确认/pending正常；开关保存失败不能伪装成功。补schema与核心组件测试，pnpm typecheck/lint。
4. 隔离真实浏览器：管理员添加邀请→用户网页设置→正常onboarding→普通用户无管理权→改密旧会话失效→新密码重登→删除用户后旧会话拒绝；开启开放可注册、关闭后直接API拒绝，最终默认关闭。Go build/vet、受影响包及真实DB测试，新失败保留证据。

DSH交独立候选PR、base..head、任务书及 evidence/clawsta-77/p5/；截图不含邮箱、登录名、密码、邀请或会话。完成后当前issue有效@后端架构师。前端负责人复审新增界面/契约，修正执行继续DSH，不增加通用评审层。

## 上线与出口

P5准备/代码批次先不部署。代码与界面验收后明确接续：DSH走已有native Build/备份/迁移/切换流程部署，实际HTTPS浏览器验证，再按已授权合并注册PR，记录真实appSHA/release与mergeSHA。保留回滚release及受限备份。

实际验证用专用临时用户，完成后删除新增测试账号/工作区/凭据，保留Tane正式数据。不得覆盖Tane已设密码；若生产管理员验证需要临时凭据，必须确认尚无正式凭据且用完清除。正式密码本人网页输入，不能把临时验证称为正式交接。

清理/合并已验收，新网页功能仍须实现、上线验证后统一交付。当前用户选择已确定，不再以等待注册范围为阻塞。P5交付后有效@后端架构师接回；父issue保持in_progress。

## P5-R1 集中修复执行结果（候选）

接续候选 `46814feec`，本轮集中在 PR #7 完成专业复审要求的修复（范围 `46814feec..16db41e34`，PR 总 base `8c61cdef`）。仍然只是候选：未部署、未合并、未重启 HTTPS，未操作承载智能体的 Multica/DSH 实例。逐项证据见 `evidence/clawsta-77/p5-r1/README.md`。

### 服务端

- **会话权限统一**：WebSocket 握手对 header、cookie、首帧三种来源都做活用户 + `auth_version` 校验；改密/删号会关闭该账号的已连接 socket，另有 60s 复检兜底；其他用户不受影响。PAT 缓存命中时检查"账号已删除"标记，删号在该标记写入成功前不会提交，缓存不能复活已删账号。
- **邮箱验证作为身份边界**：`user.email_verified` 迁移默认 true 保留旧已验证身份；开放注册建号即未验证。邮箱匹配的工作区邀请（列表、接受、拒绝、分享链接结算）只对已验证账号生效；UUID 绑定的邀请不受影响。邮箱验证码/Google 建号仍按定义已验证。
- **一致性与锁序**：管理员删除、邀请重发/撤销/兑换、改密共用同一事务咨询锁（先 `instance_admins` 后 `user:<uuid>`）。事务内重读调用者、目标、管理员数量与依赖；删除清理后再跑一次依赖清单，发现并发新增即回滚，不留孤儿。重发在锁内先撤销后签发，只有最新邀请有效；兑换条件消费一次；改密用条件更新，旧密码只能成功一次，旧密码登录与改密竞争不会拿到新版本 JWT。
- **管理员 API**：目标 id 非 UUID 稳定返回 400。删除依赖按真实 schema 枚举：工作区/运行时/智能体归属、渠道/Lark 安装、分享链接、来源上下文为阻断项；身份绑定与账号级行删除；共享历史保留，可空归属置空（含 `agent.archived_by`）。新增 `GET /api/admin/users/{id}/delete-preview`，返回 `{can_delete, blockers:[{code,count}], removes:[code]}`，与 DELETE 使用同一判定函数，仅活实例管理员可读，拒绝任务令牌/PAT。
- **账户级新表**：`registration_invite`、`instance_setting` 归入 workspace 删除清单的 KEEP，真实 DB 清单测试通过。
- **本地引导**：`local-auth invite` 默认不再打印邀请明文，只有显式 `--stdout-token` 才输出；新增候选 `bootstrap-host`：明确 UUID + 邮箱指纹、15 分钟、已有密码只打开登录页、私下打开 HTTPS 设置页，明文只存在于内存与浏览器地址栏。实际安装留部署批次。

### 前端

实例用户管理迁入设置页的管理员专属「用户与注册」tab（网页/桌面共用），`/admin` 直接移除（该路由从未发布，设置页本身是工作区域内路由）；account-tab 的硬跳按钮删除。注册页等 config 加载后再判模式，邀请模式始终可粘贴完整链接或裸邀请码，fragment 在挂载后读取并立即清除；表单 Enter 提交、密码规则常驻、错误 aria 与焦点、失败只清两个密码框；成功由宿主注入的 `onSuccess` 复用登录收尾。开放注册用 Switch，失败回服务器状态并提示。邀请面板显示只读完整 HTTPS 链接，复制失败提示并自动选中。删除用预览驱动的 AlertDialog，预览失败/畸形/有阻断项时禁止确认。改密失败清三个字段并聚焦，成功后退出并在登录页显示一次性本地化提示。新 API 全部 zod schema + `parseWithFallback`（删除预览失败即关闭），错误码集中映射到 i18n，五语言补齐并有 parity 测试，`register` 加入保留 slug。

### 验证

- Go：`go build ./...`、`go vet ./...` 通过；`TestP5`、清单测试、`internal/realtime`、`internal/auth`、`cmd/local-auth` 全通过（真实 PostgreSQL，隔离库 `multica_p5candidate`）。完整 `internal/handler` 包在本 Windows 环境有 9 个与本批无关的既有失败（Windows 路径分隔符、运行时离线的调度断言）并触发 10 分钟超时，已如实记录，未称为通过。
- 前端：core/views/web/desktop typecheck 通过；core 173 文件 2224 测试、web 25 文件 245 测试、desktop 62 文件 652 测试通过（`scripts/package.test.mjs` 为既有失败）；新增/受影响 views 测试与五语言 parity 233 项通过；lint 0 error。完整 views 套件在本机并行下有 22 个无关既有失败，单跑均通过，未称为通过。
- 真实浏览器：隔离 staging（Caddy `tls internal`、CA 装入当前用户根存储、正常证书校验）28/28 步通过，含从登录页点击注册→粘贴管理员复制的完整链接→纯键盘完成、fragment 直开、开放注册模式、config 不闪、开关失败回滚、删除预览有/无依赖与自删保护、普通用户无管理数据且 API 403、改密失败清字段、成功提示、旧密码拒绝、新密码登录、既有账号回到自己工作区。截图（浅/深色 + 375px，均无秘密）在 `evidence/clawsta-77/p5-r1/browser/`。

### 未完成

- 不部署、不合并；native 备份/Build/迁移/HTTPS 切换、`bootstrap-host` 实际安装与真实宿主引导留到复审通过后的部署批次。
- 完整 Go handler 套件与完整 views 套件在本 Windows 环境因上述既有原因非全绿，合并前应在 CI/Linux 复跑。
- ja/ko/fr 文案为本批编写并通过 parity，仍建议母语校对。

## P5-R2 集中修复执行结果（候选）

接续 `953a8294f` 完成复审剩余项（范围 `953a8294f..证据提交`，PR #7 总 base `8c61cdef`）。仍只是候选：未部署、未合并、未改 HTTPS 任务，未操作承载 Multica/DSH 的实例，未触碰 Tane 身份与正式密码。逐项证据与截图索引见 `evidence/clawsta-77/p5-r2/README.md`。

### B1 邀请身份与实例注册策略

- `CreateInvitation` 只在邮箱账号已验证（`email_verified`）时才由邮箱推导 `invitee_user_id`；未验证账号不再成为 UUID 授权。反向次序已用真实 handler 覆盖：先开放注册他人邮箱，再向该邮箱发工作区邀请，进攻方既看不到也接受不了（邀请未绑定，接受 403）；已验证账号仍正常绑定并接受。
- `findOrCreateUser` 遇未验证同邮箱账号返回明确拒绝，不自动合并、不保留进攻方已设密码。
- 开放注册、邮箱验证码、Google 三个建号入口共用同一策略：部署开关、实例 `open_registration`（默认关）与邮箱/域名白名单；工作区邀请不再等于实例注册授权，策略在真正建号时重读。生产邮箱/Google 仍关闭，未新建验证服务。

### B2 删除真实性与账号写入一致性

- 删除依赖清单新增阻断项 `chat_history`（`chat_session.creator_id` 级联会带走会话与消息）和 `active_task`（queued/dispatched/running，按 accountable/initiator/originator 关联，同一任务只计一次）。preview 与 DELETE 共用判定，DELETE 在事务内重查；被拒后聊天、消息、活动任务与归属均不变。任务结束后可删，已完成任务保留并置空归属。
- `UpsertClientUsage` 改为单事务：账号锁 → 活账号复核 → 工作区锁/成员校验 → upsert；删号在扫描前取同一账号锁，因此不会在末次扫描后插入用量行。CLI 的凭据/邀请写入同样在锁后复核活账号；确定性锁竞争测试在写入排队时删除账号，两种次序都不留孤儿。
- 既有管理员互删、邀请重发/兑换、改密并发回归继续通过。

### B3 WS 握手与撤销的共同边界

- Hub 维护撤销计数与每账号最后撤销标记；握手在验票前取计数，注册在 `DisconnectUser` 打标记的同一把锁内复核并把连接加入集合。撤销后旧会话无法再入房，新登录（新版本）正常连接，其他用户不受影响。
- 复审附件复现用例已由真实 `HandleWebSocket`/Hub/网络跑通并保留为回归；另用真实改密与删号 handler + 隔离 PostgreSQL 验证：目标 socket 立即关闭，旁观连接继续收到工作区事件。

### U1–U4

- U1：开放注册改用 `SettingsRow`（「开放注册」+ 后果说明 + 控件位 Switch）；添加用户表单、邀请面板、账号行与改密卡片补内边距（浏览器实测 16/14、16/16）。
- U2：删除对话框取消按钮为通用「取消」，不再显示「完成」。
- U3：桌面登录页消费一次性改密标记并传给共享登录页，首次显示、再次挂载不再重复；新增桌面登录页测试。
- U4：`BLOCKER_CODES` 与五语言新增 `chat_history`（有聊天记录）与 `active_task`（有进行中的任务）；未知阻断码合并为「另有 N 项依赖需先处理」（N 为 count 之和）而不是丢弃；确认按钮仍由服务端 `can_delete` 约束；补充新码、未知回退与预览失败/畸形用例。

### 验证

- Go：`go build ./...`、`go vet ./...` 通过；realtime（含复审复现用例）、auth、P5/P5-R1/P5-R2、建号/Google/邮箱验证码、`cmd/local-auth` 在隔离库 `multica_p5candidate` 全部通过。完整 `internal/handler` 套件仍为本机既有失败与超时，已如实记录，未称为通过，也未新增 CI 门槛。
- 前端：core/views/web/desktop typecheck 通过；web 245 项、desktop 653 项（`scripts/package.test.mjs` 既有失败）、受影响 views 5 文件 289 项（含五语言 parity）通过；lint 0 error；web 候选构建通过。
- 真实浏览器：隔离 staging（候选 API 18082 + web 13003 + Caddy `tls internal` 13444，隔离库经 SSH 隧道）26/26 步通过：管理员登录、管理 tab 浅/深色与中文、改密卡片、聊天记录与进行中任务两种删除阻断（确认禁用、取消文案）、普通用户无管理入口且管理员 API 403、改密成功提示、旧密码拒绝、新密码登录。截图见 `evidence/clawsta-77/p5-r2/browser/`，均为合成 `@staging.invalid` 身份，无密码/邀请明文。
- 证书校验限制：本非交互会话无法把 staging CA 装入 Windows 根存储（导入接口报 UI 不允许、直接写根存储被保护机制删除、机器存储无提权），浏览器改用 `ignoreHTTPSErrors: false` + 仅针对该 staging 叶子证书 SPKI 的 Chromium 例外；另用普通 TLS 客户端信任 staging 根独立验证了证书链与 `localhost` 主机名。这是本批唯一的环境性偏离，已在证据中写明。

### 未完成

- 不部署、不合并；native 备份/Build/迁移/HTTPS 切换与正式引导仍留部署批次。
- 完整 Go handler 与完整 views 套件在本 Windows 环境因既有原因非全绿，合并前应在 CI/Linux 复跑。
- ja/ko/fr 仍建议母语校对；本次未新增翻译流程门槛。

## P5-R3 结果（allowlist 不得绕过关闭注册）

- 缺陷：`instanceSignupPolicy` 先匹配邮箱/域名白名单并直接放行，随后才看 `ALLOW_SIGNUP` 与实例 `open_registration`，导致配置白名单时关闭注册仍可自填匹配邮箱建号。
- 修复：统一策略顺序改为部署硬开关 → 实例持久开关（默认关）→ 白名单仅收窄范围；白名单不再能打开关闭的实例。公开注册、邮箱码、Google 等自助建号入口共用；既有登录与管理员邀请设密不经过该策略，保持可用。
- 测试：更新 `TestSignupGating` 语义；新增真实 handler 回归 `TestP5R3AllowlistCannotBypassClosedRegistration`（关闭+匹配→403 `registration_disabled` 且不建 user/凭据；开启+匹配→注册成功；开启+不匹配→403 `email_not_allowed`）与 `TestP5R3InvitationRedemptionStaysAvailable`。隔离 PG（multica_p5candidate，迁移到 582）26 项 handler 测试全通过，auth/middleware/realtime 通过，build/vet 通过。
- 生产只读核对：live `multica_clawsta80` release `v0.2.0-20261031111027` 未变；live schema 尚无 `instance_setting`（P5 未部署），本次无生产写入。
- 未做：合并/部署/HTTPS 切换属授权部署批次；完整 Go/views 套件仍需 CI/Linux 复跑；前端非阻塞建议留后续。
- 证据：`evidence/clawsta-77/p5-r3/`（README、candidate-evidence.json、handler-tests.log）。


## P6 上线、真实浏览器验收与合并

任务书 `2026-10-31-local-password-login.md` 的部署批次。基线 `e275b61a4`，PR #7，base `8c61cdef`。本批已授权上线与合并；逐项证据 `evidence/clawsta-77/p6/`。

### 上线前隔离检查

- 隔离 PG `multica_p5candidate`（台式机，经临时 SSH 隧道，迁移到 582），生产库只读。`go build ./...`、`go vet ./...` 通过；定向 handler 矩阵与全部 P5/P5-R1/P5-R2 回归 **42 项通过 / 0 失败**；`internal/auth`、`internal/middleware`、`internal/realtime`、`cmd/local-auth` 通过。
- 修正残留旧预期：`whitelisted_new_user_passes_policy`（部署开关关闭仍断言放行）改为双开才允许；Google 的 `email_not_allowed` 用例先打开 mock 实例开关。新增真实 handler 覆盖：11 行开放注册矩阵、关闭状态下的真实密码登录与真实邀请兑换（单次有效、重放失败、失败不建凭据）、邮箱码与 Google 入口的关闭拒绝。所有拒绝断言无用户/凭据/cookie/会话 token，自填邮箱保持未验证。删除只调 policy helper 的 `TestP5R3InvitationRedemptionStaysAvailable`。

### 部署

- 目标 `ssh desktop` → `D:/UWorkers/clawsta80/native`；沿用既有 Backup→Build→Stop/Start，未改 Caddy/CA/计划任务/端口/LAN 限制。
- 变更前：release `v0.2.0-20261031111027`、app `065e20ad0`、schema 575、用户/工作区/成员/凭据 = 1/1/1/0。备份 `backups/native-20261031-193723`（DB+uploads+deployment.json+sha256）。旧 release 与备份全部保留。
- 源码为宿主机真实 git checkout（`src-888a2e76f`）；Build 产出 `v0.2.0-20261031202450`，迁移到 582，`deployment.json` 的 Release/Commit 与 `/health` 一致为 `a30713a0a`。
- 实例管理员仅授予 `934e10e0-…5b4a`（邮箱 SHA-256 `5983f955…12788`），幂等重复执行不重复授权；工作区 owner 不附带该能力。`open_registration` 初始关闭；`AUTH_LOCAL_PASSWORD_ENABLED=true`、邮箱码/Google/固定码/SMTP 关闭、`APP_ENV=production`。

### 部署中发现并修复的真实缺陷

部署后实测 `local-auth bootstrap-host` 直接 panic（`flag redefined: deployment-config`）：命令块重复注册了全局已注册的 flag。桌面入口因此完全不可用，而固定身份尚无密码，没有第二条路。已修（`a30713a0a`，命令块只注册自己的 flag），补 `TestCommandFlagsParseWithoutPanic` 覆盖每个命令，重新 Build/切换后复测：错误 UUID 返回 `no user with id ...`，不 panic、不产生邀请。

### 真实浏览器验收

Chromium（Playwright）访问 `https://192.168.3.49:13443`，正常证书校验（无 `ignoreHTTPSErrors`、无 SPKI 例外、无 `-k`），**19/19 步通过**：未登录 401；旧入口 GET/HEAD 308、其他方法 405；仅密码登录页含注册入口、无邮箱码表单；自助注册 403 `registration_disabled`；`/register` 默认邀请模式；固定身份登录后进入 `/uworkers-tane/issues`；管理员「用户与注册」tab 且开关关闭；添加临时用户、复制完整 HTTPS 邀请链接（剪贴板一致）；邀请页首次设密并清除 fragment；临时账号建自己的工作区→受保护看板→刷新保持；普通账号无管理入口且管理员 API 403；退出后 401；重新登录；网页改密后落到登录页并显示提示；改密前会话 token 被拒；旧密码被拒、新密码可登录；临时工作区与账号经产品流程删除。截图见 `evidence/clawsta-77/p6/browser/`，邮箱已模糊，无邀请/密码/cookie/token。

### 桌面设置密码入口

`deploy/windows-https/Set-UWorkers-Login.ps1` 改为网页初设：核对固定 UUID、邮箱 SHA-256 与数据库行，要求 HTTPS `PublicOrigin`，有密码只开登录页、无密码调用 `bootstrap-host` 生成 15 分钟单次邀请并私下打开 `/register#token=…`；秘密只在内存与地址栏，不打印/不落盘，报错不回显 URL；`-Preflight` 无副作用。宿主机实测预检显示「尚未设置密码」与当前 release/commit。本次仅有 SSH 会话，未声称已在用户可见桌面打开窗口，正式密码仍由 Tane 本人双击桌面「设置 U Workers 登录」在网页输入。

### 结束状态与合并

- 仅剩原身份、原工作区与正式数据：`users=1 workspaces=1 members=1 credentials=0 registration_invites=0 instance_admins=1 instance_settings=0`，schema 582；`uworkers-tane` 的名称/创建时间/owner 成员与身份 `onboarded_at` 均未变，无临时凭据、邀请、会话、工作区或上传残留。
- PR #7 按既有授权 ready→合并 main，记录 merge SHA 与部署源 SHA 分别对应关系。
