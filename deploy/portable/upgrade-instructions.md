# U Workers differential upgrade

Keep this folder outside the installed application. Read `UPGRADE-FILES.json` for the exact from/to versions and baseline commit. Use an independent PowerShell terminal and the existing installation's Root, Profile and HTTPS PublicOrigin.

```powershell
.\Apply-Upgrade.ps1 -Root C:\UWorkers -Profile office -PublicOrigin https://192.168.1.100:13443 -CheckOnly
.\Apply-Upgrade.ps1 -Root C:\UWorkers -Profile office -PublicOrigin https://192.168.1.100:13443
```

Wait for tasks to finish and stop only this installation's daemon from an independent terminal before applying. The preflight checks the baseline and every unchanged dependency before lifecycle operations. Never pass a sparse delta to `Upgrade.cmd -Source` or robocopy `/MIR`.

Apply backs up the database/uploads, stops the installation, records every file operation in `backups/upgrade-<version>-<timestamp>/files.json`, overlays the program files and invokes the native upgrader without Source. It skips daemon startup, firewall writes, startup registration and browser trust changes. It never grants administrators automatically. If an instance administrator grant is required, identify the exact intended account and use `local-auth.exe grant-admin --user-id <confirmed-UUID> --deployment-config <Root>\deployment.json` as a separate operation with DATABASE_URL cleared in that terminal.

Verify `/readyz`, the reported version, the existing login, workspace/task samples and old attachments. Start only the original installation-specific daemon when acceptance checks pass. Keep the delta and all checkpoints until acceptance.

If an operation fails, preserve the checkpoint and keep the installation stopped. Read files.json in reverse order: restore files present under its `files` directory, preserve the failed replacement separately, and remove added files only where `existed=false`. An intent with no saved original must leave the original in place. Restore the checkpoint's deployment.json to recover the previous Release/version. Keep old staged releases. If restoring data is necessary, use the recorded databaseBackup and the previous native runner to Restore into a new database name; retain the failed database and uploads. Do not rerun Setup or overwrite a live database.
