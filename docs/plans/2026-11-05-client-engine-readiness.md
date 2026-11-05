# Client engine readiness implementation plan

**Goal:** Keep an authenticated client connected and updatable while its engines need configuration, with actionable Chinese guidance.

**Architecture:** Connection readiness is independent of registered execution runtimes. A workspace-authorized client heartbeat carries managed update offers and confirms reconnection without creating a runtime. Execution still uses only successfully registered providers. Existing helper identity, hash, idle and rollback protections remain authoritative.

**Tech stack:** Go daemon/HTTP handlers, Windows PowerShell 5.1/7 Client-Kit.

1. Add regression coverage for authenticated zero-runtime startup, configuration diagnostics, custom-only runtimes and control-heartbeat authorization. Keep network/auth failures distinct.
2. Allow empty discovery; retain authenticated workspace tracking. Add the minimal client control heartbeat/package access with the existing workspace/member boundary. Retire only explicitly unavailable built-ins belonging to this machine and token owner; preserve runtime/task records.
3. Validate OpenCode selection references and JSON/JSONC as engine readiness rather than process readiness. Keep intentional disable, selected paths, external files and credentials unchanged. Surface current-user OpenCode and DSH integration/Web paths without claiming that a file proves model execution.
4. Expose additive connection/execution diagnostics in daemon health/status; show the same Chinese guidance through Connect/Reconnect/Status. Re-detect via idle Stop + explicit selection + Reconnect.
5. Exercise actual candidate daemon processes and managed updates in isolated profiles/directories, including zero-engine confirmation, next update, reconnect, real process failure rollback and invalid identities/auth. Run affected Go/PowerShell checks, build/vet/diff checks and attach raw evidence. Do not alter either installed machine, tag, publish or merge.

Compatibility: old clients retain runtime heartbeats. New clients on an older server can authenticate through workspace discovery and remain pending; zero-engine managed updates require the new control endpoint. No Web/Desktop layout change is planned.
