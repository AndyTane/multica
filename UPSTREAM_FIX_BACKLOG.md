# 上游修复跟进清单（Multica fork）

> 用途：把上游 `multica-ai/multica` 里**跟我们环境相关**的修复挑出来排队，等有时间再审/再挑。
> 生成：2026-11-06（由 Hermes 分析 GitHub releases + 本地代码对比得出）
> 状态：**全部未应用**，仅登记

---

## 一、现状快照

| 项 | 值 |
|---|---|
| 本地分支 | `tane/main` |
| 本地版本 | **`v0.6.1-53-gfda0c5852`**（v0.6.1 + 53 个自有提交） |
| 本地 HEAD | `fda0c5852` · 2026-10-19 · fix(privacy): disable external reporting and anonymous metrics |
| 上游最新 | **`v0.8.0`** · 2026-11-03 |
| 差距 | 8 个版本、**363 个提交**、约 **207 个 fix** |
| 上游仓库 | `github.com/multica-ai/multica`（55k★，活跃） |

**53 个自有提交的四个方向**（rebase 时会撞）：
1. Windows 原生部署（`D:\MulticaData\native` 那套）
2. 上下文压缩（Claude/Codex between-runs compaction）
3. DSH 桥（web session 事件、cache 启动）
4. 隐私（关掉外部上报与匿名统计）

受影响目录：`server/internal`(114 文件)、`packages/views`(54)、`server/pkg`(40)、`server/cmd`(32)、`apps/mobile`(20)、`server/migrations`(**18 个迁移**)

---

## 二、第一批：`daemon.go` 同族（建议一次改完）

三条都在 `server/internal/daemon/daemon.go`，其中 #9197 的补丁上下文与本地 `daemon.go:190-192` **完全对得上**。
本地该文件有 5 个自有提交，但这几段未被我们改过 → 预期冲突小。

| # | 修复 | 版本 | SHA | PR | 为什么相关 | 冲突面 |
|---|---|---|---|---|---|---|
| 1 | `daemon: keep host TMP/TEMP for every Windows task` | v0.6.2 | `3d021489f4` | #9197 | MSYS/Git Bash 的 `/tmp` 挂载在一个用户的所有 MSYS 进程间共享；任务私有 TMP 会在任务结束删目录，**弄坏后续所有任务的 /tmp**（上游 issue #8824）。本地正是老写法（只对 claude 删 TMP/TEMP） | 小：daemon.go 15 行 |
| 2 | `daemon: preserve host temp for Windows Claude Git Bash` | v0.6.2 | `eb59a69722` | #8985 | 同族前置修复，先看它理解上下文再打 #9197 | 小 |
| 3 | `daemon: don't count host sleep as idle-watchdog silence` | v0.6.2 | `b115ec80f1` | #9201 | **这台机器有休眠史**，休眠被误判成 watchdog 静默 → 任务被判死 | 小 |

**验收**：`go build ./...` → `go test ./server/internal/daemon/...` → 手工跑一个 Windows Git Bash 任务确认 `/tmp` 可用。

---

## 三、第二批：要动其他文件（逐条评估）

| # | 修复 | 版本 | SHA | PR | 为什么相关 | 冲突面 |
|---|---|---|---|---|---|---|
| 4 | `daemon: retry tasks interrupted by shutdown` | v0.8.0 | `1c82a5b3f1` | #9778 | 关机/休眠打断的任务自动重试，不再变僵尸 | 中（逻辑改） |
| 5 | `daemon: preserve task environment in Hermes tool shells` | v0.7.2 | `ca593b3103` | #9607 | 我们跑 Hermes agent，工具 shell 里环境变量会丢 | 中 |
| 6 | `daemon: reject legacy DSH profiles` (ZIC-374) | v0.7.2 | `ec72a00ac3` | #9649 | **DSH 桥**：老 profile 会被当成合法配置 | 中 |
| 7 | `daemon: treat a DSH profile without the runtime as missing` | v0.6.4 | `bbf70d125d` | #9287 | 同上：缺 runtime 时静默失败 | 中 |
| 8 | `daemon: recover undelivered batch claims after timeout` | v0.6.3 | `8352787263` | #8774 | 批量任务发出但 daemon 未收到，超时不回收 | 中 |
| 9 | `daemon: preserve Git content during artifact GC` | v0.6.3 | `640788c059` | #7893 | GC 可能删掉 worktree 未提交改动 | 中 |
| 10 | `execenv: exclude committed local HEAD changes from resume replay` | v0.6.3 | `6d5cdcbed2` | #8549 | 恢复任务时重放已提交改动 | 中 |
| 11 | `runtime: recover stalled HTTP/2 connections` | v0.7.2 | `84e56b6b00` | #9551 | **daemon 掉线不重连**就是它；`client.go` 本地 0 改动，但新增 `server/pkg/httptransport`，共 8 文件 | 小–中（多新增） |
| 12 | `runtimes: price Bedrock-served Anthropic ids and DeepSeek-V4.1-Flash` | v0.7.1 | `c11b3375f5` | #9561 | **我们用 DeepSeek**，成本统计口径不对 | 小（价格表） |
| 13 | `lark: apply pong heartbeat updates and close failed sessions` | v0.7.2 | `799f5614a7` | #9708 | 飞书心跳/断线清理 | 小 |
| 14 | `lark: preserve rich-text post file attachments` | v0.7.1 | `1ad7fa1f94` | #9302 | 飞书发文件时附件丢失 | 小 |

---

## 四、已排除（不适用本机）

| 修复 | 原因 |
|---|---|
| `desktop: stop daemon before Windows update install` #9582 | 我们用 native Windows 部署 + 网页端，没跑 Electron 桌面端（进程里只有 `multica` daemon） |
| `daemon: detect the Codex CLI nested in ChatGPT.app` #8944 | macOS 专用路径 |

## 五、暂不跟（大改造，留待评估）

| 项 | 原因 |
|---|---|
| `daemon: trust the backend's SSL_CERT_DIR CAs when syncing repos` #9312 | **1260 行 / 23 文件**，新增 `gitca` 包，且改 `daemon.go`（有本地冲突）。除非真出现「自签 CA 下 git 同步失败」再动 |
| Workflow 整套（0.6.3 → 0.8.0） | 属于新增能力不是修复；需要改 `server/internal` + `packages/views/issues`，与自有提交重叠大。功能判断：我们现在的「项目描述 + 负责人 agent 派活」够用，**暂不升级** |

---

## 六、执行模板（每条照这个走）

```bash
cd D:/01_Projects/multica-fork
git checkout -b fix/upstream-<编号>
# 1. 先单独看补丁，确认上下文
curl -s "https://api.github.com/repos/multica-ai/multica/commits/<SHA>" \
  | python -c "import json,sys;d=json.load(sys.stdin);[print(f['filename']);print(f.get('patch','')) for f in d['files']]"
# 2. 手工贴 / cherry-pick（有上游 remote 时）
git fetch upstream && git cherry-pick <SHA>
# 3. 编译 + 测试
go build ./... && go test ./server/internal/daemon/...
```

**验收纪律**：每条单独一个分支、单独一次验证，别把多条混在一个提交里；记录「改了什么 / 怎么验的 / 结果」。

---

## 七、怎么复现这份分析

```bash
# 版本与修复列表
curl -s "https://api.github.com/repos/multica-ai/multica/releases?per_page=30"
# 提交差距
curl -s "https://api.github.com/repos/multica-ai/multica/compare/v0.6.1...v0.8.0"
# 单条补丁内容
curl -s "https://api.github.com/repos/multica-ai/multica/commits/<SHA>"
```

统计口径（2026-10-04 → 2026-11-05 窗口）：8 个 release、363 提交、207 个 fix。
按范围分布：daemon 22 · agent 19 · issues 13 · views 11 · wecom 10 · skills 7 · runtimes 7 · desktop 7 · lark 5。

---

## 八、变更记录

| 日期 | 动作 | 结果 |
|---|---|---|
| 2026-11-06 | 建档 | 分析完成，未应用任何改动 |
