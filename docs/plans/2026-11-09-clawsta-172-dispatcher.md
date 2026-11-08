# CLAWSTA-172 R2：补通有人工来源的任务唤醒

负责人：后端架构师。执行：高级开发执行专家。继续原工作树及原分支 `clawsta-172-dispatcher-lineage`；产品基线 `0032afb394080e9f71de4beb006f22e2aa49512a`。本批只修本次现场失败、做必要回归、交候选；负责人负责 PR、部署及最终验收。

## 已确定的失败与原因

R1 已推 PR #3 并在台式机上线。历史活跃卡 24/24 可解析到 Tane，无需回填；151 已可解析。当前运行创建子卡 CLAWSTA-173 成功，但顶层 CLAWSTA-174 无 dispatcher，原子 review 被拒绝，status=backlog、revision=1、metadata={} 不变。两张临时卡已取消，不能把它们当作实际任务继续派发。

真实链：本轮后端运行 `01a1bc02-f41c-7bd7-89c6-eda8d70a9dbb` → 执行者 R1 → 后端首轮 → 前端运行 `01a1bbe3-a08d-78b4-aa1c-10b3ff83dc5e`。这一跳为 `trigger_owner`，evidence.kind=`issue_wakeup`，evidence.ref=`01a1bb03-5d93-78fd-b328-1b00798858b8`，delegated_from=`01a1bafb-cc1f-780e-a0ad-4037e781eeaa`；后者为 Tane 评论触发的 `direct_human`。CLI 已核实 wakeup 是智能体创建、source_task_id 匹配，等待169进入in_review。证据随交付附件提供。

`server/internal/service/issue_wakeup.go` 的 CreateWakeupTask 将这种可恢复等待也标成 trigger_owner，但保留来源任务。R1 SQL 无条件拒绝这个标签，错误截断了真实人工链。这是验收第2项的确定失败，不是用户需要另行取舍的问题。

## 实现边界

1. **只补通有可信来源运行的 issue_wakeup 跳转**。解析任务链时，除 delegation/comment_source 外，允许服务端记录的 trigger_owner + trigger_evidence_kind=issue_wakeup + 非空 evidence ref 沿 delegated_from_task_id 继续追溯。仍要求同 workspace、同 originator、路径去重，最后到 direct_human 才能得到派活人。不把 trigger_owner 本身当人工根。
2. 以服务端写入的任务来源快照为依据，不依赖当前可变 wakeup 配置来重写历史含义，不依赖自由文本/context客户端字段。核对现有 service 的信任边界；若发现与上述判断冲突，集中反馈负责人，不自行放宽。
3. 没有来源运行的定时唤醒、只有 created_by/owner 的唤醒、autopilot trigger_owner、自动化派生的 wakeup、跨工作区/成员不一致/坏引用/循环继续拒绝。人工请求的可靠接续与无人工来源自动化是不同情况。
4. 589 已在生产执行，**不得修改589**；新增下一未使用迁移（当前应590），up 替换函数，down 精确恢复589函数。视图和消费者继续复用，不改原子 review/权限/原始 attribution，不引入 per-card 网络请求。必要时更新内置简述以说明此边界。
5. 不改生产来源、creator 或 review_user；不部署、不推送、不合并。现场保持 R1，由负责人后续一次集中切换。

## 必要验证与交付

- 最小失败回归先复现：direct_human → issue_wakeup/trigger_owner → delegation → agent_create 顶层卡；真实创建 handler 产生来源，approve 成功且详情/列表 dispatcher/awaiting=mine 一致。
- 用现有 WakeupService 测试验证至少一次真实 enqueue 的来源字段形状，避免只用理想化 fixture。无需启动真实智能体。
- 上述无人工来源与跨租户等失败边界保留，非法 review 的 status/metadata/revision 不变。复用R1已通过用例，仅增加本次差异必要覆盖，不重跑范围外完整套件。
- 使用原隔离 PostgreSQL、原仓库生命周期；明确数据库测试实际执行且未skip。迁移 up/down/up，down 的函数定义与589一致。针对新增来源路径核对索引与查询计划，不重建无关性能项目。
- 构建最终 stamped backend/CLI/migrate 与全部迁移，manifest记录完整SHA和哈希。候选/报告/定向日志以附件回交；本地 commit，工作树干净。代码范围 base0032..head。
- 前端未改，不需要重建。负责人已修正部署复制方式：必须保留 reparse links（robocopy /SL /SJ）；不要 Copy-Item 递归展开 node_modules。原前端链接还引用169工作树，此批不要清理该目录。

本批只维护上述确定失败。完成后在172当前派发线程回复并有效 @ 后端架构师；保持172 in_progress。负责人再补现场顶层建卡/151原子提交，交前端验收展示。任务书只由后端负责人维护。
