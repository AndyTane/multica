# R2 测试设施修复与范围

程序行为的 R2 修复是最终接收人预览校验及小组来源字段。以下变更让测试在本机 Windows 和重复使用的隔离数据库中验证原有契约。

| 设施 | 原因与修复 |
| --- | --- |
| 桌面打包脚本 | 纯 Node 测试明确使用 Node 环境；`.mjs` 固定 LF，修复 Windows 检出后加载时的 SyntaxError。 |
| Go 默认测试 CLI 拦截名单 | 名单固定 LF，避免 Bash 把行尾 CR 当成非法命令字符，导致测试尚未运行就退出。 |
| 配置 handler | 两个原测试创建的空 Handler 没有 Queries；为其提供既有测试数据库查询对象，保留配置断言。 |
| 路径断言 | 使用本机绝对路径；按 JSON / `%q` 编码比较；Git 与接口使用正斜杠的地方同步规范化期望。 |
| OpenClaw HOME 夹具 | 同时设置 HOME 与 USERPROFILE，避免 Windows 从真实用户目录读取配置。 |
| DSH 目录夹具 | 每个子用例使用自己的临时 APPDATA，避免两个布局相互串扰。 |
| 假 CLI | 自愈缓存用例使用自建 `.cmd`；评论 shell 夹具同时提供 `.cmd` 和扩展名为空的 sh 程序，分别供 PowerShell 与 Git Bash 使用。 |
| 宿主平台分支 | macOS bundle 的四个可执行位用例在 Windows 明确跳过；浏览器配置的非 Windows 分支使用既有主机注入验证。 |
| 权限断言 | CLI 配置的内容/隔离断言保持；POSIX 权限位只在相应平台检查。上传文件在 Windows 检查可写性。其他涉及 ACL、目录锁和符号链接语义的旧失败仍交回复审。 |
| 用户 Git 配置 | 测试仓库固定字节属性与 autocrlf，保持工作树/克隆文件内容一致，沿用机器的 Git 身份。 |
| 本地化诊断 | Windows cmd.exe 的 node 缺失检查验证 stderr 分支及命令名，兼容中文系统。修复提示按 PowerShell/POSIX 分别断言。 |
| 内置技能 | 测试读取 frontmatter 前统一 CRLF/LF，再检查同一份 YAML、描述和触发词。 |
| 注册集成用例 | 需要开放注册的两个用例显式保存、打开并恢复数据库开关，避免依赖其他测试留下的实例状态。 |
| 搜索对照 | R1 已增加 `status_changed_at` 投影；旧对照测试的扫描器同步增加该列，保留原匹配、排序和分页的对照。 |
| 清理批次 | 在原有表锁/事务中暂存并恢复其他上下文的计划时间，让有界清理只处理本用例的三个对象，避免复用库中的历史记录占满批次。 |

`common-fixtures.patch` 可应用到已发布基线，内容为通用测试设施与换行规则；`r1-search-parity.patch` 单独记录依赖 R1 新字段的对照测试同步，不能应用到没有 584 迁移/类型的基线。基线 Go 全量使用的初始共同补丁另存 `base/common-fixtures-initial.patch`，后续修复由基线专项和最终 head 全量验证；最终基线前端全量使用完整共同设施补丁，产品实现仍固定为 89cb14411。

原有权限、符号链接、Windows 仓库路径、运行环境删除合并窗口、OpenClaw 缓存/预算、归档路径和企业微信文件生命周期行为详见 `comparison.md`。需要补 Windows 原生假进程的协议测试保留失败，后续须连同相应运行时契约处理。

在已经检出的 Windows 基线上应用共同补丁后，按新属性把以下三个工作树文件归一化为 LF，保留补丁内容。本次比较也执行了此步骤；新检出的 head 由根目录属性直接保证 LF。

```text
node -e "const fs=require('fs');for(const p of ['apps/desktop/scripts/package.mjs','apps/desktop/scripts/package.test.mjs','scripts/agent-cli-command-names.txt'])fs.writeFileSync(p,fs.readFileSync(p,'utf8').replace(/\r\n/g,'\n'))"
```
