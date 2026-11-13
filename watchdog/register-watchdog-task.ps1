# One-time setup: registers the RFID gate watchdog as a Windows Scheduled Task
# that runs watchdog.ps1 every 1 minute, in the current interactive user's
# session (required so the balloon notification is actually visible on screen).
#
# Run this once, in an elevated ("Run as Administrator") PowerShell window:
#   powershell -ExecutionPolicy Bypass -File register-watchdog-task.ps1
#
# To remove it later:
#   Unregister-ScheduledTask -TaskName "RFIDGateWatchdog" -Confirm:$false

$ErrorActionPreference = "Stop"

$TaskName    = "RFIDGateWatchdog"
$ScriptPath  = Join-Path $PSScriptRoot "watchdog.ps1"

if (-not (Test-Path $ScriptPath)) {
    throw "watchdog.ps1 not found next to this script at $ScriptPath"
}

$action = New-ScheduledTaskAction `
    -Execute "powershell.exe" `
    -Argument "-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File `"$ScriptPath`""

$trigger = New-ScheduledTaskTrigger -Once -At (Get-Date) `
    -RepetitionInterval (New-TimeSpan -Minutes 1) `
    -RepetitionDuration (New-TimeSpan -Days 3650)

$principal = New-ScheduledTaskPrincipal `
    -UserId "$env:USERDOMAIN\$env:USERNAME" `
    -LogonType Interactive `
    -RunLevel Limited

$settings = New-ScheduledTaskSettingsSet `
    -MultipleInstances IgnoreNew `
    -ExecutionTimeLimit (New-TimeSpan -Minutes 2) `
    -DontStopOnIdleEnd `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries

if (Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue) {
    Write-Output "Task '$TaskName' already exists - replacing it."
    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
}

Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger `
    -Principal $principal -Settings $settings `
    -Description "Checks RFID gate health (Go middleware, C# reader app, dashboard, Koha) every minute, shows a Windows notification and sends a Telegram message if something is down or frozen." `
    -ErrorAction Stop | Out-Null

Write-Output "Task '$TaskName' registered - runs every 1 minute as $env:USERNAME."
Write-Output "Run it once manually to verify: Start-ScheduledTask -TaskName '$TaskName'"

# ---------------------------------------------------------------------------
# Second task: heal.ps1 restarts a HUNG middleware (service Running but /health
# not answering). Restarting a service needs admin rights, so it runs as SYSTEM;
# that also means this script must be run elevated to register it.
# ---------------------------------------------------------------------------
$HealTaskName = "RFIDGateHeal"
$HealScript   = Join-Path $PSScriptRoot "heal.ps1"
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not (Test-Path $HealScript)) {
    Write-Warning "heal.ps1 not found next to this script; skipping $HealTaskName."
} elseif (-not $isAdmin) {
    Write-Warning "Not running elevated: $HealTaskName (restart of a hung middleware) was NOT registered. Re-run this script from an Administrator PowerShell to add it."
} else {
    $healAction = New-ScheduledTaskAction -Execute "powershell.exe" `
        -Argument "-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File `"$HealScript`""
    $healTrigger = New-ScheduledTaskTrigger -Once -At (Get-Date) `
        -RepetitionInterval (New-TimeSpan -Minutes 1) `
        -RepetitionDuration (New-TimeSpan -Days 3650)
    $healPrincipal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest
    $healSettings = New-ScheduledTaskSettingsSet -MultipleInstances IgnoreNew `
        -ExecutionTimeLimit (New-TimeSpan -Minutes 2) -StartWhenAvailable `
        -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries

    if (Get-ScheduledTask -TaskName $HealTaskName -ErrorAction SilentlyContinue) {
        Unregister-ScheduledTask -TaskName $HealTaskName -Confirm:$false
    }
    Register-ScheduledTask -TaskName $HealTaskName -Action $healAction -Trigger $healTrigger `
        -Principal $healPrincipal -Settings $healSettings `
        -Description "Restarts the RFIDMiddleware service when it is Running but /health has not answered for several minutes." `
        -ErrorAction Stop | Out-Null
    Write-Output "Task '$HealTaskName' registered - runs every 1 minute as SYSTEM."
}
