#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# Load definitions only. All executable and container operations below are fakes.
unset MULTICA_INSTALL_DIR MULTICA_BIN_DIR
source <(sed '/^main "\$@"$/d' "$ROOT_DIR/scripts/install.sh")
[[ "$INSTALL_DIR" == "$HOME/.u-workers/server" ]]
[[ "$CLI_BINARY" == "$HOME/.u-workers/bin/multica" ]]

docker_calls=""
fake_images=$'u-workers-backend:dev\nu-workers-web:dev\npostgres:16'
docker() {
  if [[ "$*" == *"config --images"* ]]; then
    printf '%s\n' "$fake_images"
  else
    docker_calls+="$*;"
  fi
}
pull_selfhost_images
[[ "$docker_calls" == *"docker-compose.selfhost.build.yml build backend frontend"* ]]
[[ "$docker_calls" != *" pull"* ]]

docker_calls=""
fake_images=$'u-workers-backend:dev\nregistry.example/web:chosen'
pull_selfhost_images
[[ "$docker_calls" == *" build backend;"* ]]
[[ "$docker_calls" != *"frontend"* ]]
[[ "$docker_calls" != *" pull"* ]]

docker_calls=""
fake_images=$'registry.example/backend:chosen\nregistry.example/web:chosen'
pull_selfhost_images
[[ "$docker_calls" == *"docker-compose.selfhost.yml pull;"* ]]
echo "install source tests passed"
