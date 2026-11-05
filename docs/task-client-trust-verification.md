# Task CLI public trust verification

CLAWSTA-146 candidate, based on main `d3777980dcd841dc4a3f68eb49eb19d2b80f148a`.

## Behavior

The daemon's existing CA selection remains unchanged: explicit environment wins over the selected CLI profile; the current environment spelling wins over its legacy alias at CLI bootstrap. After task authentication is validated, the daemon resolves the selected CA path before changing working directories, validates the certificate file, and explicitly adds that path to the child environment. The product namespace filter and `custom_env` blocklist remain intact.

Each dispatch atomically refreshes the existing private task `config.json` with only `root_ca_file`, or `{}` when no extra CA is selected. This clears stale task trust and locally persisted task identity on reuse. No owner profile is loaded or copied by this preparation step. A shell that drops the CA variable can still use the existing task-config lookup; it must retain its task identity and config-root context. A surviving task marker without the private config root also prevents trust bootstrap from loading the owner's profile. Failed trust preparation is reported as environment setup failure.

This does not introduce OS file isolation or prevent an agent from intentionally reading files allowed by its OS account. Windows `Chmod` calls are not a claim of Unix permission enforcement or a Windows ACL boundary.

## Results (Windows, Go 1.26.9)

- Before the fix, `TestTaskMulticaEnvironmentIncludesClientTrust` failed with an empty task CA while a public CA was selected.
- 121 distinct affected top-level unit/regression tests passed: daemon task trust/identity/environment assembly and preparation-failure classification, CLI profile/bootstrap/auth/config behavior, TLS, environment alias bridging and agent environment filtering. The last caller change (setup-failure classification) was followed by the six relevant daemon tests and daemon vet again.
- The real CLI subprocess matrix passed **36 cases: 18 per executable name**, `u-workers.exe` and `multica.exe`. It builds the candidate source, uses a fake provider through the actual OpenCode backend/environment merge, and performs real HTTPS comment POSTs to an isolated test server. Every accepted request must carry the exact synthetic `mat_` identity plus task, agent and workspace headers. Source owner fixtures remain byte-identical. No model executable or account is used.
- Windows PowerShell **5.1.26100.9444** and PowerShell **7.6.9** each successfully submitted UTF-8 Chinese text using task config after the CA environment variable was removed, under both CLI names.
- `go vet` passed for daemon, CLI command, CLI library, TLS, environment compatibility and agent packages. `git diff --check` passed.

The 18 subprocess cases cover current and legacy CA/context names; current-over-legacy precedence; task config after shell CA loss; explicit environment over config; wrong environment refusing profile fallback; missing CA; ordinary HTTP without an extra CA; an unrelated CA; malformed/missing CA files; hostname and expiry verification; missing task token; loss of all product context with a surviving task marker; and both Windows shells.

Separate bootstrap tests cover default and named source profiles, explicit environment precedence, wrong explicit trust and system-only trust. Preparation tests cover absolute/relative paths, public-only config, reuse with cleared trust, bad/missing files, invalid config roots, atomic installation failure and temporary-file cleanup.

## Existing Windows test limitations

Two existing assertions were excluded from the reported passing selection, rather than counted as passes:

- `TestCLIConfig_TaskRootOverridesOwnerHome` expects Unix 0700/0600 mode bits; native Windows reports 0777/0666. The real CLI matrix independently checks task identity isolation, public-only task config and unchanged synthetic owner state.
- `TestNewAPIClient_LeftoverMarkerActionableError` searches for a slash-separated marker path inside a native Windows backslash path. The actual refusal occurred; the matrix independently verifies that a surviving marker with missing identity produces no HTTP write.

Normal interactive-profile fallback tests must execute outside the running agent workspace's ancestor task marker. The compiled CLI test binary was run from a clean temporary working directory. No original marker was removed. Initial failures caused by this test working directory, the two assertions above, and the nested build's redirected Go cache were diagnosed separately and are not product regression passes.

## Reproduction

From `server/`, preserve the build cache paths explicitly before the package test harness redirects HOME:

```powershell
$env:GOPATH = (go env GOPATH).Trim()
$env:GOMODCACHE = (go env GOMODCACHE).Trim()
$env:GOCACHE = (go env GOCACHE).Trim()
go test -tags tasktrustintegration ./internal/daemon -run '^TestTaskClientTrustCLI$' -count=1 -v
go test ./internal/daemon -run 'Test(TaskMulticaEnvironment|TaskScopedAuthToken|PrepareTaskClientTrust|IsBlockedEnvKey|LayerCustomEnv|Codex.*Env|ConfigureCodexTaskShellEnv|SanitizeAgentEnv|TaskRunFailureReasonLabelsEnvironmentSetup|TaskRunFailureReasonPrefersSpecificPreparationSentinels)' -count=1
go test ./internal/cli ./internal/tlsx ./internal/envcompat ./pkg/agent -run 'Test(MergeEnv|BuildEnv|Plan|Bridge|Transport|LoadPool|TLSConfig|Install|Parse|ConfigRoot|CLIConfig|TaskLocal)' -skip '^TestCLIConfig_TaskRootOverridesOwnerHome$' -count=1
go test -c -o ./cmd-task-trust.test.exe ./cmd/multica
# Invoke the compiled binary from a directory outside any daemon task marker.
# Quote -test.* arguments when invoking a Windows native executable.+# <absolute-binary-path> '-test.run=Test(BootstrapClientTrust|ResolveToken|NewAPIClient|RunConfigCommands|HumanAuth|HumanLocal|DaemonLocal|Connect|Review107Reconnect)' '-test.skip=^TestNewAPIClient_LeftoverMarkerActionableError$' '-test.v'
go vet ./internal/daemon ./cmd/multica ./internal/cli ./internal/tlsx ./internal/envcompat ./pkg/agent
```

The subprocess suite requires both named Windows shells when run on Windows; other platforms run the remaining matrix. All servers bind ephemeral loopback ports and close through test cleanup. Temporary CLI binaries and synthetic homes are test-owned. No release, installed binary, original login/profile, model configuration, system trust, firewall or startup setting was changed.
