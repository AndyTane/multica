# Update entry completion on Windows

R4 found that the v0.2.10 Update entry does not finish after the native upgrader starts its persistent HTTPS child. The intermediate report exists, but Apply cannot proceed to final listener checks or activate the client cache. Stable remains v0.2.9.

The entry captures Apply through `Out-Null`. A controlled Windows child reproduces the difference: a direct invocation returns while the child remains alive; a capturing pipeline waits for that child to exit. This matches the real candidate's process tree and stalled final acknowledgement.

1. Extend the existing fake OSS/lifecycle regression with an owned, finite native child. Require Update to finish final checks while that child remains running; verify its executable and start time before cleanup. Reproduce the failure before fixing the entry.
2. Invoke Apply directly and retain its progress output. Keep validation, backup, listener checks, error propagation and client-cache activation unchanged.
3. Advance the root/web release version to v0.2.11. Preserve the v0.2.10 tag and uploaded immutable objects; do not promote that candidate or overwrite it.
4. Run affected OSS and entry checks on PowerShell 5.1 and 7, merge the scoped release fix under the R4 authorization, then build all components from the new main tag.
5. Rehearse the original v0.2.9 baseline through the real bootstrap Update, validate recovery, final reports/cache timing, browser login, complete target hashes, two profile imports and authenticated client-package bytes. Reuse accepted R3/R3.1 automatic handoff evidence because that implementation is unchanged.
6. Verify immutable OSS downloads and CRC64 before switching stable last. Record the skipped candidate, publish bootstrap/company instructions, and stop all isolated processes. Company business and cross-machine acceptance remain pending.
