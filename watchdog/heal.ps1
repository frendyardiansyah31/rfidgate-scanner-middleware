# Self-healing for a HUNG middleware: the service is Running but /health does not
# answer. Windows service recovery only reacts to a process that EXITS, so a
# deadlocked or frozen process would otherwise stay broken until someone notices.
#
# Runs once per invocation as SYSTEM (restarting a service needs admin rights),
# every minute, via the RFIDGateHeal scheduled task (see register-watchdog-task.ps1).
# watchdog.ps1 stays a plain user task because it shows on-screen alerts.
#
# Safety rules:
#   - Only acts when the service is Running. A service somebody stopped on purpose
#     (maintenance) is left alone; watchdog.ps1 still alerts about it.
#   - Waits for $FailMinutes of CONSECUTIVE failed probes before restarting.
#   - At most $MaxRestartsPerHour restarts per hour, so a middleware that cannot
#     start (bad config, port taken) is not restarted in a loop; it tells Telegram once.
#
# -DryRun logs what it would do without restarting (used by tests).
param(
    [string]$ServiceName = "RFIDMiddleware",
    [int]$FailMinutes = 3,
    [int]$MaxRestartsPerHour = 3,
    [switch]$DryRun
)

$ErrorActionPreference = "Stop"

$AppDir           = Split-Path -Parent $PSScriptRoot
$DefaultHealthUrl = "http://127.0.0.1:3000/health"
$StateFile        = Join-Path $PSScriptRoot "heal-state.json"
$LogFile          = Join-Path $PSScriptRoot "heal.log"

function Write-Log {
    param([string]$Message)
    $line = "{0}  {1}" -f (Get-Date -Format "yyyy-MM-dd HH:mm:ss"), $Message
    Add-Content -Path $LogFile -Value $line -Encoding utf8
}

. (Join-Path $PSScriptRoot "watchdog-common.ps1")   # Resolve-HealthUrl, Send-Telegram

function Load-State {
    $s = [PSCustomObject]@{ failingSince = $null; restarts = @(); limitNoticeAt = $null }
    if (Test-Path $StateFile) {
        try {
            $j = Get-Content -Path $StateFile -Raw | ConvertFrom-Json
            if ($j.failingSince)  { $s.failingSince  = [string]$j.failingSince }
            if ($j.restarts)      { $s.restarts      = @($j.restarts | ForEach-Object { [string]$_ }) }
            if ($j.limitNoticeAt) { $s.limitNoticeAt = [string]$j.limitNoticeAt }
        } catch {
            Write-Log "state file unreadable ($($_.Exception.Message)), starting fresh"
        }
    }
    return $s
}

function Save-State($s) { $s | ConvertTo-Json -Depth 4 | Set-Content -Path $StateFile -Encoding utf8 }

try {
    $state = Load-State
    $now   = Get-Date

    $healthy = $true
    $url = Resolve-HealthUrl
    try {
        $null = Invoke-RestMethod -Uri $url -TimeoutSec 5 -Method Get
    } catch {
        $healthy = $false
    }

    if ($healthy) {
        if ($state.failingSince) { Write-Log "healthy again (was failing since $($state.failingSince))" }
        $state.failingSince = $null
        Save-State $state
        return
    }

    if (-not $state.failingSince) { $state.failingSince = $now.ToString("o") }
    $failingMinutes = ($now - [datetime]::Parse($state.failingSince)).TotalMinutes
    Write-Log ("health probe failed ({0:N1} min of consecutive failures) at {1}" -f $failingMinutes, $url)

    if ($failingMinutes -lt $FailMinutes) { Save-State $state; return }

    $svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if (-not $svc) { Write-Log "service $ServiceName not found, nothing to heal"; Save-State $state; return }
    if ($svc.Status -ne "Running") {
        Write-Log "service is $($svc.Status), not Running: leaving it alone (stopped on purpose, or Windows recovery is already restarting it)"
        Save-State $state
        return
    }

    $cutoff = $now.AddHours(-1)
    $recent = @($state.restarts | Where-Object { [datetime]::Parse($_) -gt $cutoff })
    if ($recent.Count -ge $MaxRestartsPerHour) {
        $noticeDue = (-not $state.limitNoticeAt) -or (($now - [datetime]::Parse($state.limitNoticeAt)).TotalMinutes -ge 30)
        Write-Log "restart limit reached ($($recent.Count) in the last hour), not restarting again"
        if ($noticeDue) {
            $null = Send-Telegram -Text ("[HEAL] Middleware masih tidak merespons setelah {0} restart otomatis dalam 1 jam. Perlu dicek manual." -f $recent.Count)
            $state.limitNoticeAt = $now.ToString("o")
        }
        $state.restarts = $recent
        Save-State $state
        return
    }

    if ($DryRun) {
        Write-Log "DRYRUN: would restart $ServiceName now"
    } else {
        Write-Log "RESTARTING $ServiceName (health failing for $([int]$failingMinutes) min)"
        Restart-Service -Name $ServiceName -Force
        $null = Send-Telegram -Text ("[HEAL] Middleware tidak merespons {0} menit, service {1} direstart otomatis." -f [int]$failingMinutes, $ServiceName)
    }
    $state.restarts = @($recent) + $now.ToString("o")
    $state.failingSince = $null
    Save-State $state

} catch {
    Write-Log "HEAL SCRIPT ERROR: $($_.Exception.Message)"
}
