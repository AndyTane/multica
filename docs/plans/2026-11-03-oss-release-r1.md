# CLAWSTA-121 R2：发布工具合并与 v0.2.3 完整包

本文件为当前有效任务书；R1 实现及验证证据保留在 evidence/clawsta-121。2026-11-03 Tane 经协调负责人授权合并发布工具及现有修复，发布新版本完整包，校验上传后最后更新 latest，并交公司下载与升级步骤后转 in_review。权限收窄不再属于本任务，也不将公司实机回传作为本次转审前置条件；报告仍须区分公司实测与本地验证。

## 范围与设计

- 现有 base/main 为 3e62bb6ebb9393210a7ed44fbad021611e9bc7ff，已含 PR #20、#21、#22 的 daemon、品牌及分享地址修复。发布前再次确认 main 与开放 PR。
- 采用 v0.2.3，更新根、web、desktop 版本。先完成 PR 并按本次授权合并，再在同一 main 提交打 tag，从干净源码重新构建。不得复用二进制改版本、覆盖旧目录或手工同步安装文件。
- publish-release 增加 -Full，复用 build-package 源码构建；build-full-release 从明确程序路径组成压缩 ZIP，附工具及 UPGRADE.md，记录 FULL-FILES.json，并重新解压核验。不得打包 .env、数据、用户配置、凭据、链接或目录外内容。
- 完整包清单 schemaVersion=2/packageType=full，包含 tag、完整 commit、full.zip 与说明的对象路径/大小/SHA-256。差分保留 schemaVersion=1。旧工具拒绝新类型；公司从完整包拿新版 Fetch。
- Fetch 兼容完整包：下载、hash、安全解压、逐文件清单及源码身份验证，给出 UPGRADE.md。完整包不走差分 Apply/CheckOnly；公司使用既有完整 Source 升级，先检查 PostgreSQL 兼容、安装归属与备份，保留旧业务数据及回滚材料。
- 先上传 releases/v0.2.3/full.zip、UPGRADE.md、release.json，全部响应成功才写 channels/stable/latest.json。版本对象均 forbid-overwrite；无发布者远端读。保留新完整包作为今后差分基线。

## 验证与交付

- 两种 PowerShell 下验证完整包 prepare/fetch（固定版本及 latest）、错误身份/缺文件/多文件/坏 hash、误用 Apply、禁止覆盖、上传失败不推进 latest，保留差分回归。
- 构建环境 Go 1.26.9、Node 24.21.0、pnpm 10.28.2，依赖按锁文件安装；不运行真实模型或改现有服务。
- 新源码构建后核对版本/tag/commit、ZIP 内容与依赖、包内工具和说明；记录对象大小/hash及上传响应。真实公司升级未执行则如实标明。
- 最终交付 PR/merge/tag、下载路径、升级步骤、证据及限制；上传成功且 latest 更新后按本次新要求 in_review。

## 当前外部输入

Git 提交身份已由 Tane 于 2026-11-03 确认，使用已核验账号 AdnyTane <55218494+AndyTane@users.noreply.github.com>；仅以 git config --worktree 设置姓名邮箱，不改全局身份。按授权继续提交、PR、合并及新 tag 的完整源码发布。
