# Client-Kit OpenCode discovery implementation plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Connect an installed OpenCode outside PATH, select its external JSON/JSONC config, and retain that choice across reconnect and managed upgrades.

**Architecture:** Keep bounded discovery and console interaction in the existing `client-settings.ps1` distribution file. Store only absolute paths and the profile in `opencode-selection.json` at the stable Kit data root. Load that selection before daemon discovery, and use the same config path for task inheritance and local MCP reads. Keep existing package file lists unchanged so already installed managed clients can validate the next package.

**Tech Stack:** Windows PowerShell 5.1/7, Go, existing JSONC reader and managed-kit lifecycle.

---

### Task 1: Discover and select without invoking model CLIs

- Modify `deploy/client-kit/client-settings.ps1`, `client.ps1`, `Reconnect.cmd`.
- Add temporary-directory regression cases in `deploy/client-kit/test/opencode.test.ps1` using fake executables and scripted prompt responses.
- Cover PATH plus common directories, npm shim identity, fixed disk Tools/Programs, explicit files/directories, bounded depth/count/time, reparse-point avoidance, stable choices, retry/cancel, and JSONC syntax validation without printing contents.
- Run both PowerShell versions; confirm missing config names the expanded destination and skipping OpenCode leaves other providers available.

### Task 2: Use the persisted selection in the daemon

- Add `server/internal/managedkit/opencode.go` and tests; modify daemon `config.go` and `runtime_mcp.go`.
- Validate profile and paths before exporting the existing executable/config environment settings. Managed helper launches reload the same data root after a version-directory switch.
- Add daemon MCP and agent environment tests for explicit config and JSONC. Run targeted Go tests and vet.

### Task 3: Integration and machine handoff

- Run existing Client-Kit settings/lifecycle and managed-kit transaction regressions. Confirm package templates contain no personal selection or config contents.
- Read only safe deployment/profile/process/listener fields on `desktop` and the current PC. Document supported full-package versus native Stage migration paths; do not replace real programs or start/stop deployments in this implementation turn.
- Update Client-Kit README and managed-client docs. Submit a candidate PR with base/head, commands, four required scenarios, remaining machine attribution questions, and the final-package dependency for real upgrades.

The caller has authorized implementation in this session. No delegation, release packaging, tag, OSS operation, or real-machine program replacement is part of this candidate.
