# Codex_Tasks：DSH 网页桥三项优化（F2 收尾）

2026-10-14 · Claude 写 · 来源：F2 评审的三条建议，tane 已同意做。

## 背景

F2 已评审通过，提交 `2bc39ed65..57be19791` 在 `tane/main` 上。桥的代码在 `server/internal/dshweb/bridge.mjs`，启动程序在 `server/cmd/multica-dsh-web/main.go`，守护进程那边的改动在 `server/internal/daemon/daemon.go` 第 8722 行附近。三项都只动这几处和对应测试。

仓库：`D:\01_Projects\multica-fork`，分支 `tane/main`。

## T1　运行说明不要每轮都塞进 DSH 会话

**现状**：守护进程把运行说明（`runtimeBrief`）作为 `system_prompt` 传给桥，桥把它拼在每次的用户消息前面（`execute()` 里的 `text`）。DSH 网页会话会一直保留历史，所以同一个会话每续接一次，就多一份完整的运行说明，上下文越积越多。

**要做**：
- 新会话（没有 `resume_session_id`）照旧发完整运行说明。
- 续接时，运行说明和上次发过的一样就不再发；变了才发。先看 `execenv.InjectRuntimeConfig` 产出的内容里有没有每次运行都不同的部分（任务 id 之类）。如果有，把会变的部分单独、简短地发，不能因为省略而让 DSH 少拿到本次运行需要的信息。
- 「上次发过什么」怎么记由你定（比如在 `~/.multica` 下按会话存一个哈希，或从会话记录里找上一次的说明）。桥崩了、记录丢了时，宁可多发一次，不能漏发。
- `cli.instructions`（临时 CLI 入口的路径）每次运行都不同，照旧每次发。

**验收**：
- 测试：同一会话连续两次运行，第二次发出的消息里不含完整运行说明；运行说明变了的那次会重新发；新会话一定发。
- 现有 12 项桥测试仍然通过。

## T2　轮询时只读会话记录的新增部分

**现状**：`execute()` 每 0.5 秒调一次 `readRows(file)`，把整份 `session.v3.jsonl.zstd` 从头读、全部解压、全部解析。会话一长，每次轮询的开销跟着涨。

**要做**：
- 记住上次读到的最后一个完整 zstd 帧的字节位置，之后只从那里往后读、解压、解析。
- 帧没写完（最后一帧不完整）时留到下次，跟现在一样。
- 一行 JSON 被拆在两个帧之间的情况要处理：没有换行结尾的半行留到下次拼上。
- 文件变短或被替换时照旧报 `session_log_replaced`。
- `prepareSession` 里的首次读取可以继续读整份，但要把读完的位置交给轮询接着用。

**验收**：
- 测试：分多次追加帧时只解析新增行；最后一帧不完整；一行跨两个帧；文件被截短。
- 自己造一份大一点的假会话记录（比如几万行）对比改前改后单次轮询的耗时，结果写进下面说的结果文档。数字是实测的就写实测，没测就别写。

## T3　桥脚本不要整个塞进命令行

**现状**：`multica-dsh-web.exe` 用 `node --eval <整份脚本>` 启动，脚本现在约 17KB，加上转义更长。Windows 命令行上限约 32KB，再加功能就可能撞线。

**要做**：
- 启动时把嵌入的脚本写成文件再用 `node <文件>` 运行。建议按内容哈希命名（比如 `%LOCALAPPDATA%\multica\dsh-web\bridge-<sha256 前 16 位>.mjs`），文件已存在且内容一致就直接用；写入时先写临时文件再改名，避免两个进程同时写坏。
- 路径里有空格、中文用户名时也要能跑。
- 退出码和现在一样透传；Node 不存在或版本不够时的报错保持现有说法。

**验收**：
- Go 测试：脚本文件按哈希生成、重复启动复用、内容被改坏时会重写。
- 本机重新构建 `server/bin/multica-dsh-web.exe` 后，`--probe` 和 `--list-models` 通过（这两条不发提示词）。

## 通用要求

- 每项一个提交，提交信息用 `fix:` 或 `perf:` 开头，提交在 `tane/main` 上。不推送。
- git 按文件名暂存。禁止 `git add -A`、`git add .`、`git stash`、`git checkout --`、`git reset --hard`。
- 跑：`pnpm test:dsh-web`；`go test ./pkg/agent ./internal/handler ./cmd/multica-dsh-web`（在 `server` 目录下，按实际包路径）；`go vet` 同样这几个包。
- 守护进程正在用的 `multica.exe` 不要直接覆盖；这次应该只需要重建 `multica-dsh-web.exe`。如果确实要重建 `multica.exe`，先构建成别的文件名，等守护进程空闲再换。
- 真实模型测试（`TestDshWebBridgeRealSession`）要 tane 明确同意才跑。
- DSH 相关的禁区照旧：不重启 DSH 网页，不改 `cordis.patch.yml`，不登记 DSH 插件；不碰 `session-a9000a71-075c-4640-88b6-091a4996f301` 和 `session-db7ff202-4c65-400c-b1d8-cf82becadec9`；DSH 启动 token 和 Multica 的任务 token 不出现在任何输出、日志、测试快照、文档和评论里。

## 交付

- 在 `docs/2026-10-14-multica-F2-结果.md` 末尾加一节「F2 收尾优化」：三项各改了什么、跑了哪些测试、T2 的实测耗时、哪些没验证。
- 做完交给 Claude 评审，评审通过前不算完成。
