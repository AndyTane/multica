# CLAWSTA-172 R1：补齐派活人来源

负责人：后端架构师。执行：高级开发执行专家。仅本批实现和隔离验证；后端验收、PR 集成、现场切换及历史数据补齐由负责人接续。

## 目标与基线

让由真实成员要求派生的智能体卡能够发布 `review_by=user`，查询、列表和错误原因使用同一规则。保持没有成员来源的执行卡不可发布。

- 仓库是 `AndyTane/multica`，不是 UWorkers。基线 `47b533e41d0acd33cd090d91fa7dd6b1e7e75eb7`，包含 CLAWSTA-169 已上线功能。
- 本批工作树：`C:/Users/Administrator/multica_workspaces/agent-guanc-6e7d72e80f35/clawsta-172-e8a035c6ba44/workdir/multica`，分支 `clawsta-172-dispatcher-lineage`。已隔离，可以在此实现；不要改 169 的工作树或 D 盘主工作树。
- PR #3 仍 open、未合并，base `tane/main`，head `clawsta-169-review-handoff`。执行者先交本地提交；负责人验收后并入 #3 或开新 PR，未经 Tane 明确授权不得合并。

## 已核实的根因

`server/internal/handler/issue.go` 普通 agent create 已从鉴权后重写的 `X-Task-ID` 记录 `origin_type=agent_create`、`origin_id=当前运行`，并核对 task.agent_id。无需新增客户端可填的成员字段。迁移 586 的 `issue_dispatcher` 却显式排除了 agent_create 和父卡。

review 校验、批量响应、awaiting 筛选与问题原因都已使用这个视图，优先在此收敛规则，禁止另写一套只供 review 的回退。来源规则的修改不能改变 canInvokeAgent、权限归属或后台任务的原始 attribution。

151 首轮运行 `01a1af88-c85a-7a72-8bb6-0f0d70d0c65d` 的 CLI 历史已证明：source=delegation、precise=true、originator=Tane，delegated_from_task_id=`01a1af87-297f-7739-b74b-3b4f546e551a`。这证明存在成员委派链，但尚未证明 151 自身 origin 字段及全部历史卡均完备，不能据此宣称迁移全覆盖。

## 实现约定

1. 保留成员亲建卡与 quick_create 原有校验。智能体卡优先沿同 workspace 的父卡向上，继承最近可解析的派活人；没有可继承来源时，再使用自身经服务端记录的 agent_create 运行来源。成员亲建子卡仍认自身创建者。若最近来源成员已离开，保留该 ID 并标 inactive，不绕到更远成员冒充有效派活人。
2. 父卡循环、坏引用、跨工作区关系必须终止且不得串租户；递归路径去重。父子和任务来源都使用库内关系，不读自由文本、metadata 或客户端 supplied user/task。来源任务必须属于创建 agent 且与 issue 同 workspace；核对创建 handler 的租户验证是否完整，必要时补齐并加真实 handler 测试。
3. `originator_user_id` 与 `accountable_user_id` 意义不同，禁止把 agent owner、rule publisher 或 accountable fallback 当作用户要求。参考 `server/internal/attribution/attribution.go` 与 `service/task.go`；可信成员评论/提及/指派/quick-create 和它们的委派链可解析。自动化没有成员发起证据时不能因为 trigger_owner/rule_owner 或某个 owner 字段非空而被当作人工派活；必要时沿 delegated_from_task_id 检查根来源，避免只查最末一跳 source=delegation。历史字段缺失需按实际证据报告，不猜人。
4. 增量迁移（选下一个未使用号），提供 down；不要修改已执行的 586。批量解析仍保持批量查询，不增加每卡数据库往返。修改 SQL 后按仓库要求生成 sqlc。评估实际查询计划：父链递归不能让单卡发布或分页多次扫描全库；用相同隔离数据说明改前后耗时和规模，不编造阈值。
5. 区分「无可信成员来源」与「派活人已离开工作区」的提交错误，写清可操作原因。既有 `no_dispatcher` / `dispatcher_inactive` 语义与新判定一致。前端尽量复用服务端投影，不新增推断；若旧文案说所有 agent_create 都无派活人，修正对应说明、内置技能和 runtime brief（仅没有来源的卡才走普通交付状态）。
6. 本批不自动回填生产 issue、不把 23 张卡一律归给 Tane、不改 creator、不用 review_user 绕过 dispatcher。已有 origin/父链完备的历史卡随视图更新自然生效；其余输出具体缺失条件、一次性补齐方案与预计影响范围，负责人先在卡上公布再执行。现场数据只能用 multica CLI 读取；隔离测试使用仓库管理的测试环境，禁止拿生产库跑 fixture。若 CLI 不暴露所需字段，明确缺口，不绕过平台接口直接查生产库。

## 必要验证

- 真实 CreateIssue handler 的普通建卡、可信 task 来源、防伪造 task/跨 workspace；子卡/多层父链/顶层卡、最近父卡优先、成员离开、环和缺失引用。
- 同一张合法 agent_create 卡，review approve 原子成功并填正确收件人，详情/列表/awaiting=mine 与问题原因一致；非法来源拒绝且 status、metadata、revision 不变。
- 无成员来源的定时/自动化及其后续委派拒绝，不能借成员 owner 通过；既有 quick_create、防伪造上下文、specialist、reply/act 与 review 清理回归保持通过。
- 更新原本断言所有 agent_create 都应拒绝的测试；保留明确无可信来源的拒绝用例，不简单删除安全测试。
- 定向 Go/SQL 测试必须真正执行，数据库未配置而 skip 不算通过。前端有改动时跑对应 core/views 测试与必要类型检查。构建可运行的候选 backend/CLI；若 brief/CLI 有修改，注明 daemon 需空闲重载。不安装候选、不切换现场。

## 回交

提交聚焦代码与必要说明，不提交凭据、.env、运行时管理块或 .multica；按文件名暂存。交付 base..head、修改摘要、真实测试命令与结果、候选路径/哈希、历史覆盖结论及未关闭项。报告和日志放当前工作目录并通过 issue 附件交付。

完成后在 CLAWSTA-172 的派发线程回复并有效 @ 后端架构师。保持本卡 in_progress：R1 是本卡的一部分，现场第 1/2 项和前端复核尚由负责人负责。本任务书仅负责人维护；发现规格冲突先集中说明，不自行扩展产品边界。
