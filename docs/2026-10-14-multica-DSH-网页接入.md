# 在 Multica 里 @DSH

Multica 将消息投递到正在运行的 DSH 网页。DSH 在网页会话里执行，用户可以在那里查看、插话和停止。Multica 保存会话编号、执行记录和结果；后续运行继续使用同一会话。

桥通过 DSH 的 `session/follow` 事件流接收回复，通过 `session/page` 补读历史，不读取 DSH 的内部会话文件。断线时按事件序号补齐记录，不自动重发提示词。发送前会验证接口、项目目录和工作区归属。

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
  -AccessLog 'D:/Tools/DeepSeekHarness/dsh-web-launch.log'
```

脚本把不含凭据的配置存到 `~/.multica/dsh-web.json`，复用或创建 DSH Web 运行时配置，并固定桥命令的本机路径。使用其它 Multica CLI profile 时，加 `-Profile <名称>`。配置只需网页地址和当前启动日志路径；旧配置中的 `dataHome` 不再使用。

`.env` 加上以下配置，然后按 [启动与重建](2026-10-13-multica-启动与重建.md) 重建并启动后端、网页：

```dotenv
MULTICA_DSH_WEB_URL=http://127.0.0.1:3080
```

重启守护进程，在 Multica 创建名为 DSH 的智能体并绑定 DSH Web 运行时。可通过网页完成；智能体已有时只修改运行时绑定。

本次已在“Agent观测站”工作区完成配置，DSH 智能体已绑定在线的 DSH Web 运行时，模型为 `web/default`。刷新 Multica 网页即可选择它。构建和测试记录见 [F2 结果](2026-10-14-multica-F2-结果.md)。

## 使用

1. 在任务评论中选择 `@DSH`，输入要求并发送。
2. 打开运行详情，点击“打开 DSH 网页”。在 DSH 的对应工作区中找到 `Multica · ...` 会话。
3. 当前运行还在执行时，可在 DSH 网页使用插话（`steer`）补充要求。Multica 的后续消息在当前运行结束后继续进入同一个会话。

桥只跟踪 Multica 发起的当前轮次。DSH 默认在运行中按 Enter 或点击发送会把消息排到下一轮；默认配置下 Ctrl+Enter 才是插话。如果你改过 Enter 行为，两者会交换。运行结束后，即使使用插话手势也会开启新轮次，其结果不会自动同步回原 Multica 任务；新的要求应从 Multica 发起。

同轮插话后的文本会进入运行记录，但任务评论还有独立的交付规则：如果 DSH 已经发过评论，后台不会用新的最终文字覆盖它。因此不要把“桥读到新回复”理解为“旧评论一定自动更新”。此场景尚未做真实模型插话验收。

当前 DSH 没有经过确认的会话深链接，因此按钮打开网页首页。旁边可复制会话编号，用于核对对应会话。网页首页需要已有浏览器登录；桥不会把启动 token 放进链接。

## 故障处理

- 网页未启动、启动日志过期或认证失败：先按正常方式打开 DSH，确认日志文件是当前启动日志，再检查桥配置。桥不会重启 DSH。
- 会话忙：等网页当前运行结束，再从 Multica 发起下一次运行。网页里的插话可以在运行期间进行。
- 投递回执丢失：桥先观察原消息编号对应的事件，确认已接收后继续跟踪，不自动重发消息。连接可用但 30 秒内仍没有接收记录时，才报告投递状态不明，需到 DSH 网页核对。
- 项目路径不匹配：确认 Multica 的工作目录仍然存在，且继续使用原会话；桥拒绝把其它项目的会话接进来。
- 事件接口不兼容：更新桥或使用支持当前会话事件接口的 DSH 版本。桥在投递前检查接口，不会回退到读取内部文件。协议发生破坏性变化时仍需适配，不能保证任意升级都兼容。
- 事件连接中断或历史查询暂时超时：桥按原会话重新查询事件快照，补齐记录并继续等待，重试间隔最多 5 秒。任务的取消和总超时仍然有效；实际完成或失败以原消息对应轮次的结束事件为准。
- 启动阶段超时：项目定位、指定编号的会话创建和标题设置会沿用相同参数重试，避免初始化稍慢就直接退出。重试仍支持取消；任务提示词另走投递确认，不随启动重试重复发送。
- 历史记录改变或项目归属不符：桥停止接收并报错，避免把其它运行的结果关联到当前任务。
- 修改 DSH 网页端口：同步更新 `dsh-web.json` 和 `.env` 中的网页地址，并重启后端。

### 恢复历史结果

先从 `multica issue runs <任务编号> --output json` 取得原 run UUID。在仓库根目录执行以下命令，只读取原会话，不重新投递消息：

```powershell
node scripts/recover-dsh-run.mjs --issue CLAWSTA-57 --run <原run的UUID>
```

确认结果后，加 `--post` 可通过 Multica CLI 把结果补为原任务的评论。脚本使用当前 CLI 身份，正文以 UTF-8 文件传递；不会修改历史 run 的状态。运行仍在进行、消息编号不符或原工作目录不存在时，不会发评论。发评论失败时正文文件会保留，应先检查任务评论再决定是否重试。

桥命令也支持单独读取，返回 JSON：

```powershell
./server/bin/multica-dsh-web.exe --recover --request-id <原run的UUID> --session-id <原DSH会话编号> --cwd <原工作目录>
```

## 验证

```powershell
pnpm test:dsh-web
./server/bin/multica-dsh-web.exe --profile multica --probe
```

第一条使用模拟 HTTP/WebSocket 网页主机，不调用真实模型。第二条验证本机网页认证和事件接口，通过查询不存在的探测会话检查接口，不创建工作区、会话或投递消息。

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
