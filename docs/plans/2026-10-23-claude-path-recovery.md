# Claude native installation path recovery

Goal: Recover from a Windows Claude npm-to-native upgrade without restarting the daemon.

Use the existing discovery and verified self-heal boundary. When the bare Claude command no longer resolves on the process PATH, check the current user's native installation at `.local/bin/claude.exe`. Explicit paths remain authoritative. Preserve version and minimum-version validation before adopting a replacement. A cached verified replacement must remain launchable after the original entry disappears.

Alternatives considered: restarting on upgrades would interrupt other providers; replacing the process PATH globally would change unrelated command resolution. A provider-specific installation candidate keeps the change scoped.

Implementation and verification:
1. Add Windows regression tests for a stale PATH, PATH precedence, explicit overrides, and repeated launches after migration. Use temporary executable files and stub version detection, never a real agent request.
2. Add the native candidate to the shared executable resolver and discard obsolete resolution failures only when a verified cached replacement exists and the original entry has disappeared.
3. Run focused tests, then the daemon package suite and diff checks.
4. Build with the stamped local CLI script, replace the daemon only while idle, and verify registration without sending model requests.
