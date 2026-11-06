# Go 全量失败逐项对比

失败列表保留父/子用例，父项不重复算作独立缺陷。head 是 627b81a38 的完整 Go 输出；两边均运行 95 个包，新增失败为 0。完整诊断在 `failure-details.json`；每个失败的包和用例均可在 gzip 原输出中定位。

| 包 / 用例 | base 全量 | head 全量 | 原因 | 处理 |
| --- | --- | --- | --- | --- |
| `cmd/migrate::TestAgentTaskHistoryIndexMigrationAndPagePlans` | fail | pass | SQL 读取断言只匹配 LF；已兼容 CRLF。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `cmd/migrate::TestGitHubPRAddressIndexMigrationUpDownUp` | fail | pass | 首次在大数据夹具导入时超过 90 秒；独立三次重跑均通过。 | 偶发超时，保留原输出及 3 次复跑输出 |
| `cmd/migrate::TestEveryConcurrentUpBuildHasCleanup` | fail | fail | 已发布迁移 574、575、579、580 缺少 concurrentIndexCleanups 注册；与本批迁移无关。 | 原有迁移重试行为，交回复审 |
| `cmd/multica::TestRunAttachmentDownloadWritesBasenameIntoOutputDir` | fail | pass | 原断言用未转义的 Windows 路径匹配 JSON；已解码 JSON 后比较。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `cmd/multica::TestRunRuntimeProfileSetAndUnsetPath` | fail | pass | 原夹具用 /opt/... 作为绝对路径；已改用本机临时绝对路径。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `cmd/multica::TestRunRuntimeProfileSetPathPreservesExistingConfig` | fail | pass | 原夹具用 /opt/... 作为绝对路径；已改用本机临时绝对路径。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `cmd/multica::TestGuardLocalPathLinksOnlyFiresInAgentContext/agent_task_context_hard-fails` | fail | pass | 原断言未处理错误消息中 %q 对 Windows 路径的转义；已按引号语义检查。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `cmd/multica::TestGuardLocalPathLinksOnlyFiresInAgentContext` | fail | pass | 父用例汇总失败；具体原因见下列 1 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `internal/cli::TestCLIConfig_TaskRootOverridesOwnerHome` | fail | pass | Windows 不提供 POSIX 0700/0600 权限位；保留内容与隔离断言，权限位仅在 POSIX 检查。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestRefreshHealedVersion_KeepsHealedPairCurrentAfterInPlaceUpgrade` | fail | pass | POSIX 无扩展名假程序在 Windows 不可发现；已改为测试自建 .cmd，避免回退到机器已安装 CLI。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestLoadConfig_UsesCodexDesktopAppBundleFallback` | fail | skip | macOS app bundle 的可执行位夹具不适用于 Windows；已明确平台跳过。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestLoadConfig_UsesChatGPTAppBundleCodexPath` | fail | skip | macOS app bundle 的可执行位夹具不适用于 Windows；已明确平台跳过。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestLoadConfig_UsesNestedChatGPTCodexCLIPath` | fail | skip | macOS app bundle 的可执行位夹具不适用于 Windows；已明确平台跳过。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestLoadConfig_PrefersNestedChatGPTCodexCLIPath` | fail | skip | macOS app bundle 的可执行位夹具不适用于 Windows；已明确平台跳过。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon/execenv::TestPrepareCodexHomeFailsClosedWhenSandboxWriteFails` | fail | fail | 用例用 POSIX 权限位或 chmod 模拟拒读/拒写/拒删；Windows 合成权限及 ACL 行为不同。 | 需要 Windows 对应的权限/错误注入契约，保留失败交回复审 |
| `internal/daemon::TestDshPluginPathDirs_FindsBothLayouts/flat` | fail | pass | APPDATA 未隔离，两个子用例共用真实目录并串扰；已为各子用例使用临时 APPDATA。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestDshPluginPathDirs_FindsBothLayouts` | fail | pass | 父用例汇总失败；具体原因见下列 1 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `internal/daemon/execenv::TestReuseRestoresCodexHome` | fail | fail | 用例用 POSIX 权限位或 chmod 模拟拒读/拒写/拒删；Windows 合成权限及 ACL 行为不同。 | 需要 Windows 对应的权限/错误注入契约，保留失败交回复审 |
| `internal/daemon/execenv::TestPrepareHermesHomeLinkFailureKeepsTaskLocalDB` | fail | fail | 本机 Windows 无创建符号链接的权限，原用例要求共享链接或 POSIX 链接行为。 | 需明确 Windows 链接/复制契约；本批不改权限或运行行为，交回复审 |
| `internal/daemon/execenv::TestPrepareHermesHomeMigrationIsAtomic` | fail | fail | 本机 Windows 无创建符号链接的权限，原用例要求共享链接或 POSIX 链接行为。 | 需明确 Windows 链接/复制契约；本批不改权限或运行行为，交回复审 |
| `internal/daemon/execenv::TestPrepareWorktreeModeUsesPerIssueCodexSessionStore` | fail | fail | 本机 Windows 无创建符号链接的权限，原用例要求共享链接或 POSIX 链接行为。 | 需明确 Windows 链接/复制契约；本批不改权限或运行行为，交回复审 |
| `internal/daemon/execenv::TestOpenclawDiscoveryCacheHitProducesSameWrapper` | fail | pass | JSON 内的工作目录已转义，原归一化未命中 Windows 路径；已先编码路径再归一化。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestLockReusablePriorEnvRootRejectsIdentitySwap` | fail | fail | Windows 目录替换后，原有目录身份检查仍接受该复用根；基线同样复现。 | 原有目录身份/锁契约，交回复审 |
| `internal/daemon::TestLockReusablePriorEnvRootSurvivesWorkspacesRootSwap` | fail | fail | Windows 对打开的目录/文件句柄施加锁，原实现的 rename/remove 在该时刻被拒绝。 | 原有 Windows 文件/锁行为，交回复审 |
| `internal/daemon::TestRunTaskDeclinesReuseWhenTheClaimedDirectoryIsSwappedBeforeUse` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `internal/daemon::TestListRuntimeLocalSkills_ClaudeEnabledPlugin` | fail | pass | 插件清单夹具的 installPath 未做 JSON 转义；已按字符串编码。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon/execenv::TestOpenclawDiscoveryCacheConcurrentPreparations` | fail | fail | Windows 对打开的目录/文件句柄施加锁，原实现的 rename/remove 在该时刻被拒绝。 | 原有 Windows 文件/锁行为，交回复审 |
| `internal/daemon::TestListRuntimeLocalSkills_FollowsSymlinkedSkillDirs` | fail | fail | 本机 Windows 无创建符号链接的权限，原用例要求共享链接或 POSIX 链接行为。 | 需明确 Windows 链接/复制契约；本批不改权限或运行行为，交回复审 |
| `internal/daemon::TestListRuntimeLocalSkills_CodexUsesSharedCODEXHOME` | fail | pass | 接口 source_path 使用正斜杠，原断言写死本机反斜杠；已按接口规范归一化期望。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestListRuntimeLocalSkills_PerRootVisitedAllowsCrossRootSymlinkAlias` | fail | fail | 本机 Windows 无创建符号链接的权限，原用例要求共享链接或 POSIX 链接行为。 | 需明确 Windows 链接/复制契约；本批不改权限或运行行为，交回复审 |
| `internal/daemon/execenv::TestPrepareOpenclawConfigWorstCaseCLIBudgets` | fail | fail | 最坏路径计数为 2 个 CLI 截止时间，原断言要求 3；registry 与 schema 回退预算的既有契约不一致。 | 原有预算行为/契约，交回复审 |
| `internal/daemon/execenv::TestWindowsOpenclawShimMissingNodeSurfacesCmdStderr` | fail | pass | 原断言要求英文 cmd.exe；已检查 stderr 分支与缺失的 node，兼容中文系统。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestHandleRuntimeGone_DistinctDeletionsWithinCoalesceWindowBothRecover` | fail | fail | 连续删除不同运行环境仍只注册一次；基线独立三次均复现，属于原有合并窗口行为。 | 原有产品行为，交回复审 |
| `internal/daemon::TestListRuntimeLocalMcpServersClaudeEnabledPlugin` | fail | pass | 插件清单夹具的 installPath 未做 JSON 转义；已按字符串编码。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestMergeRuntimeAndAgentMcpConfigClaudeCombinesAndAgentWins` | fail | pass | 插件清单夹具的 installPath 未做 JSON 转义；已按字符串编码。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestDetectBuiltinRuntimes_ProbesRunConcurrently` | fail | fail | 原有探针用例要求 6 项运行时，本平台返回 5 项；独立三次均复现。 | 原有运行时发现与平台夹具契约，交回复审 |
| `internal/daemon/execenv::TestCommentReplySnippetPropagatesPostFailure/posix/sh/successful_post_still_cleans_up` | fail | pass | Windows 夹具只提供 .cmd，Git Bash 未命中该假 CLI；已同时提供扩展名为空的 sh 假程序，两个 shell 都使用测试进程。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon/execenv::TestCommentReplySnippetPropagatesPostFailure/posix/sh` | fail | pass | 父用例汇总失败；具体原因见下列 1 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `internal/daemon::TestRepoCheckoutUsesTaskScopedProjectRefByDefault` | fail | pass | 请求夹具拼接 Windows 路径时生成无效 JSON；已对路径编码。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestPruneWorktree_LegacyCacheFallbackSerializesWithCreateWorktree` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/execenv::TestCommentReplySnippetPropagatesPostFailure/windows/git-bash/successful_post_still_cleans_up` | fail | pass | Windows 夹具只提供 .cmd，Git Bash 未命中该假 CLI；已同时提供扩展名为空的 sh 假程序，两个 shell 都使用测试进程。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon/execenv::TestCommentReplySnippetPropagatesPostFailure/windows/git-bash` | fail | pass | 父用例汇总失败；具体原因见下列 1 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `internal/daemon/execenv::TestCommentReplySnippetPropagatesPostFailure` | fail | pass | 父用例汇总失败；具体原因见下列 4 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `internal/daemon/execenv::TestConflictResolvedByTheAgentIsDeliveredAndNotReplayedAgain` | fail | pass | 宿主 Git autocrlf 改写测试文件字节；测试仓库已固定属性及 autocrlf，不改宿主 Git 配置。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestEnsureRepoReadyConcurrentMissRefreshesOnce` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/execenv::TestFinalizeRefusesWhenAFollowUpResetsPastTheUserEditsItReplayed` | fail | pass | 宿主 Git autocrlf 改写测试文件字节；测试仓库已固定属性及 autocrlf，不改宿主 Git 配置。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestEnsureRepoReadyRefreshesOnMiss` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon::TestRegisterTaskReposAllowsProjectOnlyURL` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon::TestEnsureRepoReadyCachedRepoStillRefreshesSettings` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon::TestRunTaskCancelledWaitingForThePriorWorkdirStopsInsteadOfPreparing` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `internal/daemon::TestShouldReusePriorWorkdirSquadLeaderRejectsSymlinkEscape` | fail | fail | 本机 Windows 无创建符号链接的权限，原用例要求共享链接或 POSIX 链接行为。 | 需明确 Windows 链接/复制契约；本批不改权限或运行行为，交回复审 |
| `internal/daemon::TestRunTaskReusesPriorWorkdirForFreshSession` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `internal/daemon::TestRunTaskSquadLeaderReusesWorkdirBeforeGCMetaWritten` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `internal/daemon::TestRepoCheckoutReturnsRetryableBusyToCapableClient` | fail | pass | 请求夹具拼接 Windows 路径时生成无效 JSON；已对路径编码。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestRepoCheckoutForwardsFresh` | fail | pass | 请求夹具拼接 Windows 路径时生成无效 JSON；已对路径编码。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestRepoCheckoutForwardsIsolatedMode` | fail | pass | 请求夹具拼接 Windows 路径时生成无效 JSON；已对路径编码。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestRepoCheckoutExplicitRefOverridesProjectDefault` | fail | pass | 请求夹具拼接 Windows 路径时生成无效 JSON；已对路径编码。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestRepoCheckoutRejectsUnresolvableWorkdirWithTheReason` | fail | pass | 请求夹具拼接 Windows 路径时生成无效 JSON；已对路径编码。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestRepoCheckoutRejectsAnotherTaskWorkdir` | fail | pass | 请求夹具拼接 Windows 路径时生成无效 JSON；已对路径编码。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon::TestRunTaskSquadLeaderDoesNotReuseExternalPriorWorkdir` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `internal/daemon/execenv::TestPrepareQwenpawWorkspacePermissions` | fail | fail | 用例用 POSIX 权限位或 chmod 模拟拒读/拒写/拒删；Windows 合成权限及 ACL 行为不同。 | 需要 Windows 对应的权限/错误注入契约，保留失败交回复审 |
| `internal/daemon/repocache::TestCheckoutIdentityIgnoresSharedCache/linked` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCheckoutIdentityIgnoresSharedCache/isolated` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCheckoutIdentityIgnoresSharedCache` | fail | fail | 父用例汇总失败；具体原因见下列 2 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `internal/daemon/repocache::TestCheckoutIdentityConcurrentTasksAndOverrides` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCheckoutIdentityMigratesExistingWorktree/worktreeConfig=false` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCheckoutIdentityMigratesExistingWorktree/worktreeConfig=true` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCheckoutIdentityMigratesExistingWorktree` | fail | fail | 父用例汇总失败；具体原因见下列 2 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `internal/daemon/repocache::TestCheckoutIdentityUserConfigAndCoauthor` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCheckoutIdentityWithoutUserFailsClosed` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestIsolatedCheckoutPreservesIntentionalIdentity` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCheckoutIdentityConfigLockFailureIsRetryable` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCheckoutIdentityRejectsForeignWorktree` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCheckoutIdentityEnabledWithSharedConfigLocked` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/execenv::TestPrepareLocalWorktreeReplaysOnlyTheUserEditsSinceTheLastTurn` | fail | pass | 宿主 Git autocrlf 改写测试文件字节；测试仓库已固定属性及 autocrlf，不改宿主 Git 配置。 | 测试设施修复；以 head 全量/补充专项为准 |
| `internal/daemon/repocache::TestGetRemoteDefaultBranchUsesBareHeadHintForCustomDefault` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestReconcileCoAuthoredByHooksUpgradesReleasedHookInPlace` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestIsPartialClone` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateWorktreeFromPartialCacheHasFileContents` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestReusedIsolatedCheckoutRepairsPromisorConfig` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateIsolatedCheckoutFromPartialCacheHasFileContents` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestIsolatedCheckoutKeepsLinkedWorktreeHoldingWork` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestIsolatedCheckoutAcrossVolumesOnWindows` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestReconcileCoAuthoredByHookInCheckoutIgnoresLinkedWorktree` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestIsolatedCheckoutIsCommittableOnWindows` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateWorktreeDoesNotResurrectDisabledState` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestReconcileCoAuthoredByHookInCheckoutMigratesReleasedHook` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/execenv::TestFinalizeAbortRefusesToCommitAndKeepsWorktree` | fail | pass | git worktree list 输出正斜杠，原断言与本机反斜杠比较；已归一化路径，保留正反注册断言。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon/repocache::TestReconcileCoAuthoredByHooksPreservesUserHook` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateWorktreeMigratesLinkedWorktreeToIsolatedMetadata` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestGetRemoteDefaultBranchFallsBackToBareHead` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestGitFetchRefreshesOriginHeadAfterDefaultChange` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestGetRemoteDefaultBranchScansForCustomDefault` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateWorktreePathCollisionDoesNotLeakBranch` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestEnsureRemoteTrackingLayoutMigratesLegacyCache` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/execenv::TestPrepareLocalWorktreeContinuesTheConversationBranch` | fail | pass | 宿主 Git autocrlf 改写测试文件字节；测试仓库已固定属性及 autocrlf，不改宿主 Git 配置。 | 测试设施修复；以 head 全量/补充专项为准 |
| `internal/daemon/repocache::TestCreateWorktreeFetchesDespiteAgentBranchOnRemote` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateWorktreeWithUnknownRequestedRef` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateWorktreeWithRequestedTagRef` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateWorktreeWithRequestedCommitRef` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateWorktreeDoesNotExcludeReasonixProjectConfig` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateWorktreeExcludesAgentSidecars` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateWorktreeWithRequestedBranchRef` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateWorktreeExcludesOpenCodeSkills` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestIsolatedCheckoutCloneWithoutHardlinksIsIndependent` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateWorktreeReusesIsolatedGitMetadata` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateWorktreeWithIsolatedGitMetadata` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/execenv::TestPrepareLocalWorktreeSkipsMulticaSidecars` | fail | pass | 宿主 Git autocrlf 改写测试文件字节；测试仓库已固定属性及 autocrlf，不改宿主 Git 配置。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon/execenv::TestPrepareLocalWorktreePrunesStaleRegistrations` | fail | pass | git worktree list 输出正斜杠，原断言与本机反斜杠比较；已归一化路径，保留正反注册断言。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon/execenv::TestFinalizeKeepsWorktreeWhenCommitFails` | fail | pass | git worktree list 输出正斜杠，原断言与本机反斜杠比较；已归一化路径，保留正反注册断言。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon/repocache::TestSyncKeepsDistinctCachesForSegmentBoundaryColliders` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestReconcileCoAuthoredByHooksMigratesReleasedHook` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/execenv::TestPrepareLocalWorktreeSubdirectory` | fail | pass | 宿主 Git autocrlf 改写测试文件字节；测试仓库已固定属性及 autocrlf，不改宿主 Git 配置。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon/repocache::TestCreateWorktreeInstallsGatedHook` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCoAuthoredByStateStopsTrailerInExistingCheckout` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestRemoveCoAuthoredByHookPreservesUserHook` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateWorktreeStampsLastUsed` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/execenv::TestPrepareLocalWorktreeConcurrentTasks` | fail | pass | 宿主 Git autocrlf 改写测试文件字节；测试仓库已固定属性及 autocrlf，不改宿主 Git 配置。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon/repocache::TestGetRemoteDefaultBranchAmbiguousOriginReturnsEmpty` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateWorktreeRemovesLegacyCoAuthoredByHook` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateWorktreeRemovesCoAuthoredByHookWhenDisabled` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCoAuthoredByHookIdempotent` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestCreateWorktreeInstallsCoAuthoredByHook` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestFreshMigrationCarriesUnpushedBranch/another_task` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestFreshMigrationCarriesUnpushedBranch/same_task` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestFreshMigrationCarriesUnpushedBranch` | fail | fail | 父用例汇总失败；具体原因见下列 2 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `internal/daemon/repocache::TestFreshCheckoutDiscardsLocalWork/isolated` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestFreshCheckoutDiscardsLocalWork/linked` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestFreshCheckoutDiscardsLocalWork` | fail | fail | 父用例汇总失败；具体原因见下列 2 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `internal/daemon/repocache::TestRepeatedCheckoutMovesCleanCheckoutToNewBranch/linked` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestRepeatedCheckoutOnTaskBranchIsNoop/linked` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/execenv::TestPromoteHermesMemoryStagingClassifiesRemoveFailures/unremovable_empty_store_fails_closed` | fail | fail | 用例用 POSIX 权限位或 chmod 模拟拒读/拒写/拒删；Windows 合成权限及 ACL 行为不同。 | 需要 Windows 对应的权限/错误注入契约，保留失败交回复审 |
| `internal/daemon/repocache::TestRepeatedCheckoutMovesCleanCheckoutToNewBranch/isolated` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestRepeatedCheckoutMovesCleanCheckoutToNewBranch` | fail | fail | 父用例汇总失败；具体原因见下列 2 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `internal/daemon/repocache::TestRepeatedCheckoutKeepsLocalWork/linked/uncommitted_change` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/execenv::TestMigrateHermesTaskMemoriesFailureKeepsSource` | fail | fail | 用例用 POSIX 权限位或 chmod 模拟拒读/拒写/拒删；Windows 合成权限及 ACL 行为不同。 | 需要 Windows 对应的权限/错误注入契约，保留失败交回复审 |
| `internal/daemon/repocache::TestRepeatedCheckoutOnTaskBranchIsNoop/isolated` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestRepeatedCheckoutOnTaskBranchIsNoop` | fail | fail | 父用例汇总失败；具体原因见下列 2 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `internal/daemon/repocache::TestRepeatedCheckoutKeepsLocalWork/isolated/uncommitted_change` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/execenv::TestPrepareHermesHomeMemoryStorePersistsAcrossTasks` | fail | fail | 本机 Windows 无创建符号链接的权限，原用例要求共享链接或 POSIX 链接行为。 | 需明确 Windows 链接/复制契约；本批不改权限或运行行为，交回复审 |
| `internal/daemon/execenv::TestPrepareLocalWorktreeReplaysUncommittedWork` | fail | pass | 宿主 Git autocrlf 改写测试文件字节；测试仓库已固定属性及 autocrlf，不改宿主 Git 配置。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon/execenv::TestHermesOverlayEnvPinsHomeAfterDotenvOverride` | fail | fail | 用例用 POSIX 权限位或 chmod 模拟拒读/拒写/拒删；Windows 合成权限及 ACL 行为不同。 | 需要 Windows 对应的权限/错误注入契约，保留失败交回复审 |
| `internal/daemon/repocache::TestRepeatedCheckoutKeepsLocalWork/isolated/unpushed_commit` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestRepeatedCheckoutKeepsLocalWork/isolated/untracked_file` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/execenv::TestHermesOverlayPermissions` | fail | fail | 用例用 POSIX 权限位或 chmod 模拟拒读/拒写/拒删；Windows 合成权限及 ACL 行为不同。 | 需要 Windows 对应的权限/错误注入契约，保留失败交回复审 |
| `internal/daemon/repocache::TestRepeatedCheckoutKeepsLocalWork/linked/untracked_file` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestRepeatedCheckoutKeepsLocalWork/linked/unpushed_commit` | fail | fail | Windows 本地仓库路径的盘符、分隔符或长路径参与缓存/clone，原路径断言或 Git 操作失败；具体错误见本项原始诊断。 | 原有仓库路径行为，交回复审 |
| `internal/daemon/repocache::TestRepeatedCheckoutKeepsLocalWork` | fail | fail | 父用例汇总失败；具体原因见下列 6 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `internal/daemon/execenv::TestHermesExternalDirsExpandsSanitizedEnv` | fail | fail | 原用例将 /srv 或 /opt 视为绝对路径；Windows 将其解释为无盘符路径并相对 home 重定位。 | 需明确跨平台外部目录语义，原有行为交回复审 |
| `internal/daemon/execenv::TestHermesDerivedConfigRebasesRelativeExternalDirs` | fail | fail | 原用例将 /srv 或 /opt 视为绝对路径；Windows 将其解释为无盘符路径并相对 home 重定位。 | 需明确跨平台外部目录语义，原有行为交回复审 |
| `internal/daemon/execenv::TestPrepareHermesHomeOverlay` | fail | fail | 本机 Windows 无创建符号链接的权限，原用例要求共享链接或 POSIX 链接行为。 | 需明确 Windows 链接/复制契约；本批不改权限或运行行为，交回复审 |
| `internal/daemon/execenv::TestConcurrentIsolatedPreparesOnOneRepo` | fail | pass | 宿主 Git autocrlf 改写测试文件字节；测试仓库已固定属性及 autocrlf，不改宿主 Git 配置。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/daemon/execenv::TestWriteEnvRootOwnerAtomicallyReplacesMarker` | fail | fail | Windows 的文件身份/替换观测未满足原测试要求的原子替换证据。 | 原有文件身份行为，交回复审 |
| `internal/daemon/execenv::TestDirHasEntries` | fail | fail | Windows 对普通文件执行目录检查时的错误分类走入 ENOTEMPTY 分支，原测试期望 ENOTDIR。 | 原有平台错误分类，交回复审 |
| `internal/daemon/execenv::TestCleanupSidecarsSurfacesEACCESOnEmptyRecordedDir` | fail | fail | 用例用 POSIX 权限位或 chmod 模拟拒读/拒写/拒删；Windows 合成权限及 ACL 行为不同。 | 需要 Windows 对应的权限/错误注入契约，保留失败交回复审 |
| `internal/daemon/execenv::TestPrepareDirectoryMode` | fail | fail | 用例用 POSIX 权限位或 chmod 模拟拒读/拒写/拒删；Windows 合成权限及 ACL 行为不同。 | 需要 Windows 对应的权限/错误注入契约，保留失败交回复审 |
| `internal/daemon/execenv::TestVerifyCodexHomeRootRejectsSymlinkedHome` | fail | fail | 本机 Windows 无创建符号链接的权限，原用例要求共享链接或 POSIX 链接行为。 | 需明确 Windows 链接/复制契约；本批不改权限或运行行为，交回复审 |
| `internal/daemon/execenv::TestVerifyCodexHomeRootRejectsSwappedDirectory` | fail | fail | Windows 对打开的目录/文件句柄施加锁，原实现的 rename/remove 在该时刻被拒绝。 | 原有 Windows 文件/锁行为，交回复审 |
| `internal/daemon/execenv::TestPrepareCursorMcpConfigWritesProjectConfigAndApprovals` | fail | fail | 用例用 POSIX 权限位或 chmod 模拟拒读/拒写/拒删；Windows 合成权限及 ACL 行为不同。 | 需要 Windows 对应的权限/错误注入契约，保留失败交回复审 |
| `internal/daemon/execenv::TestPrepareCodexSessionsDir_LocalDirectoryUsesPerIssueStore` | fail | fail | 本机 Windows 无创建符号链接的权限，原用例要求共享链接或 POSIX 链接行为。 | 需明确 Windows 链接/复制契约；本批不改权限或运行行为，交回复审 |
| `internal/daemon/execenv::TestPromoteHermesMemoryStagingClassifiesRemoveFailures` | fail | fail | 父用例汇总失败；具体原因见下列 1 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `internal/handler::TestParseSkillArchive_RejectsUnsafeSkillMdPath` | fail | fail | Windows 对 /abs、\abs、UNC 路径的判定与原 POSIX 归档路径断言不同，未拒绝这些 SKILL.md 路径。 | 原有跨平台路径校验，交回复审 |
| `internal/integrations/wecom::TestStreamingDecryptMatchesTheBufferedOneAtEveryPadLength` | fail | fail | 解密临时文件仍打开时执行 unlink；Windows 拒绝删除，上传流因此未调用。 | 原有 Windows 文件生命周期行为，交回复审 |
| `internal/integrations/wecom::TestStreamingDecryptSurvivesADribblingReader` | fail | fail | 解密临时文件仍打开时执行 unlink；Windows 拒绝删除，上传流因此未调用。 | 原有 Windows 文件生命周期行为，交回复审 |
| `internal/integrations/wecom::TestTheTempFileIsPrivateAndUnlinked` | fail | fail | 解密临时文件仍打开时执行 unlink；Windows 拒绝删除，上传流因此未调用。 | 原有 Windows 文件生命周期行为，交回复审 |
| `internal/integrations/wecom::TestPeekFileRewinds` | fail | fail | 解密临时文件仍打开时执行 unlink；Windows 拒绝删除，上传流因此未调用。 | 原有 Windows 文件生命周期行为，交回复审 |
| `internal/integrations/wecom::TestPeekFileHandlesAShortFile` | fail | fail | 解密临时文件仍打开时执行 unlink；Windows 拒绝删除，上传流因此未调用。 | 原有 Windows 文件生命周期行为，交回复审 |
| `internal/integrations/wecom::TestIngestPrefersTheStreamingPathAndUploadsTheRightBytes` | fail | fail | 解密临时文件仍打开时执行 unlink；Windows 拒绝删除，上传流因此未调用。 | 原有 Windows 文件生命周期行为，交回复审 |
| `internal/service::TestBuiltinSkillsConformToTemplate/multica-onboarding` | fail | pass | 测试的 frontmatter 提取器只匹配 LF；已先归一化 CRLF，再检查同一份 YAML 与触发词。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/service::TestBuiltinSkillsConformToTemplate/multica-platform` | fail | pass | 测试的 frontmatter 提取器只匹配 LF；已先归一化 CRLF，再检查同一份 YAML 与触发词。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/service::TestBuiltinSkillsConformToTemplate` | fail | pass | 父用例汇总失败；具体原因见下列 2 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `internal/service::TestBuiltinSkillsFrontmatterIsStrictYAML/multica-onboarding` | fail | pass | 测试的 frontmatter 提取器只匹配 LF；已先归一化 CRLF，再检查同一份 YAML 与触发词。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/service::TestBuiltinSkillsFrontmatterIsStrictYAML/multica-platform` | fail | pass | 测试的 frontmatter 提取器只匹配 LF；已先归一化 CRLF，再检查同一份 YAML 与触发词。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/service::TestBuiltinSkillsFrontmatterIsStrictYAML` | fail | pass | 父用例汇总失败；具体原因见下列 2 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `internal/service::TestPlatformSkillDescriptionNamesEveryDomain` | fail | pass | 测试的 frontmatter 提取器只匹配 LF；已先归一化 CRLF，再检查同一份 YAML 与触发词。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `internal/maintenance::TestInternalHTTPCreateAdvanceReplayAndConfigure` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestAntigravityToolsWithSlowConsumer/cancel=false` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestAntigravityToolsWithSlowConsumer/cancel=true` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestAntigravityToolsWithSlowConsumer` | fail | fail | 父用例汇总失败；具体原因见下列 2 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `pkg/agent::TestDimBackendUsesOwnedProcessTree` | fail | fail | Windows 进程树夹具未观察到辅助程序写入的后代 PID；独立三次均复现。 | 原有进程启动/Job Object 夹具与运行契约，交回复审 |
| `pkg/agent::TestExplainExecErrorNamesPackageAndRepairCommand` | fail | pass | 原期望写死 POSIX 修复命令；Windows 实际正确输出 PowerShell 命令，已分别断言。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `pkg/agent::TestExplainExecErrorReadsRepairCommandFromManifest` | fail | pass | 原期望写死 POSIX 修复命令；Windows 实际正确输出 PowerShell 命令，已分别断言。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `pkg/agent::TestExplainExecErrorQuotesPathsForTheShell` | fail | pass | 原期望写死 POSIX 修复命令；Windows 实际正确输出 PowerShell 命令，已分别断言。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `pkg/agent::TestGrokSupplementTimesOutIndependentlyAndAllowsNextMessage` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokSupplementHandlesObservedNestedResultStatus/queued` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokSupplementHandlesObservedNestedResultStatus/rejected` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokSupplementHandlesObservedNestedResultStatus/in_band_error` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokSupplementHandlesObservedNestedResultStatus` | fail | fail | 父用例汇总失败；具体原因见下列 3 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `pkg/agent::TestGrokNoUsableAuthMethodFailsBeforeSession/none` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokNoUsableAuthMethodFailsBeforeSession/unknown` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokNoUsableAuthMethodFailsBeforeSession/api` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokNoUsableAuthMethodFailsBeforeSession` | fail | fail | 父用例汇总失败；具体原因见下列 3 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `pkg/agent::TestGrokUsesAdvertisedAPIKeyMethod` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokDrainsNotificationsAfterPromptResponse` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokPropagatesMCPAndUsage` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokCancellation` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestDiscoverGrokModelsWaitsForAdvertisedAuth` | fail | fail | 协议发现依赖的 POSIX 假 CLI 在 Windows 未启动，因而退回内置模型清单或没有生成 requests.jsonl。 | 原有协议进程夹具的 Windows 适配，交回复审 |
| `pkg/agent::TestDiscoverGrokModelsStopsOnAuthFailures/no_methods` | fail | fail | 协议发现依赖的 POSIX 假 CLI 在 Windows 未启动，因而退回内置模型清单或没有生成 requests.jsonl。 | 原有协议进程夹具的 Windows 适配，交回复审 |
| `pkg/agent::TestDiscoverGrokModelsStopsOnAuthFailures/authenticate_rejected` | fail | fail | 协议发现依赖的 POSIX 假 CLI 在 Windows 未启动，因而退回内置模型清单或没有生成 requests.jsonl。 | 原有协议进程夹具的 Windows 适配，交回复审 |
| `pkg/agent::TestDiscoverGrokModelsStopsOnAuthFailures` | fail | fail | 父用例汇总失败；具体原因见下列 2 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `pkg/agent::TestQoderBackendDeliversFinalAnswerWithoutNarration/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQoderBackendDeliversFinalAnswerWithoutNarration/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQoderBackendDeliversFinalAnswerWithoutNarration` | fail | fail | 父用例汇总失败；具体原因见下列 2 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `pkg/agent::TestQoderBackendDoesNotWaitForeverForReaderAfterPromptDone` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQoderBackendIgnoresLateReaderOutputAfterGrace` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestZeroclawSessionResumeTransientError` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestOpencodeBackendDoesNotUsePermissionEnvOverride` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestOpencodeBackendInjectsThinkingVariant` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestOpencodeBackendNeverEmitsPromptFlag` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestOpencodeBackendAnchorsDirAndPWD` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQwenpawBlockedWorkspaceAndAgentArgs` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQwenpawExtraArgsReachTheCommandLine` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQwenpawSessionLoadSendsCodingProjectDir` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQwenpawWorkspaceArgs` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQwenpawSessionNewSendsCodingProjectDir` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQwenpawSessionLoadTransientError` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQwenpawUsageModelIgnored` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQwenpawBackendUsage` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQwenpawUsesSessionLoad` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQwenpawSessionLoadNotFound` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQwenpawSessionLoad` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQwenpawSessionNew` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQoderDrainsNotificationsAfterPromptResponse` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQoderBackendClearsSessionIDWhenResumedSessionNotFoundAtSetModel` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQoderBackendClearsSessionIDWhenResumedSessionNotFoundAtPrompt` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQoderBackendAttributesUsageToACPDefaultModel` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQoderPromotesTerminalProviderErrorWithOutput` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQoderFiltersRemoteMcpWhenInitializeDoesNotAdvertiseCapability` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQoderForwardsMcpAuthHeaderToSessionNew` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQoderBackendNonPositiveTimeoutDoesNotImposeDeadline` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQoderBackendProviderErrorDetectionUsesFullOutput` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQoderBackendToolTerminatedTurnFallsBackToLatestTextBlock` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQoderBackendHappyPath` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQoderBackendInvokesACPFlagAndFiltersBlockedArgs` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestQoderBackendSetModelFailureFailsTask` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestOpencodeBackendOverridesUserOpenCodeConfigContent` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestOpencodeBackendOmitsMCPEnvWhenEmpty` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestOpencodeBackendInjectsMCPConfigViaEnv` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestOpencodeV2CancelInterruptsThroughRunConnection` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestOpencodeInterruptSessionUsesRunConnection` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestOpencodeV1ArgvKeepsDirAndVariant` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestOpencodeV2ArgvOmitsDirAndFoldsVariant` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestOpencodeV1StillDeliversMCPConfig` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestOpencodeV2ExecuteRefusesMCPConfig` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestOpencodeBackendAppendsExitDetailOnMidStepCrash` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestOpencodeBackendFailsOnStreamEndingMidTool` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestOpencodeBackendBlocksDirOverride` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestOpencodeBackendQuestionDenySurvivesUserConfig` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendOmitsThinkingConfigOnResumedSessionWhenUnset` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestMcodeLoadSessionWhenCapabilityAppears` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestMcodeUnsupportedResumeRequestsFreshRetry` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestMcodeSessionNewExitAfterInitializeReportsStartupFailure` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestMcodeWaitsForSessionNewReadinessAfterInitialize` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestMcodeFreshSessionUsesACPAndForwardsMCP` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesKeepsRemoteMcpWhenCapabilityAdvertised` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesDropsRemoteMcpWhenCapabilityExplicitlyDeclined` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesCustomProfileDropsRemoteMcpWhenCapabilityOmitted` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesForwardsRemoteMcpUnderOmittedCapabilitiesException` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesResumeIncludesMcpServers` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestClaudeBackendLaunchesWrapperSubcommandFirst` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesSendsSetModelWhenExplicitProviderAndBareCurrent` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesSkipsRedundantSetModelForBareConfiguredModel` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesSendsSetModelWhenModelDiffersFromCurrent` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesSkipsRedundantSetModelWhenAlreadyCurrent` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesSetModelPreservesCustomModelIDWithColon` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesExecuteFailsClosedOnMalformedMcpConfig` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroDrainsNotificationsAfterPromptResponse` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroFreshOversizedHistoryImageDoesNotSignalResumeRejected` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroResumedOversizedHistoryImageSignalsResumeRejected` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroLoadIncludesMcpServersFromConfig` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroBackendUsesSessionLoadForResume` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroBackendInvokesACPWithTrustAllTools` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroBackendClearsSessionIDWhenPromptSessionNotFound` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesBackendPoisonedHistorySetsResumeRejected` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroBackendClearsSessionIDWhenSetModelSessionNotFound` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesBackendDoesNotPromoteOnTransientRetry` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroBackendDoesNotCompleteAfterFailedCommentAddResult` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesBackendClearsSessionIDWhenSetModelSessionNotFound` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroBackendCompletedRetryAfterEarlierFailureIsCompleted` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesBackendClearsSessionIDWhenResumedSessionNotFound` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroBackendFailedFinalCommentOverridesEarlierSuccess` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroBackendPreservesCompletionOnRealGPT56SolFrames` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesBackendPromotesProviderErrorWithNonEmptyOutput` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroBackendTreatsCompletedCommentAddWithNonTerminalTitleAsCompleted` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroBackendDoesNotCompleteRunningGoalCompleteWithoutResult` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroBackendDoesNotCompleteRunningCommentAddWithoutResult` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroBackendTreatsCommentAddCloseErrorAsCompleted` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroBackendDoesNotCompleteAfterFailedGoalComplete` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroBackendTreatsGoalCompleteCloseErrorAsCompleted` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesBackendMergesLateCumulativeUsageAfterPromptResponse` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesBackendAttributesUsageToACPDefaultModel` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroBackendAttributesUsageToCurrentModel` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKiroBackendSetModelFailureFailsTask` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendWireTurnFailurePastOversizedRecord` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendWireTurnFailureFailsCompletedRun` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendReportsUsageFromWireLog` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendPropagatesACPPromptUsage` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendDropsHistoryReplayOnResume` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiDrainsNotificationsAfterPromptResponse` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendPoisonedHistoryTwoLineFormat` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendPoisonedHistoryFreshRunNoResumeRejected` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendPoisonedHistorySetsResumeRejected` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiFreshSessionIncludesMcpServers` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiResumeIncludesMcpServers` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendInvokesACPSubcommand` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendClearsSessionIDWhenSetModelSessionNotFound` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendSetsThinkingLevelOnResumedSession` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendOmitsThinkingConfigWhenUnset` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendThinkingFailureWarnsAndContinues` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendSetsThinkingLevelBeforePrompt` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendSetModelFailureFailsTask` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesBackendIgnoresRefusalOnFreshSession` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesBackendKeepsSessionOnRefusalWithLateActivity` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesBackendKeepsSessionOnRefusalAfterActivity` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesBackendKeepsProviderErrorOnRefusedResume` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHermesBackendRecoversFromRefusedResume` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokAttributesUsageOnResumeWithoutConfiguredModel` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokAuthFailureFailsTask` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokUsesSessionLoadForResume` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokSetModelFailureFailsTask` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokBlockedArgsFiltering` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokBackendStreamsAndCompletes` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokSupplementReportsACPRejection` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokSupplementTargetsActivePrompt` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestDimSessionLoadRetryCancelled` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestDimSessionLoadRetryExhausted` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestDimSessionLoadRetrySucceeds` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestDimSetModelFailClosesSession` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestDimResumeReappliesConfig` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestDimFreshConfigFailureWithholdsSessionID` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestDimConfigFailClosesSession` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestDimDrainsLateFinalNotificationAfterPromptResponse` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestDimCleanupKillsHangingChild` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestDimPromptMissingNotificationStillCompletes` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestDimResumeNotFound` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestDimResumeLoadsSession` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestDimSessionNew` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestDevecoBackendArgvShapeAndNoPrompt` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestEnsureCodexMcpConfigWritesManagedBlock` | fail | fail | 用例用 POSIX 权限位或 chmod 模拟拒读/拒写/拒删；Windows 合成权限及 ACL 行为不同。 | 需要 Windows 对应的权限/错误注入契约，保留失败交回复审 |
| `pkg/agent::TestBuildAntigravityArgsModel` | fail | fail | 用例用 POSIX 权限位或 chmod 模拟拒读/拒写/拒删；Windows 合成权限及 ACL 行为不同。 | 需要 Windows 对应的权限/错误注入契约，保留失败交回复审 |
| `pkg/agent::TestAntigravityBackendIgnoresTrailingTransportErrorAfterDoneResponse` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestAntigravityBackendIgnoresStaleActiveStepAfterDoneResponse` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestAntigravityBackendIgnoresTrailingNetworkErrorAfterDoneResponse` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestAntigravityBackendStreamJSONCapturesUsage` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestAntigravityBackendStreamPreservesNewlines` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestAntigravityBackendRecoversEmptyStdoutFromTranscript` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestAntigravityBackendProviderErrorSurfacesAsFailed` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestAntigravityBackendPrintTimeoutSurfacesAsTimeout` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestBuildAntigravityArgsNoCapUsesLargePrintTimeout` | fail | fail | 用例用 POSIX 权限位或 chmod 模拟拒读/拒写/拒删；Windows 合成权限及 ACL 行为不同。 | 需要 Windows 对应的权限/错误注入契约，保留失败交回复审 |
| `pkg/agent::TestBuildAntigravityArgsBasic` | fail | fail | 用例用 POSIX 权限位或 chmod 模拟拒读/拒写/拒删；Windows 合成权限及 ACL 行为不同。 | 需要 Windows 对应的权限/错误注入契约，保留失败交回复审 |
| `pkg/agent::TestZeroclawSessionNew` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestHardenBrowserMcpConfigNoopOffWindows` | fail | pass | “非 Windows 不改配置”用例却依赖宿主平台；已复用现有主机注入，明确测试非 Windows 分支。 | 测试设施修复；以 head 全量及基线专项结果为准 |
| `pkg/agent::TestAntigravityToolsStreamBeforeProcessExit` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestZeroclawSessionNewMissingAliasErrorIsActionable` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestAntigravityBackendSumsCompletedStepsWhenCancelled` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestZeroclawSessionNewOmitsAgentAliasWhenUnset` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestAntigravityBackendReportsStructuredFailureUsage` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestZeroclawDoesNotForwardMcpServers` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestZeroclawDoesNotAttemptModelSelection` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestAntigravityBackendKeepsNetworkFailureForNewerPartialResponse` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestZeroclawLegacyChoiceReturnsProtocolErrorThroughClient` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestAntigravityBackendKeepsNetworkFailureForPartialResponse` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestAntigravityBackendKeepsNonTransportFailureAfterDoneResponse` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestZeroclawResumeNotFound` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestZeroclawResumeUsesSessionResume` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestZeroclawPromptModelMetadataControlsUsageAttribution` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestZeroclawResumeDropsReplayedHistory` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestTraecliDrainsNotificationsAfterPromptResponse` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestZeroclawResumeCapabilityUnavailableReportsRejection` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestTraecliUsesSessionLoadForResume` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestTraecliSetModelFailureFailsTask` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestTraecliBlockedArgsFiltering` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestTraecliBackendStreamsAndCompletes` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestValidateServiceTierCodexPerModelCatalog` | fail | fail | 协议发现依赖的 POSIX 假 CLI 在 Windows 未启动，因而退回内置模型清单或没有生成 requests.jsonl。 | 原有协议进程夹具的 Windows 适配，交回复审 |
| `pkg/agent::TestKimiPoisonedHistorySurfacedErrorIsUnresumable/single-line` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiPoisonedHistorySurfacedErrorIsUnresumable/two-line` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiPoisonedHistorySurfacedErrorIsUnresumable` | fail | fail | 父用例汇总失败；具体原因见下列 2 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `pkg/agent::TestKimiBackendThinkingConfirmationFailureWarnsAndContinues/mismatch` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendThinkingConfirmationFailureWarnsAndContinues/missing` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestKimiBackendThinkingConfirmationFailureWarnsAndContinues` | fail | fail | 父用例汇总失败；具体原因见下列 2 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `pkg/agent::TestGrokAuthenticatesBeforeSession/new_session` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokAuthenticatesBeforeSession/resume` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokAuthenticatesBeforeSession` | fail | fail | 父用例汇总失败；具体原因见下列 2 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `pkg/agent::TestGrokPromptStopReasons/max_turn_requests` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokPromptStopReasons/end_turn` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokPromptStopReasons/max_tokens` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokPromptStopReasons/cancelled` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokPromptStopReasons/refusal` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestGrokPromptStopReasons` | fail | fail | 父用例汇总失败；具体原因见下列 5 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `pkg/agent::TestACPBackendsReportRejectedResume/qoder` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsReportRejectedResume/grok` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsReportRejectedResume/traecli` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsReportRejectedResume/kiro` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsReportRejectedResume/kimi` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsReportRejectedResume/hermes` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsReportRejectedResume` | fail | fail | 父用例汇总失败；具体原因见下列 6 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `pkg/agent::TestZeroclawSessionNewSendsConfiguredAgentAlias/separate_value` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestZeroclawSessionNewSendsConfiguredAgentAlias/long_spelling` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestZeroclawSessionNewSendsConfiguredAgentAlias/inline_value` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestZeroclawSessionNewSendsConfiguredAgentAlias` | fail | fail | 父用例汇总失败；具体原因见下列 3 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/hermes/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/traecli/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/zeroclaw/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/zeroclaw/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/dim/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/dim/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/reasonix/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/reasonix/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/kimi/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/kimi/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/hermes/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/kiro/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/qoderclicn/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/qoderclicn/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/qoder/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/qoder/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/traecli/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/kiro/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/mcode/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/mcode/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/grok/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock/grok/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsToolTerminatedTurnFallsBackToLatestTextBlock` | fail | fail | 父用例汇总失败；具体原因见下列 22 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/hermes/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/qoderclicn/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/qoderclicn/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/qoder/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/qoder/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/kiro/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/kiro/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/mcode/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/mcode/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/grok/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/grok/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/traecli/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/traecli/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/zeroclaw/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/reasonix/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/kimi/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/kimi/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/hermes/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/reasonix/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/dim/tool_use_deferred_until_completion` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/zeroclaw/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration/dim/tool_use_emitted_at_start` | fail | fail | 用例的 POSIX 临时假 CLI 在 Windows 无法作为原生命令启动；需补平台对应的协议进程夹具。 | 原有平台夹具边界，保留失败；涉及整套协议夹具改造，交回复审 |
| `pkg/agent::TestACPBackendsDeliverFinalAnswerWithoutNarration` | fail | fail | 父用例汇总失败；具体原因见下列 22 个失败子项。 | 父子项分别保留，统计时不将父项重复计入叶子数 |
