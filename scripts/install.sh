#!/usr/bin/env bash
# U Workers installer — installs the CLI and optionally provisions a self-host server.
#
# Install / upgrade CLI only:
#   bash scripts/install.sh
#
# Install CLI + provision self-host server:
#   bash scripts/install.sh --with-server
#
# After installation, run the installed binary with `--profile u-workers setup self-host` to configure your environment.
#
set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
# Every setting accepts the current U_WORKERS_ spelling first and the legacy
# MULTICA_ spelling second, so an operator can migrate before the old names
# are retired.
REPO_URL="${U_WORKERS_REPO_URL:-${MULTICA_REPO_URL:-}}"
REPO_WEB_URL="${U_WORKERS_REPO_WEB_URL:-${MULTICA_REPO_WEB_URL:-}}"  # without .git, for GitHub web APIs
INSTALL_DIR="${U_WORKERS_INSTALL_DIR:-${MULTICA_INSTALL_DIR:-$HOME/.u-workers/server}}"
BIN_DIR="${U_WORKERS_BIN_DIR:-${MULTICA_BIN_DIR:-$HOME/.u-workers/bin}}"
CLI_BINARY="$BIN_DIR/u-workers"
LEGACY_CLI_BINARY="$BIN_DIR/multica"

# Host ports Compose reported after `up -d`; set by setup_server and reused by
# the summary so the health check and the printed URLs cannot diverge.
SELFHOST_BACKEND_PORT=""
SELFHOST_FRONTEND_PORT=""

# Colors (disabled when not a terminal)
if [ -t 1 ] || [ -t 2 ]; then
  BOLD='\033[1m'
  GREEN='\033[0;32m'
  YELLOW='\033[0;33m'
  RED='\033[0;31m'
  CYAN='\033[0;36m'
  RESET='\033[0m'
else
  BOLD='' GREEN='' YELLOW='' RED='' CYAN='' RESET=''
fi

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
info()  { printf "${BOLD}${CYAN}==> %s${RESET}\n" "$*"; }
ok()    { printf "${BOLD}${GREEN}✓ %s${RESET}\n" "$*"; }
warn()  { printf "${BOLD}${YELLOW}⚠ %s${RESET}\n" "$*" >&2; }
fail()  { printf "${BOLD}${RED}✗ %s${RESET}\n" "$*" >&2; exit 1; }

command_exists() { command -v "$1" >/dev/null 2>&1; }

running_in_ssh_session() {
  [ -n "${SSH_CONNECTION:-}" ] || [ -n "${SSH_CLIENT:-}" ] || [ -n "${SSH_TTY:-}" ]
}

print_remote_server_token_hint() {
  if ! running_in_ssh_session; then
    return
  fi

  printf "  ${BOLD}Looks like a remote/SSH session.${RESET} Browser login may not be able to call back to this machine's localhost.\n"
  printf "  Token login is usually simpler here:\n"
  printf "     1. On your local computer, open ${CYAN}your server, then Settings > API Tokens${RESET}\n"
  printf "     2. On this server, run:\n"
  printf "        ${CYAN}\"$CLI_BINARY\" --profile u-workers login --token <YOUR_TOKEN> --server-url <YOUR_SERVER>${RESET}\n"
  printf "        ${CYAN}\"$CLI_BINARY\" --profile u-workers daemon start${RESET}\n"
  printf "\n"
}

# Host port Docker Compose actually published for a service.
#
# This is the only authority. Compose's interpolation gives the calling process
# environment precedence over .env, so an ambient PORT / BACKEND_PORT / API_PORT
# / SERVER_PORT / FRONTEND_PORT moves the published port without touching the
# file. Re-deriving the port from .env alone made the installer probe and print
# a port the stack was never published on (#6145). Must be called from the
# installation directory, after `up -d`.
compose_published_port() {
  local service=$1 container_port=$2 published

  published="$(docker compose -f docker-compose.selfhost.yml port "$service" "$container_port" 2>/dev/null | tail -n 1)"
  published="${published##*:}"
  published="${published%$'\r'}"

  case "$published" in
    "" | *[!0-9]*) return 1 ;;
  esac

  printf "%s" "$published"
}

detect_os() {
  case "$(uname -s)" in
    Darwin) OS="darwin" ;;
    Linux)  OS="linux" ;;
    MINGW*|MSYS*|CYGWIN*)
            fail "This script does not support Windows. Use the PowerShell installer instead:
  powershell -File scripts/install.ps1" ;;
    *)      fail "Unsupported operating system: $(uname -s). U Workers supports macOS, Linux, and Windows." ;;
  esac

  ARCH="$(uname -m)"
  case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    arm64)   ARCH="arm64" ;;
    *)       fail "Unsupported architecture: $ARCH" ;;
  esac
}

# ---------------------------------------------------------------------------
# CLI Installation
# ---------------------------------------------------------------------------
link_legacy_cli_command() {
  # The pre-rename `multica` command stays installed and points at the current
  # binary, so a self-update of either name moves both to the same release.
  if ln -sfn "u-workers" "$LEGACY_CLI_BINARY" 2>/dev/null; then
    return 0
  fi
  cp -f "$CLI_BINARY" "$LEGACY_CLI_BINARY"
}

install_cli_binary() {
  [ -n "$REPO_WEB_URL" ] || fail "Set U_WORKERS_REPO_WEB_URL (or MULTICA_REPO_WEB_URL) to your release repository before installing."
  info "Installing U Workers CLI from GitHub Releases..."

  # Get latest release tag
  local latest
  latest=$(curl -sI "$REPO_WEB_URL/releases/latest" 2>/dev/null | grep -i '^location:' | sed 's/.*tag\///' | tr -d '\r\n' || true)
  if [ -z "$latest" ]; then
    fail "Could not determine latest release. Check your network connection."
  fi

  local version="${latest#v}"
  local tmp_dir
  tmp_dir=$(mktemp -d)

  # Current download package first; fall back to the pre-rename package names
  # so an older release tag still installs. Each archive family carries one
  # executable name, so try both members before giving up on an asset.
  local asset binary_file=""
  for asset in \
    "u-workers-cli-${version}-${OS}-${ARCH}.tar.gz" \
    "multica-cli-${version}-${OS}-${ARCH}.tar.gz" \
    "multica_${OS}_${ARCH}.tar.gz"; do
    local url="${REPO_WEB_URL}/releases/download/${latest}/${asset}"
    info "Downloading $url ..."
    curl -fsSL "$url" -o "$tmp_dir/cli.tar.gz" 2>/dev/null || continue
    if tar -xzf "$tmp_dir/cli.tar.gz" -C "$tmp_dir" u-workers 2>/dev/null; then
      binary_file="$tmp_dir/u-workers"
      break
    fi
    if tar -xzf "$tmp_dir/cli.tar.gz" -C "$tmp_dir" multica 2>/dev/null; then
      binary_file="$tmp_dir/multica"
      break
    fi
  done
  if [ -z "$binary_file" ]; then
    rm -rf "$tmp_dir"
    fail "Failed to download CLI binary."
  fi

  local bin_dir="$BIN_DIR"
  mkdir -p "$bin_dir"
  mv "$binary_file" "$CLI_BINARY"
  chmod +x "$CLI_BINARY"
  link_legacy_cli_command

  rm -rf "$tmp_dir"
  ok "U Workers CLI installed to $CLI_BINARY (legacy command: $LEGACY_CLI_BINARY)"
}

get_latest_version() {
  [ -n "$REPO_WEB_URL" ] || return 0
  # grep exits 1 when no match; use `|| true` to avoid triggering pipefail
  curl -sI "$REPO_WEB_URL/releases/latest" 2>/dev/null | grep -i '^location:' | sed 's/.*tag\///' | tr -d '\r\n' || true
}

get_selfhost_ref() {
  local selfhost_ref="${U_WORKERS_SELFHOST_REF:-${MULTICA_SELFHOST_REF:-}}"
  if [ -n "$selfhost_ref" ]; then
    printf '%s' "$selfhost_ref"
    return
  fi

  local latest
  latest=$(get_latest_version)
  if [ -n "$latest" ]; then
    printf '%s' "$latest"
    return
  fi

  printf '%s' "main"
}

checkout_server_ref() {
  local ref="$1"

  if [ "$ref" = "main" ]; then
    git fetch origin main --depth 1 2>/dev/null || true
    git checkout --force main 2>/dev/null || true
    git reset --hard origin/main 2>/dev/null || true
    return
  fi

  git fetch origin --tags --force 2>/dev/null || true
  if git rev-parse --verify --quiet "refs/tags/$ref" >/dev/null; then
    git checkout --force "$ref" 2>/dev/null || git checkout --force "tags/$ref" 2>/dev/null || true
    return
  fi

  git fetch origin "$ref" --depth 1 2>/dev/null || true
  git checkout --force "$ref" 2>/dev/null || true
}

pull_selfhost_images() {
  local images
  images=$(docker compose -f docker-compose.selfhost.yml config --images) || fail "Could not resolve your server image configuration."
  if [ -z "$images" ]; then
    fail "Your server source did not provide an image configuration."
  fi
  local build_services=()
  if printf '%s\n' "$images" | grep -Fxq 'u-workers-backend:dev'; then build_services+=(backend); fi
  if printf '%s\n' "$images" | grep -Fxq 'u-workers-web:dev'; then build_services+=(frontend); fi
  if [ "${#build_services[@]}" -gt 0 ]; then
    info "Building local server images from your selected source..."
    docker compose -f docker-compose.selfhost.yml -f docker-compose.selfhost.build.yml build "${build_services[@]}" || fail "Could not build local server images."
    return
  fi
  docker compose -f docker-compose.selfhost.yml pull || fail "Could not pull your configured server images."
}

install_cli() {
  if [ -x "$CLI_BINARY" ]; then
    local current_ver
    # `multica version` outputs "multica 0.3.23 (commit: f46b929eb, built: 2026-06-16T10:11:56Z)" — extract just the version
    current_ver=$("$CLI_BINARY" version 2>/dev/null | awk 'NR==1{print $2}' || echo "unknown")

    local latest_ver
    latest_ver=$(get_latest_version)

    # Normalize: strip leading 'v' for comparison
    local current_cmp="${current_ver#v}"
    local latest_cmp="${latest_ver#v}"

    if [ -z "$latest_ver" ]; then
      fail "Could not check your configured release source."
    fi
    if [ "$current_cmp" = "$latest_cmp" ]; then
      ok "U Workers CLI is up to date ($current_ver)"
      return 0
    fi

    info "U Workers CLI $current_ver installed, latest is $latest_ver — upgrading..."
    install_cli_binary

    local new_ver
    new_ver=$("$CLI_BINARY" version 2>/dev/null | awk 'NR==1{print $2}' || echo "unknown")
    ok "U Workers CLI upgraded ($current_ver → $new_ver)"
    return 0
  fi

  install_cli_binary

  # Verify
  if [ ! -x "$CLI_BINARY" ]; then
    fail "CLI binary was not created at the configured target directory."
  fi
}

# ---------------------------------------------------------------------------
# Docker check
# ---------------------------------------------------------------------------
check_docker() {
  if ! command_exists docker; then
    printf "\n"
    fail "Docker is not installed. U Workers self-hosting requires Docker and Docker Compose.

Install Docker:
  macOS:  https://docs.docker.com/desktop/install/mac-install/
  Linux:  https://docs.docker.com/engine/install/

After installing Docker, re-run this script with --with-server."
  fi

  if ! docker info >/dev/null 2>&1; then
    fail "Docker is installed but not running. Please start Docker and re-run this script."
  fi

  ok "Docker is available"
}

# ---------------------------------------------------------------------------
# Server setup (self-host / --with-server)
# ---------------------------------------------------------------------------
setup_server() {
  [ -n "$REPO_URL" ] || fail "Set U_WORKERS_REPO_URL (or MULTICA_REPO_URL) to your source repository before provisioning."
  info "Setting up U Workers server..."
  local server_ref
  server_ref=$(get_selfhost_ref)
  info "Using self-host assets from ${server_ref}..."

  if [ -d "$INSTALL_DIR/.git" ]; then
    info "Updating existing installation at $INSTALL_DIR..."
    cd "$INSTALL_DIR"
  else
    info "Cloning U Workers repository..."
    if ! command_exists git; then
      fail "Git is not installed. Please install git and re-run."
    fi
    # Remove leftover directory from a previously interrupted clone
    if [ -d "$INSTALL_DIR" ]; then
      warn "Removing incomplete installation at $INSTALL_DIR..."
      rm -rf "$INSTALL_DIR"
    fi
    mkdir -p "$(dirname "$INSTALL_DIR")"
    git clone --depth 1 "$REPO_URL" "$INSTALL_DIR"
    cd "$INSTALL_DIR"
  fi

  checkout_server_ref "$server_ref"

  ok "Repository ready at $INSTALL_DIR ($server_ref)"

  # Generate .env if needed
  if [ ! -f .env ]; then
    info "Creating .env with random secrets..."
    cp .env.example .env
    local jwt pgpass
    jwt=$(openssl rand -hex 32)
    pgpass=$(openssl rand -hex 24)
    if [ "$(uname -s)" = "Darwin" ]; then
      sed -i '' "s/^JWT_SECRET=.*/JWT_SECRET=$jwt/" .env
      sed -i '' "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=$pgpass/" .env
      sed -i '' -E "s#^(DATABASE_URL=postgres://[^:]+:)[^@]*(@.*)#\1$pgpass\2#" .env
    else
      sed -i "s/^JWT_SECRET=.*/JWT_SECRET=$jwt/" .env
      sed -i "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=$pgpass/" .env
      sed -i -E "s#^(DATABASE_URL=postgres://[^:]+:)[^@]*(@.*)#\1$pgpass\2#" .env
    fi
    ok "Generated .env with random JWT_SECRET and POSTGRES_PASSWORD"
  else
    ok "Using existing .env"
  fi

  # Start Docker Compose
  info "Pulling images configured by your selected server source..."
  pull_selfhost_images
  info "Starting U Workers services (this may take a few minutes on first run)..."
  docker compose -f docker-compose.selfhost.yml up -d

  # Read the ports Compose actually published, once, and reuse them for both the
  # health check and the summary so the two can never disagree.
  if ! SELFHOST_BACKEND_PORT="$(compose_published_port backend 8080)"; then
    fail "Started the stack but could not read the backend host port from Docker Compose.
  Check it with: cd $INSTALL_DIR && docker compose -f docker-compose.selfhost.yml ps"
  fi
  if ! SELFHOST_FRONTEND_PORT="$(compose_published_port frontend 3000)"; then
    fail "Started the stack but could not read the frontend host port from Docker Compose.
  Check it with: cd $INSTALL_DIR && docker compose -f docker-compose.selfhost.yml ps"
  fi

  # Wait for health check
  info "Waiting for backend to be ready..."
  local ready=false
  for i in $(seq 1 45); do
    if curl -sf "http://localhost:${SELFHOST_BACKEND_PORT}/health" >/dev/null 2>&1; then
      ready=true
      break
    fi
    sleep 2
  done

  if [ "$ready" = true ]; then
    ok "U Workers server is running"
  else
    warn "Server is still starting. You can check logs with:"
    echo "  cd $INSTALL_DIR && docker compose -f docker-compose.selfhost.yml logs"
    echo ""
  fi
}


# ---------------------------------------------------------------------------
# Main: Default mode (install / upgrade CLI only)
# ---------------------------------------------------------------------------
run_default() {
  printf "\n"
  printf "${BOLD}  U Workers — Installer${RESET}\n"
  printf "\n"

  detect_os
  install_cli

  printf "\n"
  printf "${BOLD}${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}\n"
  printf "${BOLD}${GREEN}  ✓ U Workers CLI is ready!${RESET}\n"
  printf "${BOLD}${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}\n"
  printf "\n"
  printf "  ${BOLD}Next: configure your environment${RESET}\n"
  printf "\n"
  printf "     ${CYAN}\"$CLI_BINARY\" --profile u-workers setup self-host --server-url <YOUR_SERVER> --app-url <YOUR_APP>${RESET}\n"
  printf "\n"
  print_remote_server_token_hint
  printf "  ${BOLD}Self-hosting?${RESET} Install the server first:\n"
  printf "     bash scripts/install.sh --with-server\n"
  printf "\n"
}

# ---------------------------------------------------------------------------
# Main: With-server mode (provision self-host infrastructure + install CLI)
# ---------------------------------------------------------------------------
run_with_server() {
  printf "\n"
  printf "${BOLD}  U Workers — Self-Host Installer${RESET}\n"
  printf "  Provisioning server infrastructure + installing CLI\n"
  printf "\n"

  detect_os
  check_docker
  setup_server
  install_cli

  printf "\n"
  printf "${BOLD}${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}\n"
  printf "${BOLD}${GREEN}  ✓ U Workers server is running and CLI is ready!${RESET}\n"
  printf "${BOLD}${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}\n"
  printf "\n"
  printf "  ${BOLD}Frontend:${RESET}  http://localhost:%s\n" "$SELFHOST_FRONTEND_PORT"
  printf "  ${BOLD}Backend:${RESET}   http://localhost:%s\n" "$SELFHOST_BACKEND_PORT"
  printf "  ${BOLD}Server at:${RESET} %s\n" "$INSTALL_DIR"
  printf "\n"
  printf "  ${BOLD}Next: configure your CLI to connect${RESET}\n"
  printf "\n"
  printf "     ${CYAN}\"$CLI_BINARY\" --profile u-workers setup self-host --server-url http://localhost:$SELFHOST_BACKEND_PORT --app-url http://localhost:$SELFHOST_FRONTEND_PORT${RESET}\n"
  printf "\n"
  printf "  ${BOLD}Login:${RESET} configure ${CYAN}RESEND_API_KEY${RESET} in .env for email codes,\n"
  printf "  or read the generated code from backend logs when Resend is unset.\n"
  printf "\n"
  printf "  ${BOLD}To stop all services:${RESET}\n"
  printf "     bash scripts/install.sh --stop\n"
  printf "\n"
}

# ---------------------------------------------------------------------------
# Stop: shut down a self-hosted installation
# ---------------------------------------------------------------------------
run_stop() {
  printf "\n"
  info "Stopping U Workers services..."

  if [ -d "$INSTALL_DIR" ]; then
    cd "$INSTALL_DIR"
    if [ -f docker-compose.selfhost.yml ]; then
      docker compose -f docker-compose.selfhost.yml down
      ok "Docker services stopped"
    else
      warn "No docker-compose.selfhost.yml found at $INSTALL_DIR"
    fi
  else
    warn "No U Workers installation found at $INSTALL_DIR"
  fi

  if [ -x "$CLI_BINARY" ]; then
    "$CLI_BINARY" --profile u-workers daemon stop 2>/dev/null && ok "Daemon stopped" || true
  fi

  printf "\n"
}

# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------
main() {
  local mode="default"

  while [ $# -gt 0 ]; do
    case "$1" in
      --with-server) mode="with-server" ;;
      --local)       mode="with-server" ;;  # backwards compat alias
      --stop)        mode="stop" ;;
      --help|-h)
        echo "Usage: install.sh [--with-server | --stop]"
        echo ""
        echo "  (default)       Install / upgrade the U Workers CLI"
        echo "  --with-server   Install CLI + provision a self-host server (Docker)"
        echo "  --stop          Stop a self-hosted installation"
        echo ""
        echo "Environment variables:"
        echo "  U_WORKERS_REPO_WEB_URL  Your release repository web URL (required for downloads)"
        echo "  U_WORKERS_REPO_URL      Your source Git URL (required with --with-server)"
        echo "  U_WORKERS_INSTALL_DIR   Self-host server install directory"
        echo "                        (default: \$HOME/.u-workers/server)"
        echo "  U_WORKERS_BIN_DIR       Target directory for the CLI binary when"
        echo "                        installing from GitHub Releases"
        echo "                        (default: \$HOME/.u-workers/bin)"
        echo "  U_WORKERS_SELFHOST_REF  Git ref to check out for self-host assets"
        echo "                          (default: latest release tag, falling back to main)"
        echo ""
        echo "  The legacy MULTICA_* spellings of these variables are still read."
        echo ""
        echo "After installation, run the installed binary with '--profile u-workers setup self-host' to configure your environment."
        exit 0
        ;;
      *) warn "Unknown option: $1" ;;
    esac
    shift
  done

  if [ "$mode" != "stop" ]; then
    [ -n "$REPO_WEB_URL" ] || fail "Set U_WORKERS_REPO_WEB_URL (or MULTICA_REPO_WEB_URL) to your chosen CLI release source."
    if [ "$mode" = "with-server" ]; then
      [ -n "$REPO_URL" ] || fail "Set U_WORKERS_REPO_URL (or MULTICA_REPO_URL) to your source repository."
    fi
  fi

  case "$mode" in
    default)     run_default ;;
    with-server) run_with_server ;;
    stop)        run_stop ;;
  esac
}

main "$@"
