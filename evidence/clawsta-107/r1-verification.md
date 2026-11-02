# CLAWSTA-107 R1 — review-response verification record

Date: 2026-11-02. Executor: DSH (agent \`e6510524-b66d-43f5-92ad-a151f6774da3\`). Reviewer: 后端架构师.
Branch: \`clawsta-107-stacked\` (draft PR #18, stacked on PR #17 branch \`agent/dsh/21bf1ab1a33c\`).
No merge, no release, no real deployment was repeated; the machine's existing Multica service and CLI state were not touched.

## Delivered ranges

- PR #17 (base \`main\`): \`d572cfea7a611199c9da6f921e6f38c09edeecdd..9816168b9af081e17649169f8e074a68e3fd15a5\`
  - \`b68985107\` fix(connect): R1-3 — refuse a root CA on a plain-HTTP URL, honour the saved CA on reconnect, name the real fallback flag.
  - \`9816168b9\` fix(installer): R1-4 — keep a stable uninstaller inside the installation, register \`--install-dir\`, uninstall only this installation.
- PR #18 (base = PR #17 branch, draft): \`adf07e4e48f9b2097b488121775043134376f45e..HEAD\`
  - \`6ddbdd17b\` merge of the PR #17 fixes into the stacked branch (stack kept, no force-push, no rewrite of \`201e8f12b\`/\`51591de29\`).
  - \`0553aaa23\`, \`61d01b909\`, \`b1a76180d\`, \`15ae2d4b9\`, \`1ef1b7932\` — CLAWSTA-108 commits reused through \`git cherry-pick -x\` (see mapping below).
  - \`5e7683c08\` fix(deploy): R1-1 preflight/staging and R1-2 profile ownership and uninstaller boundary.
  - \`d6b95b35d\` ci: Windows runs the new adoption/profile regressions.
  - \`892e68913\` test(deploy): isolated real-deployment adoption acceptance + wording corrections (last code/test commit).
  - evidence commit on top (this file) only adds the verification record; the isolated package below was built from \`892e68913\`.

### CLAWSTA-108 commits reused (original → this branch)

| CLAWSTA-108 commit | reused as | content |
| --- | --- | --- |
| \`cdca3c6a6\` | \`0553aaa23\` | stage the portable runtime before adopting an old deployment |
| \`7f93050f3\` | \`61d01b909\` | build the CLI/installer with the supplied Go module cache |
| \`7ab3da40a\` | \`b1a76180d\` | read command exit codes on Windows PowerShell 5.1 |
| \`141a909be\` | \`15ae2d4b9\` | run commands through a generated batch file so 5.1 reports exit codes |
| \`9ce6dbdc3\` | \`1ef1b7932\` | never let Caddy block on the automatic root-trust prompt (\`skip_install_trust\`) |

All five applied cleanly on top of the preserved commits; the only overlap with the newer branch content was resolved by the merge commit. No second copy of the same fixes was written.

## R1-1 / P1 — first adoption of an old deployment (closed)

Code: \`deploy/portable/install.ps1\` (\`Assert-PortableAdoptionLayout\`, \`Assert-PortableAdoptionPreflight\`, \`Invoke-PortableAdoption\`), \`deploy/portable/portable-common.ps1\` (\`Get-PortablePostgresMajorVersion\`).

- layout refuses \`-AdoptRoot\` equal to, inside, or containing the new package folder;
- a new package folder that already holds \`deployment.json\`, \`data/postgres\`, \`releases\` or an install report is refused;
- the bundled runtime is staged **before** the old service is started/stopped, so Prepare cannot fail with "The server script is missing" after the shutdown;
- the old cluster's \`PG_VERSION\` is compared with the packaged PostgreSQL major version **before** any copy, and the copied cluster is only used when they match;
- target ports are checked before the shutdown (ports the old deployment itself uses are allowed to be reused);
- backup happens before the stop, and any failure after the stop reports the retained backup and the untouched old folder.

Verification:

- \`powershell.exe -NoProfile -ExecutionPolicy Bypass -File deploy/portable/test/adoption.test.ps1\` (PS 5.1) and the same with \`pwsh\` (PS 7): **passed** — preflight failures (nested/same folder, dirty target, PostgreSQL 16 vs 17, busy port) run before any start/stop and leave the packaged runtime untouched; the success case asserts the runtime exists at Prepare time, backup-before-stop order, copied cluster/uploads, preserved cluster credentials and JWT secret, pinned loopback listener/port, reported backup, and untouched old deployment.
- Real isolated acceptance: \`deploy/portable/test/adoption-acceptance.ps1\` installed a real native deployment from the pre-R1 package (PostgreSQL, API, web, HTTPS, local account, workspace, issue, upload, generated JWT secret), then adopted it into a fresh copy of the fixed package with different ports: **ADOPTION ACCEPTANCE PASSED (32 checks, 180.1 s)**. The issue and workspace created before the adoption still exist, the old account still signs in, the JWT and database credentials are preserved, the backup contains \`database.dump\`, the old folder is untouched, the old ports are closed, the new ports serve HTTPS; the scratch area, start-on-login item and processes were removed afterwards.

## R1-2 / P1 — uninstaller could delete other Multica configuration (closed)

Code: \`deploy/portable/uninstall.ps1\`, \`deploy/portable/portable-common.ps1\` (\`Assert-PortableProfileName\`, \`Get-PortableOwnedProfileDirectories\`, \`Register-PortableProfileOwnership\`), \`deploy/portable/install.ps1\` (ownership marker at install time).

- the profile name is validated before the first stop or delete: one plain segment starting with a letter/digit, so \`.\`, \`..\`, leading dots, separators and empty names are refused;
- install records \`uworkers-portable-profile.json\` in the profile folder, and only when the profile's saved \`server_url\` is this deployment's URL, so a same-named profile of another product in the other CLI root is never claimed;
- uninstall deletes only directories carrying a marker for this package folder and profile; unmarked directories are reported and left alone;
- \`-PurgeData\` keeps its existing "inside the package folder" guard.

Verification: \`deploy/portable/test/profile-ownership.test.ps1\` under PS 5.1 and PS 7: **passed** — escape names are rejected without side effects, foreign same-name profile survives, a marker from another package folder does not transfer ownership, a real \`uninstall.ps1\` run with \`".."\` in its install report is refused before it stops anything, and validation precedes the first stop in the script source.

## R1-3 / P1+P2 — certificate verification versus saved trust (closed)

Code: \`server/cmd/multica/cmd_connect.go\`, \`cmd_connect_trust_windows.go\`.

- \`verifyServerTrustsCA\` now refuses a non-HTTPS server URL with an actionable error instead of treating \`/health\` 200 as proof; the CA is never persisted or imported in that case;
- \`verifyServerReachable\` now uses \`tlsx.HTTPTransport()\`, i.e. the CA saved by an earlier connect run (and the process trust pool), instead of the system pool only — a reconnect after \`--skip-browser-trust\` works;
- the timeout hint names \`--skip-browser-trust\`.

Verification (Windows): \`cd server && go test ./cmd/multica -run 'TestReview107|TestConnect|TestSameOrigin|TestDaemonRegisteredRuntimes' -count=1\` → **ok**; both original failures (\`TestReview107RejectCAOverHTTP\`, \`TestReview107ReconnectUsesSavedTrust\`) are now part of the repository suite. \`go vet ./cmd/multica ./cmd/u-workers-setup\` clean.

## R1-4 / P2 — registered uninstall entry (closed)

Code: \`server/cmd/u-workers-setup/install.go\`.

- install keeps a stable copy of the uninstaller at \`<install-dir>\.uninstall\u-workers-setup.exe\` and registers \`"<install-dir>\.uninstall\u-workers-setup.exe" --uninstall --install-dir "<install-dir>"\`;
- uninstall removes the binaries, the copied uninstaller, the exact PATH entry and the Add/Remove Programs entry; when the running process *is* the copy, a detached shell removes it and the leftover folders after exit; other installations are not touched.

Verification (Windows): \`cd server && go test ./cmd/u-workers-setup -count=1\` → **ok**; the original failure (\`TestReview107UninstallerRetainsTarget\`) plus uninstall-removes-own-directory and leaves-other-installation-alone regressions pass; the previous install/uninstall contract test was updated for the new stable entry.

## Final package source consistency (closed)

Isolated build (no live deployment touched): \`C:\UWorkers\r1-build\UWorkers-Portable-v0.2.0\`, built by \`deploy/portable/build-package.ps1\`.

- manifest: version \`v0.2.0\`, commit \`892e68913\`, buildKit \`true\`;
- \`payload/source/source.zip\` is byte-identical to \`git archive --format=zip 892e68913\` (SHA-256 \`1799D3D87FC103558473B6B3224EE92BC9187FE9ED1360407409E6B17AC26560\`);
- no tracked source file was modified at build time beyond the generated \`apps/web/next-env.d.ts\`;
- the package's own \`build-installer.ps1\` rebuilt the installer with the network disabled (dead proxy, \`GOPROXY=off\`): the rebuilt installer reports \`u-workers-setup v0.2.0\`, the packaged CLI reports \`u-workers v0.2.0\`.

Verification: \`deploy/portable/test/verify-package-source.ps1 -Package C:\UWorkers\r1-build\UWorkers-Portable-v0.2.0 -ExpectedCommit 892e68913\` → **PACKAGE SOURCE CONSISTENCY PASSED (14 checks)**.

## Other verification run this batch

- \`cd server && go test ./internal/tlsx ./internal/wintrust -count=1\` → ok.
- \`powershell.exe ... portable-common.test.ps1\`, \`adoption.test.ps1\`, \`profile-ownership.test.ps1\` under PS 5.1 → passed; the two new suites also under PS 7 (pwsh) → passed.
- CI: the new regressions are wired into the Windows job in \`.github/workflows/ci.yml\`.

## Remaining items / notes for the reviewer

1. **The desktop's real CLAWSTA-81 deployment was not adopted here.** R1-1 was verified on isolated real deployments; the desktop step remains a desktop operation (108 has already delivered the desktop usability and data-retention results the task book references). No real data or firewall was touched.
2. \`clawsta-107-stacked\` is based on \`agent/dsh/21bf1ab1a33c\`; the workflow only triggers on \`main\`, so no CI record exists for the stacked branch. Manual targeted suites above are the evidence; no green check is claimed.
3. PR #17's existing backend CI failures (listed in the task book) are in modules this batch did not modify and were not re-run wholesale; the targeted packages above are green. The baseline/regression split remains for the reviewer.
4. The acceptance runs needed a child process with a deduplicated environment on this laptop because the harness session exports both \`NO_PROXY\`/\`no_proxy\` (and HTTP(S)_PROXY pairs); that is a test-machine environment quirk, not a package change — package scripts run under the normal Windows PowerShell 5.1/7 environment. The package's scripts were not changed for it.
5. R1-1 adoption requires a freshly extracted package folder; a half-prepared folder is refused by design (delete/re-extract before retrying). Backup always exists before that point.
6. Still no merge/release: PR #17 and #18 stay draft until professional review and CLAWSTA-108 acceptance are complete.

## 2026-11-02 复审后补修（三处边界 + CI 基线归因）

复审基线：PR #17 \`9816168b9\` → \`9a5c1c940\`；PR #18 \`f9f3df490\` →（本轮）\`01f585d2a\` 后接本证据提交。

### R1-1 目标数据边界（关闭）

\`deploy/portable/install.ps1\` 的 \`Assert-PortableAdoptionPreflight\` 在旧部署 Start/Backup/Stop 之前，除原有 \`deployment.json\`/\`data/install-report.json\`/\`data/postgres\`/\`releases\` 外，还拒绝目标目录里已存在的业务内容：\`uploads\`、\`data\`、\`backups\`、\`https\`、\`state\`、\`logs\` 任意非空即拒绝；空目录仍是合法的新解压布局。

证据：
- \`review107-r1-boundaries.ps1\`（复审附件）：\`R1-1 target with existing uploads rejected: True (expected True)\`。
- \`adoption.test.ps1\` 新增用例：目标含 \`uploads/sentinel.txt\` 时在 Start/Backup/Stop 之前拒绝，哨兵内容不变、包内 runtime 未被消费；目标含 \`data/leftover.txt\` 同样拒绝；空的 \`uploads\` 目录通过预检。PS 5.1 与 PS 7 均通过。

### R1-2 profile 归属（关闭）

\`portable-common.ps1\`：新增 \`Get-PortableCliConfigRoot\`，按 \`server/internal/brand.ConfigRoot\` 的同一规则确定 CLI 实际使用的唯一配置根（\`~/.u-workers\` 存在 config.json/profiles 时用它，否则已有 \`~/.multica\`，新机用 \`~/.u-workers\`）。\`Register-PortableProfileOwnership\` 只在 \`ConfigRoots\` 的第一个（即实际根）\`profiles/<name>\` 写标记，标记记录 \`configRoot\`；\`Get-PortableOwnedProfileDirectories\` 只扫描该实际根，并要求标记的 \`configRoot\` 与该根一致（上一版无 \`configRoot\` 的旧标记只在实际根内接受，保证升级路径）。\`uninstall.ps1\` 增加 \`-ConfigRoots\` 测试缝，卸载控制流可在临时 HOME 中真实运行。

证据：
- \`review107-r1-boundaries.ps1\`：两个配置根同名同 URL → \`marked=1, removable=1 (expected only selected root: 1)\`。
- \`profile-ownership.test.ps1\` 重写：两配置根同名同 URL 时只认领 CLI 实际根（另一个根不被标记）；指向另一根的标记不可删；在临时 HOME 中真实执行 \`uninstall.ps1\`（keep-data 与 \`-PurgeData\` 两种模式）：只删本包实际根中的 profile，其他 profile、另一个根的默认配置/同名 profile 哨兵、包外文件均保持不变；\`-PurgeData\` 仍只清包目录内的 data/uploads/backups 等。PS 5.1 与 PS 7 均通过。

### R1-4 多安装注册项（关闭）

PR #17 \`9a5c1c940\`：每个安装使用独立卸载注册键 \`UWorkersCLI-<sha256(安装目录)[前12位]>\`；卸载只删自己键，旧共享键 \`UWorkersCLI\` 仅当其 InstallLocation 指向被卸载目录时清理。附件失败用例 \`TestReview107R1PreservesOtherRegistration\` 已入库并通过：安装 A、安装 B（均含空格路径）、卸载 A 后 B 的注册项、二进制、PATH 项保持不变。\`go test ./cmd/u-workers-setup -count=1\` 全绿，\`go vet\` 干净。

### PR #17 历史 CI 失败的最小基线归因（补齐）

命令（同一环境、同一 -run 集合）：

\`\`\`
cd <checkout>/server && go test -run '^(TestEveryConcurrentUpBuildHasCleanup|TestSendCodeAndVerify|TestVerifyCodeNewUserHasNoWorkspace|TestLoadConfig_SkipsMulticaHooks.*|TestGetConfigExposesFrontendFeatureFlags|TestConfigDisablesUpstreamReportingAndBilling)$' ./cmd/migrate ./cmd/server ./internal/daemon ./internal/handler ./internal/handler/configpolicy -count=1
\`\`\`

基线 \`d572cfea7\`（独立 worktree \`C:\UWorkers\r1-baseline\`）与 head \`01f585d2a\` 结果逐项一致：

| 包/用例 | 基线 d572cfea7 | head 01f585d2a | 归因 |
| --- | --- | --- | --- |
| cmd/migrate TestEveryConcurrentUpBuildHasCleanup | FAIL（574/575/579/580 四个并发索引缺少 up 清理） | FAIL（同 4 项） | 基线既有失败；该目录在 \`d572cfea7..HEAD\` 无差异 |
| cmd/server TestSendCodeAndVerify | FAIL（send-code 500） | FAIL（同） | 本机测试数据库未迁移/无邮件服务，环境性；该目录无差异 |
| cmd/server TestVerifyCodeNewUserHasNoWorkspace | FAIL（DB 无 code 行） | FAIL（同） | 同上 |
| internal/handler（TestGetConfigExposesFrontendFeatureFlags 等） | 包初始化失败：\`relation "instance_setting" does not exist\` | 同 | 本机测试数据库未迁移，环境性；该目录无差异 |
| internal/handler/configpolicy TestConfigDisablesUpstreamReportingAndBilling | panic（DB 为 nil，同一缺表根因） | 同 | 同上；该目录无差异 |
| internal/daemon TestLoadConfig_SkipsMulticaHooks* 等 | ok（0.665s） | ok（0.475s） | #17 的 CA 加载改动没有回归，两版本均通过 |

diff 证明：\`git diff --stat d572cfea7..HEAD -- server/cmd/migrate server/cmd/server server/internal/handler server/internal/handler/configpolicy\` 为空；\`server/internal/daemon/config.go\`/测试在本批无差异。结论：上述 5 组失败不是本批引入，属于基线/本机数据库环境；daemon 的 config 相关用例在两侧均通过。当前两个 PR head 均无 CI check 记录，不宣称绿色 CI。

### 本轮最终包源码一致性（关闭）

隔离输出：\`C:\UWorkers\r1b-build\UWorkers-Portable-v0.2.0\`（\`build-package.ps1\`，未覆盖任何现用部署）。

- manifest：\`v0.2.0\` / build commit \`01f585d2a\` / buildKit \`true\`；
- \`payload/source/source.zip\` 与 \`git archive 01f585d2a\` 逐字节一致：SHA-256 \`145CC10C20A6555A55FAE68F290BF87192A37E0C3DA378B0B6BC24DDDE800348\`；
- 断网下用包内工具链重建安装器成功，重打安装器与包内 CLI 均报告 \`v0.2.0\`；
- \`verify-package-source.ps1 -Package C:\UWorkers\r1b-build\UWorkers-Portable-v0.2.0 -ExpectedCommit 01f585d2a\` → **PACKAGE SOURCE CONSISTENCY PASSED (14 checks)**。

说明：容器压缩字节不比较；本轮以 manifest 提交、git archive 内容一致及离线重建结果为准。证据提交位于该构建提交之后，只改本文件。

### 本轮回归结果汇总

- PS 5.1：\`portable-common.test.ps1\`、\`adoption.test.ps1\`、\`profile-ownership.test.ps1\` 全过；复审边界夹具与命令包装夹具（\`review107-command.ps1\`，未改动）通过。
- PS 7：上述三套脚本全过。
- Go（Windows）：\`go test ./cmd/u-workers-setup ./internal/tlsx ./internal/wintrust -count=1\` 全 ok；\`go test ./cmd/multica -run 'TestReview107|TestConnect' -count=1\` ok；\`go vet ./cmd/multica ./cmd/u-workers-setup\` 干净。
- 未重跑无关的整套后端测试；未重复 108 台式机部署、未触碰真实数据/防火墙/信任库；两个 PR 保持 draft。
