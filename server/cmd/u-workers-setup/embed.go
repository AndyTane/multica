//go:build windows

package main

import "embed"

// payloadFS holds the CLI executables this installer ships.
// scripts/build-windows-installer.ps1 stages u-workers.exe and multica.exe here
// before building the installer; the committed payload/README.md is what keeps
// this embed pattern valid for a plain 'go build ./...', so the binaries can
// legitimately be absent and main reports the missing build step instead of
// installing nothing.
//
//go:embed payload/*
var payloadFS embed.FS
