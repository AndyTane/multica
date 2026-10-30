# DSH transport incident and recovery

Times below use Asia/Taipei (UTC+8). Historical facts were checked on 2026-10-30 through the authenticated Multica CLI and DSH session event API; no model prompts were submitted.

## Confirmed findings

The incident affected four runs, rather than the three initially reported:

| Issue | Multica marked failed | Transport code | Original DSH turn completed |
| --- | --- | --- | --- |
| CLAWSTA-59 | 00:50:25 | `web_rpc_outcome_unknown` | 02:21:29 |
| CLAWSTA-57 | 00:50:56 | `web_stream_reconnect_failed` | 02:25:12 |
| CLAWSTA-55 | 00:51:15 | `web_rpc_outcome_unknown` | 02:03:39 |
| CLAWSTA-54 | 00:52:00 | `web_rpc_outcome_unknown` | 02:10:03 |

Each completion was correlated with the original Multica request UUID and its durable `user/message`, `turn/start`, and `turn/end` events, rather than the session's latest response. All four original turns completed successfully. Full output was recovered for attachment to CLAWSTA-61, not committed into this repository.

The live DSH host PID was 33000, started on 2026-10-28 at 01:22:32. It had not restarted during this incident. The bridge connects to loopback port 3080, so its transport does not traverse Cloudflare or the LAN gateway. Multica's own loopback port 8080 message-report request also timed out at 00:50:14, while daemon/server heartbeat traffic continued. No relevant Windows System events were present in the 00:45–00:55 interval.

## Fault in the bridge

`SessionJournal.readNew()` counted reconnects over the entire run and failed after three, including previously successful reconnections. Its recovery catch did not retry HTTP page timeouts (`web_rpc_outcome_unknown`), even though those calls only read history. Either path prematurely released the Multica task and deleted its task-scoped CLI wrapper, leaving DSH running without a working delivery channel.

This is a confirmed Multica-side state-classification defect. The retained logs do not contain the original WebSocket close code, TCP error, or RPC method. They cannot identify what first interrupted that carrier or prove that DSH port 3080 alone caused the interruption. Concurrent loopback timeouts are consistent with local load, but that remains an inference. A host-wide `session/list` query also timed out during current diagnosis; recovery therefore uses only the original session's event snapshot.

## Repair

The bridge now re-queries the original session and replays validated events after stream disconnects and transient HTTP read failures. Retries use exponential delay capped at five seconds and remain subject to task cancellation/timeouts. Protocol, history-integrity and project-identity errors remain fatal. A received terminal event survives its carrier closing immediately afterward.

Lost prompt receipts are resolved by observing the original request witness, without resubmitting the prompt. A new read-only `--recover` entry and `scripts/recover-dsh-run.mjs` recover historical final output; the helper can post that output through the supported Multica comment CLI. Historical execution rows retain their original recorded state.

Sanitized recovery diagnostics now include retry count, transport code and RPC endpoint when available, so future incidents preserve more useful evidence without logging credentials or raw host errors.

## Validation

The fake host tests deliberately terminate progress sockets while leaving the turn running, fail five successive reconnects, lose HTTP history responses, and finish with either a successful or failed DSH turn. They assert the real terminal status, one submitted prompt and no duplicated output. Additional cases cover lost prompt receipts, cancellation during an outage, read-only historical recovery, and UTF-8 comment posting with retained files after failure.

The incident's four completed responses were also recovered from the existing live host. The CLI helper recovered CLAWSTA-59 independently in read-only mode. No real model smoke runs or service restarts were needed.

- `node --test server/internal/dshweb/bridge.test.mjs`: 46 passed.
- `go test ./cmd/multica-dsh-web -count=1`: passed.
- `go test ./pkg/agent -run 'Test(ParseDsh|BuildDsh|DshBackend|DiscoverDsh)' -count=1`: passed.
- `go vet ./cmd/multica-dsh-web ./pkg/agent`: passed.
- Candidate `1.0.2` built successfully and authenticated `--probe` passed against the existing DSH host.
- `git diff --check`: passed. Frontend/mobile checks were not run; those components were not changed.

## CLAWSTA-68 startup regression follow-up

The user reported another failure at 14:28. Its two Multica runs, `01a18ad8-a1c7-74b0-b9e6-2504ca240531` (11:26:55–11:27:09) and `01a18b7e-445d-751b-b940-d839f98144aa` (14:27:50–14:28:06), failed before the bridge emitted a session frame. Both deterministic session identities exist on the host, with initialization events at 11:27:07–08 and 14:28:04–05 respectively, and no original request's prompt witness or turn. This was a startup failure, not a running turn whose completed result needed recovery.

Version 1.0.2 recovered session history but still allowed `workspace/create`, `session/create`, membership confirmation and `session/rename` transport failures to escape preparation. The HTTP deadline was ten seconds. The original failed-call endpoint was not retained; event timestamps and absent prompt witnesses establish that preparation failed around session initialization, but do not distinguish its individual RPC. All four preparation calls now retry in place, with capped backoff and cancellation. Host source confirms workspace creation resolves the same path and explicit session creation joins an existing in-flight creation and checks its directory. Session identities, receipt validation and membership checks remain unchanged. Prompt submission is never retried by this helper.

Six new regressions cover repeated failures before mutation, dropped receipts after mutation, lost membership confirmation, initialization exceeding the HTTP deadline with the host's same-session creation coalescing, cancellation, and fatal protocol errors. The five affected regressions failed against 1.0.2 before implementation. The bridge suite now contains 52 cases.

Preparation was also exercised against both original CLAWSTA-68 empty sessions, with one accepted `session/create` receipt deliberately discarded in each case. The retry completed preparation under the same session ID and confirmed workspace membership. No prompt or cancel calls were made. Detailed receipts are attached to CLAWSTA-61 separately rather than committed. This verifies the startup boundary; the complete skills-development task remains for its assigned executor.

Version 1.0.3 was built and installed at the existing runtime command path using atomic file replacement. Candidate and installed SHA-256 match (`4971B92B82B359C4DA4145E1D349EE5CC751EF896C26577730FC61A5EE20B935`); the installed authenticated `--probe` passed. The pre-existing bridge PID 45984, Multica daemon PID 83428 and DSH host PID 33000 retained their start times. New launches load 1.0.3 while the existing bridge continues its original run. The previous executable was backed up before replacement.

The final 52-case bridge suite, affected Go launcher/backend tests, `go vet` and `git diff --check` all passed.
