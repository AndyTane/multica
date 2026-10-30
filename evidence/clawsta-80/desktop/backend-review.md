# Desktop D2 backend acceptance review

Accepted on 2026-10-31. Candidate PR #4 head:
`c1a976f1bf821cdf4ac09bad446cdb1b48ddae92`, based on `9930709c7`.
Integration starts at PR #3 `f76f3b7e0`; product commit `245cf2e36` was
already included as `9930709c7` and was not duplicated. The remaining four
commits map to `8d5033162`, `8d33552df`, `7f92eb57c`, `5f88b52a8`.
Only the outbound report conflicted; both accepted C1 and D2 results were retained.

## Accepted evidence

The real D2 desktop pass ran at 19:43–19:45 UTC on 2026-10-30. The reviewer
replayed the retained `recorder.jsonl` and `tcp-samples.jsonl` against every
phase and compared the complete computed results to `run-summary.json`.
All six phase results match. Samples per phase: 15 / 2 / 2 / 2 / 2 / 28.
There are 52 total samples; one falls after the idle endpoint and is not counted
in a phase. The four recorder traffic entries are exact controls inside their
declared windows; the fifth entry is a control-time client connection reset.
The only sampled external TCP destination is the pre-launch TEST-NET control.
Daemon PIDs 49512 and 52756 appear from login onward; no non-control egress
was observed. Median interval 2067 ms, maximum 2290 ms: sampling is a backstop,
not a complete traffic record. Browser events before first-window attachment
are explicitly not measured; the Chromium recorder predates launch.

The candidate cleanup record identifies the owned daemon by path/profile/time,
records its termination and no remaining app/daemon processes, and confirms
workspace daemon 83428 survived. The driver still stopped by PID after its
snapshot, so the reviewer corrected the last step before accepting the reusable
driver. This does not retroactively claim that the live run used the new guard.

## Reviewer corrections and validation

- Recheck executable and creation timestamp immediately before termination,
  retain the process handle and kill through it. Re-query the workspace daemon
  via CLI; reject unknown ownership and record stop failures. Failed process
  snapshots no longer become empty successful-cleanup scans.
- Validate sampler PID arrays and connection fields; normalize deletion paths
  before checking containment. Malformed samples cannot become measured zeroes.
- Use phase-end browser snapshots, limit browser probe exclusions to the actual
  control interval, require the Go control to be recorded, and assert recorder
  traffic over the complete pass including phase boundaries.

`node --test scripts/e2e/desktop-observer.test.mjs`: **14 passed, exit 0**.
This includes replay of all retained phases, an injected forbidden request in
each phase, malformed data and traversal rejection, and a real Windows Node
test child: protected PID, wrong executable and wrong creation time refuse to
stop it; matching identity stops it. No product or workspace process is killed.
The first guard run exposed CIM's microsecond versus process-handle 100 ns
timestamp precision; comparison now allows only that sub-microsecond precision
difference while requiring the fresh CIM timestamp to match exactly.

`pnpm exec playwright test e2e/u-workers-brand.spec.ts --project=chromium --grep
"packaged app boots" --list`: **exit 0**, one case discovered.
`git diff --check`: **exit 0**.

DSH's live desktop run remains **1 passed, exit 0** (2.1 minutes). No second
desktop run, packaging, screenshots, C1 browser matrix or broad unit suite was
performed in this review. Raw evidence and D1 history remain unmodified.

## Task 8 outcome

C1 remote lifecycle/LAN business workflow and D1 frontend acceptance remain
accepted. D2 closes startup/login/browse observation and cleanup-attribution
gaps. Task 8 is ready for Tane's review through draft PR #3; PRs remain unmerged.
The existing full-suite failures and unverified NSIS/Linux frontend/PWA/company
network/HTTPS scopes remain explicitly listed in the main report.
