## U Workers desktop identity

The packaged Electron productName and `app.setName` are `U Workers`. Before taking the single-instance lock, the application explicitly selects `appData/U Workers` as userData (development uses `U Workers Canary` plus the existing worktree suffix). Cookies, session storage, window state and managed binaries therefore use a separate directory. No old userData is copied or migrated.

The packaged runtime configuration is `~/.u-workers/desktop.json`; daemon preferences are also under `~/.u-workers`. A missing file shows the configuration error page. All three paths resolve `~` through `os.homedir()`, so redirecting `USERPROFILE`/`HOME` relocates the whole desktop profile; Electron's `app.getPath("home")` is deliberately not used because it ignores `USERPROFILE` on Windows.

The bundled CLI retains its `multica` executable and profile protocol. Desktop derives only `u-workers-desktop-<API host and port>` profiles and passes `--profile` on CLI lifecycle operations. Their configuration, PAT, PID and logs live under `~/.multica/profiles/u-workers-desktop-<host>/`, as required by that CLI protocol. This does not reuse the default CLI profile or the former `desktop-<host>` profile. Invalid/unresolved API addresses cannot fall back to a default profile. No old daemon credentials or data are migrated.

Profile and configuration tests use fake addresses and paths; they do not start a daemon or inspect the user's actual configuration. Native installer, tray and operating-system icon association still require platform release testing.
