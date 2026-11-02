## 目标

CLAWSTA-117：按新用户路径走一遍部署（便携包解压 → Setup → Web → 登录 → 工作区/任务 → 运行时 → CLI/daemon 输出），把用户能看到的 Multica 清干净；兼容层保留但不露出。

## 走查方法

- 便携包 v0.2.2 用 `deploy/portable/test/acceptance.ps1` 装到隔离目录，独立端口与 profile。
- Web：Playwright 中英文逐页抓文本、标题、manifest，命中 `multica` 即记录。
- CLI/daemon：用包内 `u-workers.exe` 跑 `--help`、各子命令 help、无网络/未登录错误。

## 清单

| 位置 | 原文 | 处理 | 理由 |
| --- | --- | --- | --- |
| Setup 输出、进程名、开机启动项 | 已是 U Workers | 保持 | — |
| Web 页面标题、manifest、各页正文 | 已是 U Workers | 保持 | — |
| 运行时弹窗 `U_WORKERS_CLI_PATH=/absolute/path/to/multica` | multica | 已由 CLAWSTA-105 在 main 修复 | v0.2.2 早于该修复 |
| CLI help：`Authenticate multica with Multica` 等 | Multica / multica | 改为 U Workers，命令名取用户实际输入（`cliName`） | 用户直读 |
| CLI 提示 `Run 'multica login'` / `workspace watch` / `daemon start` 等 | multica | 改为 `cliName` | 用户照抄执行 |
| CLI/daemon flag 与错误中的 `MULTICA_*` | MULTICA_* | 改为 `U_WORKERS_*`（旧名仍可读） | `envcompat.Bridge` 双向映射 |
| 网络错误「无法访问 Multica 服务器」 | Multica | U Workers | 用户直读 |
| CLI 登录成功页标题 | Multica | U Workers | 浏览器标签可见 |
| Desktop「multica CLI is not installed」 | multica | U Workers CLI | 用户直读 |
| 运行时报错（claude/codex/grok/opencode/reasonix/zeroclaw/codebuddy） | Multica | U Workers | 会显示在任务结果里 |
| DSH profile 提示、agent_ready 引导 | Multica runtime profile | U Workers runtime profile | 运行时页可见 |
| Agent 提交 trailer `multica-agent <github@multica.ai>` | 上游账号 | `U Workers Agent <agent@u-workers.invalid>` | 提交会被归到上游 GitHub 账号 |
| 本地 worktree `user.name=Multica Agent` | Multica | U Workers Agent | 出现在 git log |
| Agent Builder / Squad 提示词、MCP OAuth client_name、metrics help | Multica | U Workers | 对话、授权页可见 |
| OpenClaw 超时文案（5 种语言） | MULTICA_OPENCLAW_CLI_TIMEOUT | U_WORKERS_OPENCLAW_CLI_TIMEOUT | 聊天界面可见 |

## 保留（兼容/协议，不面向用户）

- 二进制：便携包里的 `bin\multica.exe` 旧名副本、`~/.multica` 旧配置目录（新机器用 `.u-workers`）。
- 数据库用户 `multica`、`@multica/*` 包名、Go module 路径、localStorage key、`multica://` 协议。
- 注入任务的 `MULTICA_*` 环境变量、`X-Multica-*` 请求头、`Multica-Workspace/Agent/Conversation` trailer（会被回读解析）、`Multica-Hooks/1` UA、`multica.plugin.json`、DSH `multica` profile 名。
- 服务端运维日志里的 `MULTICA_*`：`docker-compose.selfhost.yml` 只透传旧名，改了反而误导运维。
- 给 agent 看的 `multica issue ...` 指令：agent 运行时内置的就是这个命令名。

## 其他问题

- 站点标题 `U Workers — Project Management for Human + Agent Teams` 在中文页面仍是英文（登录页、首页）。
- `acceptance.ps1` 新增的 LAN 计划断言不兼容 v0.2.2 等旧包，安装本身成功，脚本误判失败。
- 本地根证书名为 `Caddy Local Authority`，非 Multica；可后续改为 U Workers 品牌。
