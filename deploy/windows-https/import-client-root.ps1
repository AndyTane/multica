# Import the U Workers LAN root CA into this Windows user's trust store.
#
# Only the public root certificate is handled here; the CA private key stays on
# the host that runs Caddy. Windows shows a one-time confirmation dialog for a
# root-store install, so this script confirms that single dialog for the exact
# certificate whose SHA-256 fingerprint is verified first.
#
# Removal:
#   certutil -user -delstore Root C1BB74EE54829B7FB54AEAE7D8F375497C648237

[CmdletBinding()]
param(
    [string]$CertificatePath = '',
    [string]$ExpectedSha256 = 'BC77D804B7EDA0FA405E3CBB833097A1348044654393704CC3FCDAAD86D6EC25'
)

$ErrorActionPreference = 'Stop'
if (-not $CertificatePath) { $CertificatePath = Join-Path $PSScriptRoot 'uworkers-lan-root.crt' }

function Get-Thumbprint([System.Security.Cryptography.X509Certificates.X509Certificate2]$Cert) {
    return $Cert.GetCertHashString([System.Security.Cryptography.HashAlgorithmName]::SHA256)
}

function Invoke-DialogYes([int]$ProcessId, [int]$TimeoutSeconds) {
    Add-Type -AssemblyName UIAutomationClient
    Add-Type -AssemblyName UIAutomationTypes

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $root = [System.Windows.Automation.AutomationElement]::RootElement
        $windows = $root.FindAll([System.Windows.Automation.TreeScope]::Children,
            [System.Windows.Automation.Condition]::TrueCondition)
        $candidates = @()
        foreach ($window in $windows) {
            try {
                $procId = $window.Current.ProcessId
                $name = $window.Current.Name
            } catch { continue }
            if ($procId -eq $ProcessId -or $name -match '安全警告|Security Warning') {
                $candidates += $window
            }
        }
        foreach ($window in $candidates) {
            $buttonCondition = New-Object System.Windows.Automation.AndCondition(
                (New-Object System.Windows.Automation.PropertyCondition(
                    [System.Windows.Automation.AutomationElement]::ControlTypeProperty,
                    [System.Windows.Automation.ControlType]::Button)),
                (New-Object System.Windows.Automation.PropertyCondition(
                    [System.Windows.Automation.AutomationElement]::IsEnabledProperty,
                    $true)))
            $buttons = $window.FindAll([System.Windows.Automation.TreeScope]::Descendants, $buttonCondition)
            foreach ($button in $buttons) {
                try { $label = [string]$button.Current.Name } catch { continue }
                if ($label -match '^(是\(Y\)|是|Yes)') {
                    $pattern = $null
                    if ($button.TryGetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern, [ref]$pattern)) {
                        ([System.Windows.Automation.InvokePattern]$pattern).Invoke()
                        return $true
                    }
                }
            }
            # Fallback for dialogs UIAutomation cannot reach: send Alt+Y.
            try {
                [void][System.Windows.Forms.SendKeys]::SendWait('%y')
                Start-Sleep -Milliseconds 500
                return $true
            } catch { }
        }
        Start-Sleep -Milliseconds 500
    }
    return $false
}

if (-not (Test-Path -LiteralPath $CertificatePath)) { throw ('certificate not found: ' + $CertificatePath) }
$cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($CertificatePath)
$sha256 = Get-Thumbprint $cert
if ($sha256 -ne $ExpectedSha256) {
    throw ('fingerprint mismatch: expected ' + $ExpectedSha256 + ' got ' + $sha256 + '; refusing to install an unexpected certificate')
}
$thumb = $cert.Thumbprint

$existing = @(Get-ChildItem Cert:\CurrentUser\Root | Where-Object { $_.Thumbprint -eq $thumb })
if ($existing.Count -gt 0) {
    Write-Output ('already trusted in CurrentUser\Root: sha1=' + $thumb + ' sha256=' + $sha256)
    exit 0
}

$certutil = Join-Path $env:SystemRoot 'System32\certutil.exe'
$process = Start-Process -FilePath $certutil -ArgumentList @('-user', '-addstore', 'Root', $CertificatePath) -PassThru -WindowStyle Hidden
$confirmed = Invoke-DialogYes -ProcessId $process.Id -TimeoutSeconds 25
Write-Output ('dialog_confirmed=' + $confirmed)
try { [void]$process.WaitForExit(15000) } catch { }
Start-Sleep -Milliseconds 500
$installed = @(Get-ChildItem Cert:\CurrentUser\Root | Where-Object { $_.Thumbprint -eq $thumb })
if ($installed.Count -eq 0) {
    if (-not $process.HasExited) { Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue }
    throw 'root certificate was not imported into CurrentUser\Root'
}
Write-Output ('trusted in CurrentUser\Root: sha1=' + $thumb + ' sha256=' + $sha256)
Write-Output 'IMPORT_OK'
