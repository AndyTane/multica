# 在 Multica 里 @DSH

F2 将 Multica 的消息投递到正在运行的 DSH 网页。DSH 在网页会话里执行，用户可以在那里查看、插话和停止。Multica 保存会话编号、执行记录和结果；后续运行继续使用同一会话。

## 本机配置

本机 DSH 网页是 `http://127.0.0.1:3080/`。模型、工具和 MCP 在 DSH 网页里配置。Multica 的 DSH 智能体选择“Use DSH web settings”；不要为它另外配置模型、思考强度或 MCP 覆盖。

在仓库根目录的 PowerShell 里构建：

```powershell
Push-Location server
go build -o bin/multica-dsh-web.exe ./cmd/multica-dsh-web
go build -o bin/multica.exe ./cmd/multica
Pop-Location
```

守护进程正在使用 `multica.exe` 时，先构建为另一个文件，等空闲后停止守护进程，再替换文件。

配置桥和运行时：

```powershell
./scripts/configure-dsh-web.ps1 `
  -WebOrigin 'http://127.0.0.1:3080/' `
  -AccessLog 'D:/Tools/DeepSeekHarness/dsh-web-launch.log' `
  -DataHome 'D:/Tools/DeepSeekHarness/data'
```

脚本把不含凭据的配置存到 `~/.multica/dsh-web.json`，复用或创建 DSH Web 运行时配置，并固定桥命令的本机路径。使用其它 Multica CLI profile 时，加 `-Profile <名称>`。

`.env` 加上以下配置，然后按 [启动与重建](2026-10-13-multica-启动与重建.md) 重建并启动后端、网页：

```dotenv
MULTICA_DSH_WEB_URL=http://127.0.0.1:3080
```

重启守护进程，在 Multica 创建名为 DSH 的智能体并绑定 DSH Web 运行时。可通过网页完成；智能体已有时只修改运行时绑定。

本次已在“Agent观测站”工作区完成配置，DSH 智能体已绑定在线的 DSH Web 运行时，模型为 `web/default`。刷新 Multica 网页即可选择它。构建和测试记录见 [F2 结果](2026-10-14-multica-F2-结果.md)。

## 使用

1. 在任务评论中选择 `@DSH`，输入要求并发送。
2. 打开运行详情，点击“打开 DSH 网页”。在 DSH 的对应工作区中找到 `Multica · ...` 会话。
3. 在 DSH 网页继续插话；Multica 的后续消息在当前运行结束后继续进入同一个会话。

当前 DSH 没有经过确认的会话深链接，因此按钮打开网页首页。旁边可复制会话编号，用于核对对应会话。网页首页需要已有浏览器登录；桥不会把启动 token 放进链接。

## 故障处理

- 网页未启动、启动日志过期或认证失败：先按正常方式打开 DSH，确认日志文件是当前启动日志，再检查桥配置。桥不会重启 DSH。
- 会话忙：等网页当前运行结束，再从 Multica 发起下一次运行。网页里的插话可以在运行期间进行。
- 投递状态不明：先去 DSH 网页核对消息和结果，避免重复发送。桥不会自动重发消息。
- 项目路径不匹配：确认 Multica 的工作目录仍然存在，且继续使用原会话；桥拒绝把其它项目的会话接进来。
- 修改 DSH 网页端口：同步更新 `dsh-web.json` 和 `.env` 中的网页地址，并重启后端。

## 验证

```powershell
pnpm test:dsh-web
./server/bin/multica-dsh-web.exe --profile multica --probe
```

第一条使用模拟网页主机，不调用真实模型。第二条只验证本机网页认证，不投递消息。

真实模型测试单独授权后执行；测试先检查开关，再读取桥路径并接入网页账号：

```powershell
$env:MULTICA_RUN_REAL_AGENT_SMOKE = '1'
$env:MULTICA_DSH_WEB_SMOKE_PATH = (Resolve-Path './server/bin/multica-dsh-web.exe').Path
Push-Location server
go test -tags=agentintegration ./pkg/agent -run '^TestDshWebBridgeRealSession$' -count=1 -v
Pop-Location
```

测试发送两条要求不使用工具、不修改文件的短消息，并验证返回结果和会话编号一致。

2026-10-14 已获授权并通过此验证，结果见 [F2 结果](2026-10-14-multica-F2-结果.md)。

修复桥后需要恢复已经投递的测试时，可同时设置 `MULTICA_DSH_WEB_SMOKE_CWD` 和 `MULTICA_DSH_WEB_SMOKE_REQUEST_PREFIX`，使用原测试的项目目录和消息编号前缀。目录必须存在；测试先从记录取回第一条已完成消息，再续接第二条。保持授权开关开启，先核对原会话，避免把新的消息编号当作恢复。
