# Windows native self-hosting

This workflow runs PostgreSQL, the Go API, and the production web application as native Windows processes. Docker is needed only to export an existing Docker deployment. The CLI/agent daemon remains a separate process and keeps its existing identity and credentials.

Use `scripts/native-windows.ps1` from the repository root. Keep the checkout and installed dependencies available: Windows standalone dependency junctions may refer to their original locations. Build the frontend on Windows; Linux binaries and native Node dependencies are not portable to Windows.

## Preparation

Requirements: the repository's Go, Node.js, and pnpm versions, Windows PowerShell 5.1 or newer, Git, and Windows `tar.exe`. Use an absolute deployment directory outside the checkout. Commands below use `D:\MulticaData\native`.

Download the PostgreSQL 17 Windows binary archive from [EDB](https://www.enterprisedb.com/download-postgresql-binaries), the binary distribution linked by [PostgreSQL](https://www.postgresql.org/download/windows/). Extract its `pgsql` directory into `<root>\runtime\pgsql`. Standard PostgreSQL includes the required `pgcrypto` and `pg_trgm` extensions; this application does not require pgvector.

```powershell
.\scripts\native-windows.ps1 -Action Prepare -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action Backup -BackupSource Docker -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action Build -Root D:\MulticaData\native
```

`Prepare` imports the current `multica-*` containers' application settings and database credentials. It restricts the deployment directory to the current Windows user and SYSTEM. Configuration and backups contain credentials; keep them outside the repository and do not publish them.

The initial ports are API `18080`, web `13000`, and database `15432`. All bind to loopback. PostgreSQL uses an ICU English locale; validate search and sorting when moving from a Linux libc locale. The default database is `multica_native_rehearsal`.

## Restore and verify

```powershell
.\scripts\native-windows.ps1 -Action Restore -Root D:\MulticaData\native `
  -BackupPath D:\MulticaData\native\backups\<timestamp> -Database multica_native_rehearsal
.\scripts\native-windows.ps1 -Action Start -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action Status -Root D:\MulticaData\native
```

Restore verifies backup checksums, uses `pg_restore --exit-on-error`, refuses an existing target database or a running native supervisor, restores uploads, and runs `ANALYZE`. Start runs migrations before the API, checks readiness, and proves the responding API PID belongs to this deployment.

A restored database includes scheduled jobs and channel installations. Do not run its API concurrently with production when that could dispatch duplicate work or connect the same bot twice. Check these tables first or stop the old API during application rehearsal. Database-only restore checks can run while production remains available.

Verify `/readyz`, existing-account login, issue data, attachment downloads, WebSocket connections, and the existing daemon's reconnect. Do not start real agent runs as part of a default smoke check.

## Cutover

1. Verify the database restore and native build before stopping the current application.
2. Pause task dispatch and stop the Docker frontend/API. Keep PostgreSQL running for the final export. `Backup` can copy uploads from a stopped backend container.
3. Take a final backup, restore it into a new native production database, and stop the rehearsal stack.
4. Configure the original ports, then start the native stack:

```powershell
.\scripts\native-windows.ps1 -Action Configure -Root D:\MulticaData\native `
  -Database multica -ApiPort 8080 -WebPort 3000
.\scripts\native-windows.ps1 -Action Start -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action InstallStartup -Root D:\MulticaData\native
```

The startup entry runs the supervisor hidden when the current user signs in. This is per-user login startup, not a system service that runs before login. PostgreSQL, the API, and frontend are restarted if they exit. Logs are separated by process start in `<root>\logs`.

Preserve the Docker containers and volumes until the native deployment has been accepted. Stop Docker Desktop to release its overhead. Ubuntu or other WSL distributions may still consume memory independently.

## Operations and recovery

```powershell
.\scripts\native-windows.ps1 -Action Status -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action Stop -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action Start -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action RemoveStartup -Root D:\MulticaData\native
```

Stop verifies each child by PID, executable, and process start time. It stops the application processes, then shuts down this PostgreSQL cluster in `fast` mode. Schedule stops/upgrades when tasks are idle; the application children are terminated by their supervisor.

For a consistent native backup, stop application processes while leaving PostgreSQL available, export the database and uploads, then start again:

```powershell
.\scripts\native-windows.ps1 -Action Stop -KeepDatabase -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action Backup -Root D:\MulticaData\native
.\scripts\native-windows.ps1 -Action Start -Root D:\MulticaData\native
```

For an upgrade, take this backup, stop the deployment, build a new release, and start. The migrator runs before the API. Restore overwrites no existing database. Keep tested backups and previous releases.

Before the native deployment accepts writes, rollback can restart the preserved Docker deployment. After native writes begin, export the native database and uploads back to the recovery deployment first; starting the old database snapshot alone would lose those writes.

Run the script tests with `powershell -NoProfile -File scripts/native-windows.test.ps1`. They use temporary files and fake child processes, never PostgreSQL, Docker, or user-installed agent CLIs.
