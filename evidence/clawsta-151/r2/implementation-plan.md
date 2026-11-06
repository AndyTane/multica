# CLAWSTA-151 R2 执行与复现顺序

**目标**：只有最终触发预览确认意见能交到负责人手里，才发送评论并打回；收齐相对 v0.2.16 的测试差异和四张实际截图。

**实现**：沿用现有评论预览、评论发送和带 revision 的状态更新。core 负责接收人判定；views 保留意见并显示五语错误。小组预览新增可选 squad_id，精确识别实际路由的小组。

1. `packages/core/issues/review-mutations.ts` / `.test.ts`：预览最终内容；覆盖普通、显式 @、note、空/失败预览、其他小组/共享负责人；拒绝时不发评论。
2. `packages/views/awaiting/awaiting-page.tsx` / `.test.tsx` 与五语 issues.json：保留可编辑意见、显示原因、不改状态；评论成功后只重试状态；去标题固定宽度与重复弹层标题，保留输入框名称。
3. `server/internal/handler/comment.go` / `comment_trigger_preview_test.go`、共享类型与 schema：输出可选小组来源；兼容旧响应，对共享负责人的实际路由做回归验证。
4. 比较基线 `89cb14411` 和 R2：同机 Node/Go、相同隔离库、相同参数全量检查；测试设施修复以 common-fixtures.patch 同步应用基线，业务实现留在基线；保存原始失败、修复后输出和逐项原因，偶发失败另跑三次。
5. `e2e-visual/awaiting-you.spec.ts` 的 R2 分支：23 张真实卡摘要导入隔离库，截 1024 亮色列表、亮暗打回层、note 错误；用数据库前后快照验证 note 未写评论/状态/运行队列。生成截图 SHA-256，图片通过 issue 附件交付。
6. 汇总证据、追加 conventional commits 并推送原分支/PR #48；不合并，不升号，不发布。保留全局 issue 的后续专业验收与发布阶段。
