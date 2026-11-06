# 全量检查汇总

基线 89cb14411，head 627b81a38。两边类型检查 10/10、lint 7/7 通过。Go 各 95 包，新增失败 0。

前端全量中执行的断言均通过，双方 views 各发生一次同源 Vitest worker 引导超时，命令均退出 1。基线 describe.test.ts 的 16 项、head file-card.test.tsx 的 1 项未在全量中执行。这两个文件在两边各独立重跑三次，每次 2 文件 / 17 项全部通过。完整用例覆盖补齐至 base 752 文件 / 9359 项、head 756 文件 / 9395 项；新增断言失败 0、新增失败包 0。全量退出码及未处理错误如实保留，逐项见 frontend-comparison.md。

| 包 | base 全量已执行 | head 全量已执行 |
| --- | --- | --- |
| @multica/core | 2288 通过 | 2316 通过 |
| @multica/docs | 16 通过 | 16 通过 |
| @multica/views | 5983 通过 | 6006 通过 |
| @multica/web | 323 通过 | 323 通过 |
| @multica/ui-lab | 46 通过 | 46 通过 |
| @multica/desktop | 687 通过 | 687 通过 |

具体超时文件、三次补跑与完整覆盖计算见 checks.json；旧 Go 失败原因见 comparison.md。
