# DSH Stream Recovery Implementation Plan

**Goal:** Keep a DSH task attached across transport failures and recover completed output without submitting another prompt.

**Architecture:** The existing authenticated session event API remains authoritative. On transport loss query the original session through a fresh `session/follow` snapshot and replay validated history for the original request. Retry transport failures with capped backoff until the owning task is cancelled; protocol and identity errors remain fatal. A read-only recovery entry returns a historical request's durable terminal result. Avoid host-wide `session/list`: it does not identify an individual request's outcome and timed out against this installation during diagnosis.

**Tech Stack:** Node.js bridge, Go launcher, fake HTTP/WebSocket host tests.

1. Add regressions for more than three disconnects, a prolonged outage while the run continues, page RPC failure, and actual failed turns. Verify each prompt is sent once and each output event emitted once.
2. Replace the lifetime reconnect limit with cancellable transport recovery; keep history validation strict. Check prompt receipt uncertainty by observing the request witness without resending.
3. Add a read-only historical recovery command, validate request/session/project identity and refuse incomplete turns. Test that it creates no workspace, session or prompt.
4. Inspect incident logs and public session histories, record verified facts and recover available results. Do not restart active services or alter historical task records through unsupported platform APIs.
5. Run bridge tests and affected Go tests, build the launcher, and deliver the commit, recovery evidence and rollout instructions.

The fixed retry-count alternative still misclassifies longer outages; immediately retrying prompts can repeat work. Neither is appropriate here.
