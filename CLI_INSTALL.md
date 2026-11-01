# U Workers CLI — Installation Guide for AI Agents

> **Audience:** This document is designed for AI agents (Claude Code, Codex, etc.) to read and execute step-by-step. Each step includes the exact shell commands to run, expected outcomes, and error handling.

## One-Line Instruction for Users

Give this instruction to your AI agent:

```
Fetch https://github.com/multica-ai/multica/blob/main/CLI_INSTALL.md and follow the instructions to install Multica CLI, log in, and start the daemon on this machine.
```

---

## Step 1: Check if U Workers CLI is already installed

Run:

```bash
multica version
```

- **If it prints a version string** (e.g. `multica v0.x.x`): skip to **Step 3**.
- **If command not found**: continue to **Step 2**.

---

## Step 2: Install the U Workers CLI

> **Windows users:** Skip to [Option C: Windows (PowerShell)](#option-c-windows-powershell) below.

### Option A: Homebrew (preferred — macOS/Linux)

Check if Homebrew is available:

```bash
which brew
```

If `brew` is found, install via Homebrew:

```bash
brew install multica-ai/tap/multica
```

Then verify:

```bash
multica version
```

If the version prints successfully, skip to **Step 3**.

To upgrade later, run:

```bash
brew upgrade multica-ai/tap/multica
```

### Option B: Download from GitHub Releases (macOS/Linux, no Homebrew)

If Homebrew is not available, download the binary directly.

Detect OS and architecture, then download the correct archive:

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')   # "darwin" or "linux"
ARCH=$(uname -m)                                # "x86_64" or "arm64"

# Normalize architecture name
if [ "$ARCH" = "x86_64" ]; then
  ARCH="amd64"
fi

# Get the latest release tag from GitHub
LATEST=$(curl -sI https://github.com/multica-ai/multica/releases/latest | grep -i '^location:' | sed 's/.*tag\///' | tr -d '\r\n')

# Download and extract
VERSION="${LATEST#v}"
curl -sL "https://github.com/multica-ai/multica/releases/download/${LATEST}/u-workers-cli-${VERSION}-${OS}-${ARCH}.tar.gz" -o /tmp/u-workers.tar.gz
tar -xzf /tmp/u-workers.tar.gz -C /tmp u-workers
sudo mv /tmp/u-workers /usr/local/bin/u-workers
# The legacy command name keeps working and is installed by both installers:
sudo ln -sfn /usr/local/bin/u-workers /usr/local/bin/multica
rm /tmp/u-workers.tar.gz
```

Releases older than the rename publish `multica-cli-<version>-<os>-<arch>` with a `multica` binary instead; both names run the same command.

Verify:

```bash
u-workers version
```

**If this fails:**
- Check that `/usr/local/bin` is in `$PATH`.
- On Linux, you may need `chmod +x /usr/local/bin/u-workers`.
- If `sudo` is not available, install to a user-writable directory: `mv /tmp/u-workers ~/.local/bin/u-workers` (and link `multica` next to it) and ensure `~/.local/bin` is in `$PATH`.

### Option C: Windows (PowerShell)

Run in PowerShell (no admin required):

```powershell
irm https://raw.githubusercontent.com/multica-ai/multica/main/scripts/install.ps1 | iex
```

This downloads the latest Windows package (`u-workers-cli-<version>-windows-<arch>.zip`) from GitHub Releases, installs `u-workers.exe` to `%USERPROFILE%\.u-workers\bin\`, keeps `multica.exe` alongside it so the old command name still works, and adds the directory to your user PATH.

Verify:

```powershell
u-workers version
```

Older releases publish `multica-cli-<version>-windows-<arch>.zip` instead; both run the same command.

**If this fails:**
- Restart your terminal so the updated PATH takes effect.
- If you use Scoop, the installer will use it automatically: `scoop bucket add multica https://github.com/multica-ai/scoop-bucket.git && scoop install multica`
- If your execution policy blocks the script: `Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy RemoteSigned` then re-run.

---

## Step 3: Log in

Run:

```bash
multica login
```

**Important:** This command opens a browser window for OAuth authentication. Tell the user:

> "A browser window will open for U Workers login. Please complete the authentication in your browser, then come back here."

Wait for the command to complete. It will automatically discover and watch all workspaces the user belongs to.

Verify:

```bash
multica auth status
```

Expected output should show the authenticated user and server URL.

**If login fails:**
- If no browser is available (headless environment), the user can generate a Personal Access Token at `https://multica.ai/settings?tab=tokens` and run: `multica login --token <mul_...>` (use `--token=` with an empty value to be prompted interactively).
- If the server URL needs to be customized: `multica config set server_url <url>` before logging in.

---

## Step 4: Start the daemon

First, check if the daemon is already running:

```bash
multica daemon status
```

- **If status is "running"**: skip to **Step 5**.
- **If status is "stopped"**: start it:

```bash
multica daemon start
```

Wait 3 seconds, then verify:

```bash
multica daemon status
```

Expected output should show `running` status with detected agents (e.g. `claude`, `codex`, `copilot`, `opencode`, `openclaw`, `hermes`, `pi`, `cursor-agent`, `grok`).

**If daemon fails to start:**
- Check logs: `multica daemon logs`
- If a port conflict occurs, the daemon may already be running under a different profile.
- If no agents are detected, ensure at least one AI CLI (`claude`, `codex`, `copilot`, `opencode`, `openclaw`, `hermes`, `pi`, `cursor-agent`, or `grok`) is installed and on the `$PATH`.

---

## Step 5: Verify everything is working

Run:

```bash
multica daemon status
```

Confirm:
1. Status is `running`
2. At least one agent is listed (e.g. `claude`, `codex`, `copilot`, `opencode`, `openclaw`, `hermes`, `pi`, `cursor-agent`, or `grok`)
3. At least one workspace is being watched

If the agents list is empty, tell the user:

> "The U Workers daemon is running but no AI agent CLIs were detected. Please install at least one supported CLI (`claude`, `codex`, `copilot`, `opencode`, `openclaw`, `hermes`, `pi`, `cursor-agent`, or `grok`), then restart the daemon with `multica daemon stop && multica daemon start`."

---

## Summary

When all steps are complete, inform the user:

> "U Workers CLI is installed and the daemon is running. Agents in your workspaces can now execute tasks on this machine. You can manage workspaces with `multica workspace list` and view daemon logs with `multica daemon logs -f`."
