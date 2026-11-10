#!/bin/sh
# Windows Git helper; credential values travel only over Git's stdin/stdout.
helper_path=$(cygpath -w "$(dirname "$0")/git-cred-ghtoken.ps1") || exit 1
exec powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$helper_path" "$@"
