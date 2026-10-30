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
