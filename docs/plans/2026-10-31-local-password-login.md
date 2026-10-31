# Local password login delivery plan

**批次 P3-R1；负责人/验收：后端架构师；执行：DSH。替换同路径 P3。**

## 目标与基线

接续 `76acbbfcb` / PR #6，纠正本人身份绑定并完成实际可用交付。不重做 P1/P2，不合并 main。应用仍是 release v0.2.0-20261031111027 / 065e20ad0。

HTTPS 生命周期本轮已通过独立验收：宿主脚本 SHA F531993716C76CF19A667163633263F19B19BB14AA039475E10E64BFF42B2E44；任务 Running、PT0S、999次/1分钟、电池条件关闭、StartWhenAvailable、IgnoreNew；标准证书校验 health/login/config200，未登录me401。无需再次修改或重启 HTTPS。

## 唯一交付阻塞：本人身份来源不符

后端只读核验结果：
- 当前 local-login-identity.json 的邮箱与后端本轮用户明确给出的 On Behalf Of 邮箱不一致。
- 对本轮明确邮箱查询 user，结果不存在。
- 当前绑定用户存在、已onboarded、是 uworkers:owner；C1成员仍1；本地凭据总数0。
- 不输出当前邮箱、登录名或任何凭据。不能只比较 identity文件与DB相同就宣称绑定本人，也不能从运行时拥有者资料推断请求人。

先核对你创建身份时采用的来源和原始授权。本轮后端明确身份的规范化邮箱 SHA-256 见随附 p3-review-result.json 的 expected_identity_email_sha256（trim+lower UTF-8）。你接到的 On Behalf Of 若与该值不同，说明上下文身份源冲突：不要凭显示名合并、覆盖或扩权，报告来源差异即可，交回后端集中向本人确认，不重复试错。

若确认本轮 On Behalf Of 邮箱与该值一致，沿 P3 授权仅创建/复用这个明确身份，绑定入口并通过正常 onboarding 建本人独立工作区。uworkers 已被另一个邮箱身份占用时用唯一 slug（如 uworkers-tane），不转让或覆盖原身份、C1、成员/内容；旧 P3 新建身份先保留待归属核对，不新增其凭据、不删除账户。禁止根据相同显示名接管其他身份。
创建前留受限备份；已有正式凭据不覆盖；所有密码仅本机隐藏输入，临时验证密码仅内存/加密stdin，测试后退出并删除临时凭据。正式密码仍由Tane本人设置。

## 必要验证与证据纠正

- 确认固定入口绑定 user UUID + 邮箱匹配明确请求身份；入口在实际桌面账号下可运行（必要时系统原生提权），取消无变更、已有凭据不覆盖。不公开账号名和密码。
- 仅对正确身份跑必要跨机真实浏览器登录→本人工作区→刷新→退出me401→重登；正常证书校验，无 paid agent 或 runtime 启动，不能访问C1。
- P3 的 browser/onboarding-evidence.json 实际 passed=false，identity_is_owner_of_its_own_workspace=false，响应只有slug/name；不得再称16/16通过。代码查询和本轮DB确认原新建用户确为owner，属于证据字段/断言问题，不是权限缺陷。用实际支持的成员API或明确独立DB断言验证owner，浏览器和DB结果分开记录；修正原报告说明并保留失败历史，不单纯把false改true。
- 不为报告格式单独扩大重测；与身份修正一次完成。若上下文身份来源冲突阻止纠正，交清楚已有事实与唯一待确认项，不宣称“只差密码输入”。

## 交付

同分支更新 draft PR #6，同步任务书。证据 evidence/clawsta-77/p3-r1/，只保留脱敏比较布尔值、成员范围、临时凭据清理和必要浏览器结果。
在当前issue回复并@后端架构师，写 base..head、已关闭项、未关闭项，明确是否可让本人双击入口设置正式密码。
父issue in_progress；SMTP、固定码保持关闭，企业邮箱/飞书只摸底。
