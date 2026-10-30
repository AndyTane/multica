# CLAWSTA-80 Task 8 evidence (sanitized)

Only destination/trigger/result metadata is kept. Credentials, request bodies,
query strings, tokens and production traffic are not recorded here.

- `acceptance/run-summary.json` — isolated Windows deployment run summary,
  observed scenarios, caveats and the reviewer's independent evidence scope.
- `acceptance/browser-observation.json` — browser/WS/navigation/TCP scenario results.
- `acceptance/offline-scenario.json` — external-network-unreachable behavior.
- `acceptance/observer-controls.json` — Node/Go control results and deployment proxy env keys.
- Raw Playwright output is kept locally next to this file but is gitignored
  (the final dedicated run was `10 passed, 1 skipped, exit 0`).
- `linux-acceptance.json` — Linux API actual-startup run on WSL Ubuntu 24.04.

The isolated deployment and its PostgreSQL cluster lived outside the repository
and were stopped/removed after the run; see docs/outbound-data-verification.md
for the runbook, executed revision and cleanup record.

- `desktop/run-summary.json` — desktop batch D2 continuous observation pass:
  phase windows, recorder/TCP counts, control windows, daemon identification,
  cleanup attribution checks and the declared boundaries (sanitized metadata
  only; no tokens, login codes, cookies or request bodies).
- `desktop/recorder.jsonl` — recorder metadata captured during the D2 pass
  (timestamp, kind, method, destination host, refusal result).
- `desktop/tcp-samples.jsonl` — sanitized Windows TCP samples from the D2 pass
  (timestamp, interval, PID lists, non-loopback remote/state only).
- `desktop/run-summary-d1.json` — historical D1 desktop summary. D1 only
  observed its final idle window; D2 supersedes the observation scope.
- `desktop/login.png`, `desktop/main.png` — approved D1 full-window screenshots;
  D2 did not regenerate them.
