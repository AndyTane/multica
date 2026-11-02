#Requires -Version 5.1
<#
.SYNOPSIS
    Adds the package root CA to this Windows user's certificate store.

.DESCRIPTION
    The same one-time Windows security confirmation as during Setup.cmd. Use it
    when Setup.cmd was run with -SkipBrowserTrust and the browser still warns
    about the server certificate.
#>
[CmdletBinding()]
param([int]$TimeoutSec = 90)

$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
. (Join-Path $root 'portable-common.ps1')

if (-not (Test-Path -LiteralPath (Get-PortablePublicRootCAPath $root))) {
    Write-Host 'root-ca.crt is missing; start the server once (Start.cmd) to generate it.'
    exit 1
}
$result = Install-PortableBrowserTrust -Root $root -TimeoutSec $TimeoutSec
if ($result.Trusted) {
    Write-Host ('Browser now trusts the U Workers root CA (' + $result.Thumbprint + ').')
} else {
    Write-Host 'The certificate was not trusted (the confirmation dialog was not answered). Nothing else was changed.' -ForegroundColor Yellow
    exit 1
}
