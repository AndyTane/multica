$ErrorActionPreference = 'Stop'
$env:GOMAXPROCS = '2'
Set-Location (Join-Path $PSScriptRoot '../server')

# cmd/server's TestMain opens a database and skips the whole suite when the
# database is absent. Select production files plus these fake-only tests so
# verification neither writes deployment data nor silently skips unit tests.
$source = @(Get-ChildItem cmd/server/*.go | Where-Object { $_.Name -notlike '*_test.go' } | ForEach-Object { './cmd/server/' + $_.Name })
& go test -p 2 -count=1 @source ./cmd/server/router_local_policy_test.go
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

# Keep collector protocol tests; exclude the explicit Postgres integration suite.
$telemetry = @(Get-ChildItem internal/selfhosttelemetry/*.go | Where-Object { $_.Name -ne 'postgres_test.go' } | ForEach-Object { './internal/selfhosttelemetry/' + $_.Name })
& go test -p 2 -count=1 @telemetry
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$storageArgs = @()
if ([System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT) {
    # Windows reports 0666 rather than POSIX 0644. This does not verify Unix
    # permission bits; Linux runs must retain the original permission assertion.
    $storageArgs = @('-skip', '^TestLocalStorageUploadsAreWorldReadable$')
    Write-Host 'Windows: skipping POSIX permission-bit assertion TestLocalStorageUploadsAreWorldReadable'
}
& go test -p 2 -count=1 @storageArgs ./internal/storage ./internal/cloudruntime ./pkg/composio ./internal/integrations/composio
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

& go test -p 2 -count=1 ./internal/service/email.go ./internal/service/email_test.go ./internal/service/email_local_policy_test.go
exit $LASTEXITCODE
