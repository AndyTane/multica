# 前端失败项对比与复跑

最终对照命令、机器、隔离数据库与参数相同；六个前端包均执行。base 全量 751 文件 / 9343 项断言通过，head 全量 755 文件 / 9394 项断言通过；双方各有一次同源 worker 引导错误，退出码均为 1。

| 项目 | base 最终全量 | head 最终全量 | 原因 | 处理与证据 |
| --- | --- | --- | --- | --- |
| Vitest forks worker 启动 | 1 次超时 | 1 次超时 | 框架固定 60 秒引导窗口，worker 未响应；没有产生断言失败 | 下列两个文件在两边各独立重跑三次，每次 17 项通过；base/head 的 startup-repeat1/2/3-complete.log |
| autopilots/components/schedule-editor/describe.test.ts | 16 项未执行 | 16 项通过 | base 该文件的 worker 未启动 | 两边三次均通过 |
| editor/extensions/file-card.test.tsx | 1 项通过 | 1 项未执行 | head 该文件的 worker 未启动 | 两边三次均通过 |
| 桌面打包脚本（首次基线） | 最终 687 项通过 | 最终 687 项通过 | 首次基线因 CRLF / Node 环境导致脚本加载失败；原输出为 base/frontend.log | 测试设施修复，最终全量两边均通过 |
| 早期 head 4 worker 尝试 | 最终同类启动超时已在基线复现 | 原有三份文件未启动 | Vitest 引导超时；已执行断言均通过 | create-agent-dialog 与四个 R1 时序文件三次每次 68 项通过；issue-limit-upgrade-dialog / issue-page-refresh 三次每次 4 项通过；对应 views-repeat / worker-repeat 日志 |

完整覆盖补齐：base 752 文件 / 9359 项，head 756 文件 / 9395 项。新增断言失败 0、新增失败包 0；原始框架错误与退出码保留。已在全量通过的文件不重复累计，补跑只为各自缺失文件增加一次唯一覆盖。细节和机器可读结果见 checks.json，全部输出及 SHA-256 见 logs-manifest.json。

Go 的全部 446 条基线失败记录与 head 的逐项对比另见 comparison.md。当前结果按任务书 R2 的基线旧失败和偶发项三次复跑口径交回验收。
