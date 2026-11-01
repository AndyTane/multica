# CLAWSTA-85 批次 C：下载包、CLI 与环境变量的新旧兼容

日期：2026-11-01
范围：CLAWSTA-85「批次 C（需兼容）」——新的 Windows 下载包用 U Workers 命名；CLI 改名后新旧命令并存；
环境变量、配置目录、Cookie、HTTP 头双读过渡。Windows 计划任务改名、npm 作用域、Go module、数据库列、
system key、令牌前缀、移动端 bundle id 不动（父任务「不做」清单）。提示词与技能文本属于批次 B，本轮不碰。

品牌写法统一为 **U Workers**（带空格）。内部标识沿用仓库已有的 `u-workers` 短横线写法
（`DESKTOP_CONFIG_DIRECTORY = ".u-workers"`、`--profile u-workers`、`u-workers-desktop-*` 早已在使用）。

## 新旧名字对照

| 类别 | 旧名（保留可用） | 新名（本轮启用） | 说明 |
| --- | --- | --- | --- |
| 产品显示名 | Multica | U Workers | 批次 A 已完成界面与文档站 |
| CLI 命令 | `multica` | `u-workers` | 同一次构建产出两个可执行文件名，命令本身完全一致 |
| 版本化 CLI 下载包 | `multica-cli-<v>-<os>-<arch>.(tar.gz\|zip)` | `u-workers-cli-<v>-<os>-<arch>.(tar.gz\|zip)` | Windows 包即 `u-workers-cli-<v>-windows-<arch>.zip` |
| 旧版不带版本号下载包 | `multica_<os>_<arch>.(tar.gz\|zip)` | 不新增 | 只为已发布旧 CLI 的自更新继续发布 |
| 环境变量前缀 | `MULTICA_*` | `U_WORKERS_*` | 进程启动时双向映射，见下文 |
| 配置目录 | `~/.multica/` | `~/.u-workers/` | 已有旧目录的机器继续用旧目录；新机器用新目录 |
| 会话 Cookie | `multica_auth` / `multica_csrf` / `multica_csrf_session` | `u_workers_auth` / `u_workers_csrf` / `u_workers_csrf_session` | 服务端新旧都读、都写 |
| HTTP 头 | `X-Multica-*` | `X-U-Workers-*` | 服务端新旧都发，客户端新旧都读 |

实现上的单一来源：`server/internal/brand`（Go）与 `packages/core/config/brand.ts`（前端）。
两处都刻意保留成对的旧名常量，新增第三套拼写不做——历史拼写无法移除。

## 双读是怎么实现的

### 环境变量：进程级映射，不改 224 个读取点

`server/internal/envcompat.Bridge()` 在每个可执行文件 `main()` 的第一行执行一次：扫描进程环境，
把 `MULTICA_*` 映射成 `U_WORKERS_*`、反之亦然。规则固定且与写入顺序无关：

- 只设旧名 → 新名读到同一个值；
- 只设新名 → 旧名读到同一个值；
- 两个都设 → **新名获胜**（新旧名读到同一个新值）；
- 未涉及的变量不动；空字符串按「已设置」复制，因为「设成空」和「未设置」对读取方含义不同。

这样没有第二份读取逻辑，也没有 224 个读取点各自实现一次优先级。

桥接的代价是：daemon 自己的环境里每个产品变量都会有两个名字。所有「不让 daemon 的值进入任务子进程」的过滤点
（`mergeEnv`、custom_env 黑名单 `isBlockedEnvKey`、Codex shell 白名单、`agent resume` 子进程环境）
统一用 `brand.IsProductEnvKey` 同时拦两个前缀；只拦旧前缀会让 `U_WORKERS_*` 副本漏进任务，
子进程里的 CLI 再桥接一次时新名获胜，覆盖 daemon 为该任务注入的 token / server / 配置根。安装脚本自身的 `MULTICA_*`
变量同样支持 `U_WORKERS_*` 优先读取（`scripts/install.sh`、`scripts/install.ps1`）。
Docker Compose 的 operator 变量映射（`.env.example`、`docker-compose*.yml` 里那 41 个 `MULTICA_*`）
仍走旧名，属于部署批次；本轮的 Go 桥接保证镜像内进程对两种前缀都生效，但 Compose 只透传旧名，
这一点已写进 `.env.example` 顶部提示。

### 配置目录：旧目录优先，绝不静默搬家

`cli.ConfigRootDir()` 的解析顺序：

1. `MULTICA_TASK_CONFIG_ROOT`（也接受 `U_WORKERS_TASK_CONFIG_ROOT`）— daemon 任务私有根，行为不变；
2. `~/.multica` 已存在 → 用它。升级机器保留登录、profile、daemon 状态；
3. 否则 `~/.u-workers` — 从没跑过本产品的机器。

配置目录里放的是凭据和 daemon 状态，升级过程中自动改目录名会静默丢登录，所以不做自动迁移。
Desktop 用同一条规则解析 `profileDir`（`apps/desktop/src/main/daemon-profile.ts`），否则它持有的
`u-workers-desktop-*` profile 会落在 CLI 不看的目录。

### Cookie：旧名照读照写

`server/internal/auth` 在登录、续期时同时写三对 cookie（新名 + 旧名），读会话时**旧名优先**、新名兜底，
登出同时清六个名字。旧名优先是因为双写期内只有旧名是每个服务端版本都会写、都会清的：回滚到旧服务端后，
旧服务端的登录/登出不碰新名，而登出不吊销 token，新名优先会把上一个账号带回来。停止双写时把读取顺序翻成新名优先。这样：

- 升级前登录的浏览器手里只有旧 cookie，新服务端照样认；
- 新登录后回滚到旧服务端，旧服务端读旧 cookie，仍然认；
- 前端（`packages/core/api/client.ts`）读 CSRF 值时新旧名都读、与服务端同样旧名优先，握手仍然只用 `X-CSRF-Token` 一个头
  （加第二个头会让旧服务端的 CORS allowlist 拒绝预检，见 MUL-7436 的注释）。

### HTTP 头：服务端双发，客户端双读

- `X-U-Workers-Next-Before` / `-Next-Before-Id`：评论分页游标，服务端两套都发，CLI 新名优先读。
- `X-U-Workers-Retryable`：daemon 仓库忙信号，daemon 两套都发，CLI 两套都读。
- `X-U-Workers-Plugin-Installation`：插件 surface 的安装标识；服务端两套都读、CORS allowlist 两套都放行；
  浏览器端过渡期只发旧名（与 CSRF 只用一个头同理：旧服务端的 allowlist 不认新名，预检会失败）。
- `X-U-Workers-Timestamp` / `-Signature`：插件 webhook 签名头，两套都发；插件服务端迁移期间读哪套都行。
- `X-U-Workers-RateLimit-Scope`：seat-capacity 服务响应头，客户端两套都读。

### CLI 双命令与 Windows 下载包

- `.goreleaser.yml`：同一 `main` 构建两次，产出 `u-workers` 与 `multica` 两个二进制；两个归档族各带对应名字：
  版本化族改名 `u-workers-cli-<v>-<os>-<arch>`，旧的不带版本号族保持 `multica_<os>_<arch>`（已发布旧 CLI
  的自更新按这个名字找）。Homebrew formula 名仍是 `multica`（`brew upgrade` 不断），安装 `u-workers` 并把
  `multica` 链到它。
- `server/internal/cli/update.go`：自更新候选资产新名字优先，回退旧版本化名和旧不带版本号名；
  解包时按当前可执行文件名优先、另一名字兜底；更新完顺手刷新同目录的另一个可执行文件名
  （装了双份副本的 Windows 安装不会出现两个命令两个版本）。
- `scripts/install.sh` / `scripts/install.ps1`：下载新包名优先、旧包名兜底；装 `u-workers` 并保留
  `multica`（Unix 用符号链接、Windows 用副本）；旧安装只有 `multica` 时直接就地采用，不重复下载。
- CLI 进程按 `os.Args[0]` 决定帮助、版本输出、LEARN MORE 里显示的命令名：`u-workers` 调用显示新名，
  `multica` 调用显示旧名，两边看到的信息都与自己敲的命令一致。

## 过渡期怎么结束

C1（本 PR）只开兼容，不删任何旧路径。之后按下面的条件逐项收口，每项单独 PR：

| 项 | 停止双写的条件 | 停止双读的条件 |
| --- | --- | --- |
| Cookie | 本版本发布 ≥2 个 tag，且 Web/Desktop/Mobile 的最低支持版本都已读新名（届时回滚目标也都认识新 cookie）；同一 PR 把服务端与前端的读取顺序翻成新名优先 | 停写后再过 1 个 tag，旧名 cookie 自然过期（TTL 30 天） |
| HTTP 头 | 与 Cookie 同批（同一批客户端/服务端组合决定） | 停发后再过 1 个 tag |
| 环境变量 | 无「写」方向：新名一直是唯一写入方 | 本版本发布 ≥2 个 tag，且 `.env.example`、`SELF_HOSTING*` 文档已全部改成 `U_WORKERS_*`，Compose 映射（部署批次）同时接受两种前缀 |
| 配置目录 | 永久：不做自动迁移；旧目录一直可用 | 永不删除旧目录支持；可选将来提供显式 `config migrate`，不排期 |
| 旧 CLI 命令 `multica` | 一个 major 版本里在 release notes 宣布后移除；在此之前两个名字一直一起装 | — |
| 旧不带版本号下载包 `multica_<os>_<arch>` | 本版本发布 ≥2 个 tag，确认没有更旧的 CLI 还在自更新（发布页下载统计/issue 反馈；无遥测，按保守估计），且 Desktop 的 CLI 下载（`apps/desktop/src/main/cli-release-asset.ts`，目前只认 `multica-cli-` 前缀、靠这个旧包兜底）已改认 `u-workers-cli-` 并发布，release notes 宣布 | 与「写」同批 |
| 旧下载包名/旧二进制名回退读逻辑 | 与上一条同批 | 与上一条同批 |

判定口径统一为「本版本发布 ≥2 个 tag」而不是日历时间，因为 GitHub Actions 当前不可用、发布节奏不确定；
每个 tag 发布时在 release notes 记一行「兼容批次 C 计数」。

## 本地验证（实际输出）

本机 Windows / Go 1.26.9 / pnpm 10.28.2。GitHub Actions 不可用，以本地结果为准。

| 检查 | 结果 |
| --- | --- |
| `go build ./...`（server） | 通过 |
| `go test ./internal/brand ./internal/envcompat` | 通过 |
| `go test ./internal/cli` | 通过，除 `TestCLIConfig_TaskRootOverridesOwnerHome`：Windows 上 `chmod` 语义导致 0777 ≠ 0700，在 `origin/main` 干净 worktree 上同样失败（既有问题） |
| `go test ./internal/auth ./internal/middleware` | 通过 |
| `go test ./internal/handler ./internal/service ./internal/seatcapacity` | 通过 / 数据库相关用例按既有方式跳过 |
| `go test ./cmd/multica` | 通过，除 origin/main 上同样失败的 4 个 Windows 路径语义用例与 1 个同类 lint 用例 |
| `go test ./internal/daemon`（相关子集） | 通过，除 origin/main 上同样失败的 `TestLockReusablePriorEnvRootRejectsIdentitySwap` |
| `bash scripts/install.test.sh` | `install.sh tests passed` |
| `bash scripts/install-source.test.sh` | `install source tests passed` |
| `pwsh scripts/install.ps1.test.ps1` | `install.ps1 tests passed` |
| `pwsh scripts/install-source.test.ps1` | `install source tests passed` |
| `pnpm --filter @multica/core test` | 173 文件 / 2226 用例通过 |
| `pnpm --filter @multica/desktop test` | 通过（含 daemon-profile 新旧目录用例） |
| `pnpm typecheck` | 通过 |
| `pnpm lint` | 见下方结果 |

### 环境变量三类矩阵（真实二进制）

`u-workers auth status`，只给 `MULTICA_TOKEN`，其余环境变量隔离：

```
--- only legacy MULTICA_SERVER_URL=http://127.0.0.1:9
Token is invalid or expired: GET /api/me: Get "http://127.0.0.1:9/api/me": dial tcp 127.0.0.1:9: ...
--- only current U_WORKERS_SERVER_URL=http://127.0.0.1:9
Token is invalid or expired: GET /api/me: Get "http://127.0.0.1:9/api/me": dial tcp 127.0.0.1:9: ...
--- both: legacy=:1, current=:9
Token is invalid or expired: GET /api/me: Get "http://127.0.0.1:9/api/me": dial tcp 127.0.0.1:9: ...
```

三种情况都指向预期地址，两个都设时新名获胜。

### 配置目录三种情况（真实二进制）

```
A 只有 ~/.multica（升级机器）：config file: ...\.multica\config.json，server_url 来自旧文件，未创建 ~/.u-workers
B 全新 HOME：config set server_url 创建 ...\.u-workers\config.json，未创建 ~/.multica
C 两个目录都在：读取 ...\.multica\config.json（旧目录优先，不掉登录）
```

### 新旧 CLI 命令

```
u-workers version → u-workers v0.0.0-batch-c (commit: local, built: 2026-11-01)
multica version   → multica v0.0.0-batch-c (commit: local, built: 2026-11-01)
u-workers --help  → USAGE: u-workers <command> <subcommand> [flags]
multica --help    → USAGE: multica <command> <subcommand> [flags]
```

同一份二进制、按调用名显示；两个命令都可正常执行全部子命令。

### Cookie 升级不掉登录

Go 测试覆盖：
`TestAuthCookie_ReadsBothSpellings`（旧名 cookie 可认证）、`TestSetAuthCookies_WritesBothNameFamilies`（登录同时写新旧名）、
`TestClearAuthCookies_ClearsBothNameFamilies`（登出清六个名字）、`TestValidateCSRF_ReadsLegacyAuthCookieName`（旧名 cookie 下 CSRF 校验通过）；
前端 `packages/core/api/client.test.ts` 覆盖「只读旧名」「只读新名」「新旧都在时旧名优先」三种 CSRF cookie 场景。

## 没做 / 边界（明确留给后续批次）

1. **部署批次**：Windows 计划任务改名、Compose 里 41 个 operator 变量映射、`.env.example` 变量实际改名
   （本轮只在顶部加提示；Go 桥接与安装脚本已接受新前缀）。
2. **文档站 210 个页面的命令/变量示例**：旧名仍然可用，示例继续有效；统一改到新名放在旧名停止双读的同一批。
   `CLI_INSTALL.md`、`CLI_AND_DAEMON.md` 这两处安装/协议文档本轮已更新。
3. **批次 B**：提示词、内置技能、kickoff 文本里的 Multica/Mika（另一个 PR）。
4. **Desktop 内置 CLI 文件名**：Desktop 打包进 `resources/bin` 的私有副本仍叫 `multica`，它只被 Desktop 用绝对路径
   调用，不影响「两个命令都可用」；`findCliOnPath` 已优先找 `u-workers`。
5. **`multica://` 协议、token 前缀、`@multica/*` 包名、Go module、数据库列、system key、mobile bundle id**：按父任务「不做」清单保持。
6. **`.github/workflows/release.yml`**：release job 只在上游仓库跑，本轮改的是 GoReleaser 配置本身；
   本仓库无 tag 可验证真实产物，资产名由 `releaseAssetCandidates` 单元测试与 GoReleaser 配置审阅保证。

## 风险与回滚

- 六个 Cookie 与两套 header 是刻意多出来的写操作，体积可忽略；回滚方式就是回滚本 PR：旧服务端只认旧名，
  而本轮旧名照写，所以回滚不会把用户踢下线。
- 环境变量桥接是进程内的一次性映射，不落盘、不改子进程协议；回滚后旧名行为与之前完全一致。
- 配置目录只在「旧目录不存在」时才用新目录，回滚不会产生两个 profile 根。
- 自更新新增「刷新同目录另一个可执行文件名」是 best-effort：失败不影响主更新（只是另一个命令名版本落后）。
