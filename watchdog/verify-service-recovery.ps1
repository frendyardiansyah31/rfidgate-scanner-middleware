# Verifies that the RFIDMiddleware Windows service heals itself after a crash.
# Run from an ELEVATED PowerShell, on a PC where install.bat has been run:
#
#   powershell -ExecutionPolicy Bypass -File .\verify-service-recovery.ps1
#
# It (1) prints the configured recovery actions and checks them, then (2) kills the
# service process the hard way (like a crash) and measures how long until the
# service is Running again and /health answers. Add -SkipKill to only do step 1.
param(
    [string]$ServiceName = "RFIDMiddleware",
    [int]$TimeoutSeconds = 90,
    [switch]$SkipKill
)

$ErrorActionPreference = "Stop"
$AppDir = Split-Path -Parent $PSScriptRoot
$DefaultHealthUrl = "http://127.0.0.1:3000/health"
function Write-Log { param([string]$Message) }   # watchdog-common expects it; nothing to log here
. (Join-Path $PSScriptRoot "watchdog-common.ps1")

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) { throw "Run this from an elevated (Administrator) PowerShell." }

$svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if (-not $svc) { throw "Service $ServiceName is not installed. Run install.bat first." }

Write-Host "--- 1. configuration" -ForegroundColor Cyan
$cfg = (& sc.exe qc $ServiceName) -join "`n"
$fail = (& sc.exe qfailure $ServiceName) -join "`n"
$flag = (& sc.exe qfailureflag $ServiceName) -join "`n"
Write-Host $fail
$ok = $true
if ($cfg -notmatch "AUTO_START") { Write-Host "FAIL: start type is not AUTO_START" -ForegroundColor Red; $ok = $false }
if ($fail -notmatch "RESTART") { Write-Host "FAIL: no RESTART recovery action configured" -ForegroundColor Red; $ok = $false }
if ($flag -notmatch "TRUE|: 1") { Write-Host "FAIL: failure flag is not set (service would not be restarted after a non-crash error exit)" -ForegroundColor Red; $ok = $false }
if ($ok) { Write-Host "OK: auto-start + restart-on-failure configured" -ForegroundColor Green }

if ($SkipKill) { return }

Write-Host ""
Write-Host "--- 2. crash test" -ForegroundColor Cyan
if ($svc.Status -ne "Running") { Start-Service $ServiceName; Start-Sleep -Seconds 3 }
$procName = "rfid-middleware"
$p = Get-Process -Name $procName -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $p) { throw "Service is Running but no $procName process found." }
$oldPid = $p.Id

$t0 = Get-Date
Stop-Process -Id $oldPid -Force
Write-Host ("killed pid {0} at {1:HH:mm:ss.fff}" -f $oldPid, $t0)

$url = Resolve-HealthUrl
$serviceBack = $null; $healthBack = $null
while (((Get-Date) - $t0).TotalSeconds -lt $TimeoutSeconds) {
    $np = Get-Process -Name $procName -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $serviceBack -and $np -and $np.Id -ne $oldPid) { $serviceBack = (Get-Date) - $t0 }
    if ($serviceBack) {
        try { Invoke-RestMethod -Uri $url -TimeoutSec 2 | Out-Null; $healthBack = (Get-Date) - $t0; break } catch { }
    }
    Start-Sleep -Milliseconds 250
}

if ($healthBack) {
    Write-Host ("OK: new process after {0:N1}s, /health answering after {1:N1}s" -f $serviceBack.TotalSeconds, $healthBack.TotalSeconds) -ForegroundColor Green
} elseif ($serviceBack) {
    Write-Host ("WARN: process came back after {0:N1}s but /health did not answer within {1}s" -f $serviceBack.TotalSeconds, $TimeoutSeconds) -ForegroundColor Yellow
} else {
    Write-Host ("FAIL: service did not come back within {0}s" -f $TimeoutSeconds) -ForegroundColor Red
}
