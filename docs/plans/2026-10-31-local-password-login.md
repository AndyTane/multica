# Local password login implementation plan

**批次：P1；负责：后端架构师；执行：高级开发执行专家。**

**Goal:** 为台式机试用提供管理员本地创建的自定义账号密码登录，复用现有用户、工作区权限及会话。

**Architecture:** 新增独立的本地密码凭据，关联现有 user ID；登录成功走现有 JWT、HttpOnly cookie、CSRF 及客户端登录收尾。凭据只由主机管理员在本地交互工具中管理，不提供未认证的网页初始化或注册接口。

**Tech Stack:** 现有 Go/Chi/PostgreSQL/sqlc、共享 React 登录页和 API/auth store。密码使用维护中的标准 bcrypt 实现，禁止自制哈希；如需 x/crypto、x/term，可添加直接依赖并锁定版本。

## 当前授权与基线

- Issue CLAWSTA-77，用户 2026-10-31 08:55 回复：当前先用自定义账号密码；未来内网邮箱、内网飞书仅摸底。
- 仓库 https://github.com/AndyTane/UWorkers-nice.git，基线 main `5af22c3eb`。执行者在自己的 managed checkout 建分支，不改他人的工作树。
- 本批交付可部署候选、必要测试与本地账号管理工具。完成后交回后端架构师做专业验收、台式机部署及真实浏览器登录；不把隔离测试当成台式机交付完成。
- 可以提交工作分支并创建 draft PR；不合并 main、不替换实际部署、不重启工作区 Multica/DSH。
- 目标台式机通过 `ssh desktop` 访问；真实部署根 `D:\UWorkers\clawsta80\native`，Web 13001，API 18081，API/DB 仅回环；现有原生生命周期 `runtime\native-windows.ps1`。这些是后续部署上下文，不授权本批修改生产数据。
- 固定验证码保持关闭，APP_ENV=production；所有隐私与外发限制继续保持。

## 设计取舍

- 选择原生密码登录：比依赖外部身份服务更适合当前单机试用，且能保留现有身份和权限。
- 不用共享固定验证码或 Basic Auth 网关替代应用身份；不在本批建立 SSO 平台。
- 本地用户名独立于邮箱，可自定义；凭据关联现有用户，不能因输入任意用户名就创建用户或获得工作区权限。
- 本批只提供管理员本地创建/重置工具，不新增网页注册、找回密码或账户管理大页面。工具负责检查目标用户、唯一用户名、事务一致性；已有用户资料和成员关系保持不变。

## Task 1：凭据存储和本地管理工具

涉及 `server/internal/auth/`、`server/pkg/db/queries/`、`server/migrations/`、生成代码和独立本地管理命令（建议 `server/cmd/local-auth/`）。

1. 先测试：自定义用户名规范化与冲突、正确/错误密码、密码长度边界、已不存在/禁用用户、重置后旧密码失败。
2. 新增本地凭据表：规范化用户名、user ID、bcrypt 哈希及必要时间字段；不存明文，不添加外键。唯一索引遵循 AGENTS.md 的独立 CONCURRENTLY migration 规则；执行 sqlc 生成，不手写生成代码。
3. bcrypt 默认 cost 12；密码最少 12 字符，最多 72 UTF-8 字节，超限明确拒绝而非截断；限制用户名与请求体长度。
4. 管理工具通过主机本地权限和 DB 连接工作，读取密码时禁止回显与命令行参数。账号映射只在本地交互提示；可从部署配置读连接但不得打印配置或连接串。准备 Windows 包装脚本/快捷入口说明。
5. 测试密码仅在隔离测试进程内生成使用，不写到报告、截图、脚本参数、日志、环境转储或任何交付文件。用户正式密码由本人在台式机私下设置，不生成带密码的文本文件交接。

## Task 2：登录 API 和限流

涉及 `server/internal/handler/auth.go`（宜将新增实现放同目录独立文件）、`server/internal/handler/config.go`、`server/cmd/server/router.go`、`server/internal/middleware/`。

1. 新增 `POST /auth/password`，请求 `{username,password}`，成功保持现有 `LoginResponse` 与 cookie/CSRF 语义。错误用户名与错误密码使用相同外部错误；未知用户执行 dummy hash 校验避免明显时序差异。
2. 仅查询已配置凭据与已存在用户；复用禁用用户检查和 `issueJWT`，不走开放注册。
3. 通过运行时开关启用本地密码；默认关闭。`/api/config` 暴露能力布尔值，绝不返回凭据、用户名清单、哈希。
4. 邮箱方式增加独立开关，默认兼容原行为；此台式机后续仅启用密码，禁用无发信服务的验证码入口和服务端 send/verify 路由行为，不能只隐藏 UI。
5. 新密码路由必须在无 Redis 时仍有限流。现有 `middleware.RateLimit(nil,...)` 是 no-op，不得直接复用作为唯一保护。实现有过期清理和容量上限的单进程 limiter：账号 5 次/分钟、来源 20 次/分钟，返回 429/Retry-After；验证成功也不能清空共享来源预算。限制成本在 bcrypt 之前，伪造 X-Forwarded-For 不能绕过。
6. 单机内存限流是当前明确部署范围；不为多节点引入 Redis 强依赖，也不改变其他 API 限流行为。
7. 不记录 password/request body/token/hash，失败日志不回显解码输入。

## Task 3：共享登录界面与契约

涉及 `packages/core/api/client.ts`、`schema.ts`、`packages/core/auth/store.ts`、`packages/views/auth/login-page.tsx`、对应测试和 auth 翻译。根据实际现有 config hook 复用能力读取，不新增应用层全局状态。

1. schema 新字段有默认值，旧服务器响应仍可解析；补 malformed-response 测试。
2. 共享页面支持“账号”“密码”“登录”，正确 autocomplete、掩码、提交中防重复、失败保留用户名且不泄露密码。
3. 同时配置多种方式时保留清晰切换；password-only 时不显示不可用邮箱验证码流程。所有支持语言补必要文案，遵循仓库 UI 规范。
4. 复用登录后 auth store、工作区预取、web cookie 与 desktop token 流程；CLI callback 如复用此页也不能退化。不要复制第二套登录收尾逻辑。
5. 不修改 mobile；明确其本批范围。web/desktop 的共享改动由后端架构师交前端负责人复审，不自行反复另派任务。

## Task 4：必要验证与交付

- 后端单元/DB 测试：正确密码成功；错密/未知账号统一失败；禁用用户拒绝；无 Redis 下限流真实生效、过期恢复、伪造代理头不能绕过；请求体边界；开关关闭拒绝；生产固定码仍无效。
- 前端测试：password-only 页面、API 失败、成功进入应用、旧 config 兼容、既有验证码流程不回归。执行仓库要求的相关 typecheck/lint/tests；Go 范围执行 `make test` 或解释 Windows 等价命令。
- 隔离部署真实浏览器：自定义账号密码 → 登录 → 访问受保护页面 → 刷新 → 退出 → 再登录；未登录 /api/me 为 401。实际测试用户和凭据仅存在隔离环境，最终清理。
- 证据只记录动作、响应码、是否成功、命令 exit code、base..head；截图避开凭据输入、浏览器存储、网络认证数据。不要保存 Playwright trace/HAR/storageState 中的密码或会话。
- 更新原生部署说明：工具使用、能力开关、初始设置与重置，不能包含实际账号或密码。
- 当前目标 Web 是 HTTP，不能据此宣称局域网密码传输受保护。本批提供 TLS/可信代理兼容所需配置说明；后端架构师负责实际部署时提供 HTTPS 入口、证书信任及真实跨机器验证。不得以关闭证书校验或开放 API/DB 端口作为完成。
- 回报本 issue 并有效 @ 后端架构师：P1、base..head、PR/分支、测试与证据路径、已关闭问题、实际未关闭阻塞。此时父 issue 保持 in_progress，不能宣告最终阶段完成。

## 已完成的未来登录摸底（不在本批实现）

- `apps/docs/content/docs/auth-setup.zh.mdx:30`、`server/internal/service/email.go`：已有 SMTP，能对接内网邮件服务器发验证码。邮箱账户本身的密码/企业目录认证不是当前能力。
- `server/cmd/server/router.go:1454`：公开登录入口是 send-code、verify-code、google；`handler/auth.go` 实现 Google 专用 OAuth，没有通用 OIDC/SAML 登录配置。
- `server/internal/handler/lark.go:210`：飞书 binding redeem 先要求已登录用户。现有飞书机器人/消息通道和身份绑定不等于飞书登录。
- 建议未来复用相同 user ID 和会话，在确认企业实际提供的身份协议后接独立身份提供方：邮件验证码直接配 SMTP；飞书/企业统一身份需增加授权回调、服务端校验和稳定外部身份映射。不能凭未验证邮箱自动合并账号。本次不申请企业应用、不读取企业账号凭据、不实现这些扩展。
