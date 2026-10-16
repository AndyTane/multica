# Windows Native Self-Hosting Implementation Plan

> Execute the accepted deployment design in this session, retaining the existing Docker deployment as a recovery source.

**Goal:** Run this machine's existing Multica deployment on Windows without requiring Docker or WSL.

**Architecture:** Run PostgreSQL 17, the Go API, and the production Next.js frontend as native processes under one per-user supervisor. Keep runtime configuration, database files, uploads, logs, and backups outside the checkout. Reuse the existing local daemon and preserve its server URL, credentials, and identity.

**Tech Stack:** PowerShell, PostgreSQL 17 Windows binaries, Go, Node.js, Next.js production output.

## Accepted decisions

- Keep PostgreSQL 17 and the existing pgcrypto/pg_trgm extensions. pgvector is not used by this deployment.
- Use a per-user supervisor and login startup entry because the current session is not elevated. No administrator account or Windows service is required.
- Bind native services to loopback. Add a backward-compatible API listen-host setting with a regression test.
- Build Windows artifacts locally with source-version metadata. Do not reuse Linux executables or native Node dependencies.
- Place the deployment in `D:\MulticaData\native`, protect configuration and database files with user-only ACLs, and preserve the Docker volumes.
- Rehearse database restore on an isolated native database and alternate ports. Do not run two production dispatchers against the same database.
- Cut over only after recovery artifacts exist and rehearsal checks pass. Stop the existing API before the final database and upload snapshot. Preserve the public URLs so the existing daemon can reconnect.
- A rollback after native writes requires exporting those writes back to PostgreSQL; restarting the old snapshot alone is not a data-safe rollback.

## Tasks and verification

1. Add loopback host configuration to `server/cmd/server/main.go` and test default and explicit binding in the existing server unit-test layer.
2. Implement `scripts/native-windows.ps1` with preparation, build, backup/restore, start/stop/status, supervisor, and login startup operations. Keep credentials out of arguments and logs, identify owned processes before stopping them, and fail on migration or restore errors.
3. Add meaningful PowerShell tests for configuration, process ownership, and startup behavior without touching real services.
4. Document the Windows workflow and update the repository's managed-environment boundary in `AGENTS.md`.
5. Download PostgreSQL 17.11 Windows binaries from EDB, verify provenance/hash where available, initialize loopback-only storage, and build the API/migrator and production frontend with bounded concurrency.
6. Back up the current database, uploads, and configuration. Restore into a rehearsal database using an appropriate Windows locale and verify extension availability, migration state, data counts, authentication, uploads, and WebSocket connectivity. Keep real agent execution out of the smoke checks.
7. Stop the old application services, take the final consistent snapshot, restore it into the native production database, run migrations, and start the native stack on the existing ports.
8. Verify readiness, existing-account API access, attachments, real-time connections, daemon reconnect, supervisor recovery, and startup installation. Stop Docker Desktop after confirming no unrelated containers are running. Do not terminate unrelated Ubuntu work.
9. Report measured native memory, remaining WSL memory, exact verification performed, and recovery paths.
