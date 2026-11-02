# 分享地址 R3 实施计划

**目标：** 从本机入口复制分享链接时，优先使用部署中明确配置的、有效且非回环的前端 origin；保留 R1/R2 成果，继续 PR #22。

**方案：** `/api/config.public_app_url` 独立于 daemon 配置，依次校验 `MULTICA_APP_URL`、`FRONTEND_ORIGIN`。只接受无凭据、无路径参数的 HTTP(S) origin，排除回环及未指定地址。core schema/store 接收此可选字段；web 导航适配器订阅配置并在复制时读取最新值，原样拼接应用路径、query、hash。未加载、旧服务端或无有效地址时沿用浏览器 origin。桌面适配器继续使用原有 runtime 配置。

**取舍：** 不用构建期常量（不能随便携部署变化），不借用 daemon 字段（用途不同），不让浏览器猜局域网地址（不能判断同事的访问入口）。不改邀请创建、注册和权限。

**执行与验证：**

1. 在 `server/internal/handler/configpolicy/` 添加公开配置契约测试，在 core 添加地址解析/schema/store 测试，在 web 导航测试中覆盖配置晚到及旧回调读取新地址；先确认回归失败。
2. 修改 `server/internal/handler/config.go`、core config/schema/auth initializer、web 导航适配器；运行窄测试及既有分享相关测试。
3. 仅在既有 R2 隔离根构建和运行候选 API/web；测试域名通过 Chromium 映射到回环服务，从本机和另一前端入口复制同一条已有链接，验证新账号加入目标工作区。脱敏记录、截图，停止服务。
4. 更新 `docs/windows-portable.md` 的旧包绕行及新行为，关闭过时的标签缺失判断；新增 R3 证据，不改历史记录。
5. 执行相关 lint/typecheck/build 和 diff 检查；按文件提交、推送原分支，更新 PR 描述，在触发线程回交后端架构师。保持 in_progress，不合并、不发版、不改公司部署。
