# Manual CLI updates

U Workers does not check or download CLI updates on startup or a timer. The
legacy `MULTICA_DAEMON_AUTO_UPDATE` setting cannot enable automatic updates.
An idle daemon may still reload a binary that you replaced yourself; use
`--no-auto-reload` to disable that local behavior.

The executable remains named `multica`. To run `multica update`, configure your
own GitHub Releases-compatible **releases collection endpoint** with
`MULTICA_RELEASE_API_BASE_URL`. For example, for your chosen GitHub repository,
use `https://api.github.com/repos/OWNER/REPOSITORY/releases`. The CLI appends
`/latest` or `/tags/VERSION`; it no longer appends an upstream repository name.
Existing API-root-only mirror settings must be updated to the full collection
endpoint. No release service is selected by default.

Release metadata must include platform archives and `checksums.txt`. Downloads
use the URLs in that metadata, or an explicitly configured
`MULTICA_RELEASE_DOWNLOAD_BASE_URL` serving `/<tag>/<asset-name>`. Archive
checksums are verified before replacing the executable. For Homebrew-managed
installs, set `MULTICA_BREW_PACKAGE` to your actual formula before invoking the
update command. A manually requested runtime update uses these same settings
from the daemon's environment.

Run installation scripts from a trusted local checkout. Bash requires
`MULTICA_REPO_WEB_URL` (the release repository web URL). PowerShell requires `MULTICA_REPO_WEB_URL` and
`MULTICA_RELEASE_API_BASE_URL`. Server provisioning additionally requires
`MULTICA_REPO_URL`, your source Git URL. These are installation actions, so run
them only against the intended installation directory and profile. You can
also build or replace the executable locally with the repository's existing
build tools; the installers do not invent a separate U Workers binary channel.

The installers use `.u-workers/server` and `.u-workers/bin` under the user's
home directory, and print commands for the `u-workers` daemon profile. They
do not inspect a different `multica` executable already on `PATH`, or change
the user's shell `PATH`. `MULTICA_INSTALL_DIR` and `MULTICA_BIN_DIR` select
explicit alternate targets. Local `u-workers-backend:dev` / `u-workers-web:dev`
images are built with the existing source-build override; the installer never pulls
those local names from a registry. Homebrew installations remain managed by
the user's package manager and are not used by these isolated installers.
