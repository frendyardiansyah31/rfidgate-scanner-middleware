# RFID gate health watchdog.
# Runs once per invocation (scheduled every 1 minute via Task Scheduler - see
# register-watchdog-task.ps1). Checks Go middleware, the C# reader app, the
# Electron dashboard, and SLiMS/internet reachability; appends every cycle to
# watchdog.log regardless of outcome (for debugging after the fact).
#
# Notification channel depends on severity:
#   - Freeze/crash of Go middleware, the C# app, or the dashboard = severe ->
#     blocking MessageBox centered on screen (can't be missed/ignored).
#   - SLiMS/internet unreachable = handled/non-fatal (offline-first) -> toast
#     balloon only, and only after it's been down $SlimsDownThresholdMinutes.
#   - Recovery of any check -> toast (good news never needs to block).

# -NoUi skips the balloon/MessageBox (used by tests and headless runs); logging
# and Telegram are unaffected.
param([switch]$NoUi)

$ErrorActionPreference = "Stop"

# added by frendy start
$AppDir                  = Split-Path -Parent $PSScriptRoot   # folder with the exe, config.yaml, server-info.txt
$DefaultHealthUrl        = "http://127.0.0.1:3000/health"     # last resort if the port cannot be read
$CsharpProcessName      = "RFD1861Demo"
$ElectronProcessName     = "electron"
$ElectronWindowTitleLike = "*RFID Dashboard*"

$GoDownThresholdMinutes    = 2    # delay before alerting that Go is down: the service restarts itself within seconds, so a single failed probe during a restart is not an incident
$GateFailOpenFreshMinutes  = 3    # a fail-open entry in gate-status.txt counts as "happening now" only if its "updated" time is this recent
$SlimsDownThresholdMinutes = 10  # delay before first alert for Koha/internet down (state key stays "slims" for state.json compatibility)
$ReminderMinutes            = 15  # how often to re-notify while still bad (freeze/crash)
$SlimsReminderMinutes       = 30  # how often to re-notify while SLiMS still down

$StateFile = Join-Path $PSScriptRoot "state.json"
$LogFile   = Join-Path $PSScriptRoot "watchdog.log"
# added by frendy end

function Write-Log {
    param([string]$Message)
    $line = "{0}  {1}" -f (Get-Date -Format "yyyy-MM-dd HH:mm:ss"), $Message
    Add-Content -Path $LogFile -Value $line -Encoding utf8
}

. (Join-Path $PSScriptRoot "watchdog-common.ps1")   # Resolve-HealthUrl, Send-Telegram

function Show-Toast {
    param(
        [string]$Title,
        [string]$Message,
        [ValidateSet("Info", "Warning", "Error")][string]$Level = "Error"
    )
    if ($NoUi) { return }
    try {
        Add-Type -AssemblyName System.Windows.Forms
        Add-Type -AssemblyName System.Drawing

        $icon = switch ($Level) {
            "Info"    { [System.Drawing.SystemIcons]::Information }
            "Warning" { [System.Drawing.SystemIcons]::Warning }
            default   { [System.Drawing.SystemIcons]::Error }
        }
        $balloonIcon = switch ($Level) {
            "Info"    { [System.Windows.Forms.ToolTipIcon]::Info }
            "Warning" { [System.Windows.Forms.ToolTipIcon]::Warning }
            default   { [System.Windows.Forms.ToolTipIcon]::Error }
        }

        $notify = New-Object System.Windows.Forms.NotifyIcon
        $notify.Icon = $icon
        $notify.Visible = $true
        $notify.BalloonTipTitle = $Title
        $notify.BalloonTipText = $Message
        $notify.BalloonTipIcon = $balloonIcon
        $notify.ShowBalloonTip(10000)
        Start-Sleep -Seconds 2
        $notify.Dispose()
    } catch {
        Write-Log "FAILED to show toast: $($_.Exception.Message)"
    }
}

# Blocking MessageBox, centered on screen and forced on top - used only for
# severe cases (freeze/crash of Go, the C# reader app, or the dashboard), so
# staff standing at the gate PC can't miss it the way a balloon can be missed.
# Task Scheduler's ExecutionTimeLimit (see register-watchdog-task.ps1) kills
# the script if this sits un-dismissed too long, so a missed box can't wedge
# the watchdog forever.
function Show-AlertBox {
    param(
        [string]$Title,
        [string]$Message
    )
    if ($NoUi) { return }
    try {
        Add-Type -AssemblyName System.Windows.Forms

        $owner = New-Object System.Windows.Forms.Form
        $owner.TopMost = $true
        $owner.StartPosition = "CenterScreen"
        $owner.ShowInTaskbar = $false
        $owner.Size = New-Object System.Drawing.Size(0, 0)
        $owner.Show()
        $owner.Hide()

        [System.Windows.Forms.MessageBox]::Show(
            $owner,
            $Message,
            $Title,
            [System.Windows.Forms.MessageBoxButtons]::OK,
            [System.Windows.Forms.MessageBoxIcon]::Error
        ) | Out-Null

        $owner.Dispose()
    } catch {
        Write-Log "FAILED to show alert box: $($_.Exception.Message)"
    }
}

function New-DefaultCheckState {
    [PSCustomObject]@{ status = "ok"; since = $null; lastAlertAt = $null }
}

function New-DefaultState {
    [PSCustomObject]@{
        go       = New-DefaultCheckState
        slims    = New-DefaultCheckState
        csharp   = New-DefaultCheckState
        electron = [PSCustomObject]@{ status = "ok"; since = $null; lastAlertAt = $null; everSeen = $false }
        gate     = New-DefaultCheckState
    }
}

function Load-State {
    if (-not (Test-Path $StateFile)) {
        return New-DefaultState
    }
    try {
        $raw = Get-Content -Path $StateFile -Raw
        $loaded = $raw | ConvertFrom-Json
        # Guard against a partially-written/corrupt state file from a previous run
        if (-not $loaded.go -or -not $loaded.slims -or -not $loaded.csharp -or -not $loaded.electron) {
            throw "state file missing expected fields"
        }
        # state.json files written before the gate check existed have no "gate" entry
        if (-not $loaded.gate) {
            $loaded | Add-Member -NotePropertyName gate -NotePropertyValue (New-DefaultCheckState)
        }
        return $loaded
    } catch {
        Write-Log "state file unreadable ($($_.Exception.Message)), starting fresh"
        return New-DefaultState
    }
}

function Save-State {
    param($State)
    $State | ConvertTo-Json -Depth 6 | Set-Content -Path $StateFile -Encoding utf8
}

# Handles the alert/reminder/recovery transition logic shared by every check,
# so it's written once instead of duplicated per check (DRY).
function Update-CheckState {
    param(
        [Parameter(Mandatory)] $Sub,
        [Parameter(Mandatory)][bool]$IsBad,
        [Parameter(Mandatory)][string]$Name,
        [string]$BadMessage,
        [int]$DelayMinutes = 0,
        [int]$ReminderEveryMinutes = 15,
        # Critical = freeze/fatal (Go, C# app, dashboard down/unresponsive) -> blocking
        # centered MessageBox, impossible to miss. Warning = degraded-but-still-working
        # state (SLiMS/internet down) -> non-blocking toast, matches how the system is
        # designed to keep functioning offline. Both always get logged either way.
        [ValidateSet("Critical", "Warning")][string]$Severity = "Critical"
    )

    $now = Get-Date

    if ($IsBad) {
        if ($Sub.status -ne "bad") {
            $Sub.status = "bad"
            $Sub.since = $now.ToString("o")
        }

        $sinceTime = [datetime]::Parse($Sub.since)
        $elapsedMinutes = ($now - $sinceTime).TotalMinutes

        if ($elapsedMinutes -ge $DelayMinutes) {
            $shouldAlert = $true
            if ($Sub.lastAlertAt) {
                $lastAlert = [datetime]::Parse($Sub.lastAlertAt)
                $shouldAlert = ($now - $lastAlert).TotalMinutes -ge $ReminderEveryMinutes
            }

            if ($shouldAlert) {
                # Telegram FIRST: the critical MessageBox below blocks until someone
                # clicks it, and the whole point of Telegram is reaching people who
                # are not standing at the gate PC.
                $tag = if ($Severity -eq "Critical") { "[DOWN]" } else { "[WARN]" }
                $sinceLocal = $sinceTime.ToString("yyyy-MM-dd HH:mm")
                $null = Send-Telegram -Text ("{0} {1}: {2} (sejak {3}, {4} menit)" -f $tag, $Name, $BadMessage, $sinceLocal, [int]$elapsedMinutes)

                if ($Severity -eq "Critical") {
                    Show-AlertBox -Title "RFID Gate - $Name Bermasalah" -Message $BadMessage
                } else {
                    Show-Toast -Title "RFID Gate - $Name Bermasalah" -Message $BadMessage -Level "Warning"
                }
                Write-Log "ALERT [$Name] $BadMessage (bad since $($Sub.since))"
                $Sub.lastAlertAt = $now.ToString("o")
            }
        }
    } else {
        if ($Sub.status -eq "bad") {
            # Recovery is good news either way - never worth blocking staff with a modal for it.
            # Only tell Telegram about a recovery if the problem was alerted there at all
            # (a blip shorter than the alert delay never produced a [DOWN] message).
            if ($Sub.lastAlertAt) {
                $null = Send-Telegram -Text ("[PULIH] {0} sudah normal kembali (sempat bermasalah sejak {1})." -f $Name, ([datetime]::Parse($Sub.since)).ToString("yyyy-MM-dd HH:mm"))
            }
            Show-Toast -Title "RFID Gate - $Name Pulih" -Message "$Name sudah normal kembali." -Level "Info"
            Write-Log "RECOVERED [$Name] (was bad since $($Sub.since))"
        }
        $Sub.status = "ok"
        $Sub.since = $null
        $Sub.lastAlertAt = $null
    }

    return $Sub
}

try {
    $state = Load-State

    # --- Check 1: Go middleware + library (Koha) reachability (via Go's own /health) ---
    # /health reports Koha's reachability in "slims_api" (the key keeps its old name
    # because the dashboards read it).
    $goBad = $false
    $slimsBad = $false
    $slimsMsg = "Koha/internet tidak bisa dijangkau."
    $HealthUrl = Resolve-HealthUrl
    try {
        $resp = Invoke-RestMethod -Uri $HealthUrl -TimeoutSec 5 -Method Get
        $libraryState = [string]$resp.data.slims_api
        if ($libraryState -like "unreachable*" -or $libraryState -like "error*") {
            $slimsBad = $true
            $slimsMsg = "Koha/internet tidak bisa dijangkau: $libraryState"
        }
    } catch {
        $goBad = $true
        Write-Log "health check failed at $HealthUrl : $($_.Exception.Message)"
    }

    $state.go = Update-CheckState -Sub $state.go -IsBad $goBad -Name "Go Middleware" `
        -BadMessage "Service Go middleware tidak merespons (kemungkinan crash/freeze)." `
        -DelayMinutes $GoDownThresholdMinutes -ReminderEveryMinutes $ReminderMinutes -Severity "Critical"

    # Only meaningful to evaluate Koha reachability if Go itself answered.
    # Koha/internet down is a handled, non-fatal degraded state (the offline mirror
    # keeps deciding), so it stays a toast, not a blocking box.
    if (-not $goBad) {
        $state.slims = Update-CheckState -Sub $state.slims -IsBad $slimsBad -Name "Koneksi Koha" `
            -BadMessage $slimsMsg -DelayMinutes $SlimsDownThresholdMinutes -ReminderEveryMinutes $SlimsReminderMinutes `
            -Severity "Warning"
    }

    # --- Check 2: C# reader app ---
    $csharpProc = Get-Process -Name $CsharpProcessName -ErrorAction SilentlyContinue | Select-Object -First 1
    $csharpBad = $false
    $csharpMsg = ""
    if (-not $csharpProc) {
        $csharpBad = $true
        $csharpMsg = "Aplikasi reader RFID ($CsharpProcessName) tidak berjalan."
    } elseif (-not $csharpProc.Responding) {
        $csharpBad = $true
        $csharpMsg = "Aplikasi reader RFID ($CsharpProcessName) tidak merespons (freeze)."
    }
    $state.csharp = Update-CheckState -Sub $state.csharp -IsBad $csharpBad -Name "Aplikasi Reader RFID" `
        -BadMessage $csharpMsg -DelayMinutes 0 -ReminderEveryMinutes $ReminderMinutes -Severity "Critical"

    # --- Check 3: Electron dashboard ---
    # Only monitored once seen running at least once - the dashboard isn't
    # necessarily expected to always be open at the gate PC, so its absence
    # alone shouldn't page anyone; going from open -> unresponsive/closed should.
    if (-not (Get-Member -InputObject $state.electron -Name "everSeen" -MemberType Properties)) {
        $state.electron | Add-Member -NotePropertyName everSeen -NotePropertyValue $false
    }

    $electronProc = Get-Process -Name $ElectronProcessName -ErrorAction SilentlyContinue |
        Where-Object { $_.MainWindowTitle -like $ElectronWindowTitleLike } | Select-Object -First 1

    if ($electronProc -and -not $state.electron.everSeen) {
        $state.electron.everSeen = $true
    }

    if ($state.electron.everSeen) {
        $electronBad = $false
        $electronMsg = ""
        if (-not $electronProc) {
            $electronBad = $true
            $electronMsg = "Dashboard RFID (Electron) tertutup/berhenti."
        } elseif (-not $electronProc.Responding) {
            $electronBad = $true
            $electronMsg = "Dashboard RFID (Electron) tidak merespons (freeze)."
        }
        # Update-CheckState only touches status/since/lastAlertAt, so everSeen survives intact
        $state.electron = Update-CheckState -Sub $state.electron -IsBad $electronBad -Name "Dashboard RFID" `
            -BadMessage $electronMsg -DelayMinutes 0 -ReminderEveryMinutes $ReminderMinutes -Severity "Critical"
    }

    # --- Check 4: the gate app is FAIL-OPEN (middleware AND Koha both unreachable from the gate) ---
    # The gate app (GateDecisionClient.cs) writes %ProgramData%\RFIDGate\gate-status.txt:
    #   mode=normal | koha-direct | fail-open, plus since= / updated= timestamps. "updated" is refreshed
    # while scans keep arriving in a degraded mode, so a recent fail-open means people are walking
    # through right now WITHOUT any book check. koha-direct is still a correct decision (only Go is
    # down, which Check 1 already reports), so just fail-open is raised here.
    $gateStatusFile = $env:RFID_GATE_STATUS_FILE
    if (-not $gateStatusFile) {
        $gateStatusFile = Join-Path ([Environment]::GetFolderPath("CommonApplicationData")) "RFIDGate\gate-status.txt"
    }
    $gateBad = $false
    $gateMsg = ""
    if (Test-Path $gateStatusFile) {
        try {
            $kv = @{}
            foreach ($line in (Get-Content -Path $gateStatusFile)) {
                if ($line -match '^(\w+)=(.*)$') { $kv[$Matches[1]] = $Matches[2].Trim() }
            }
            if ($kv["mode"] -eq "fail-open" -and $kv["updated"]) {
                $updated = [datetime]::ParseExact($kv["updated"], "yyyy-MM-dd HH:mm:ss", [Globalization.CultureInfo]::InvariantCulture)
                if (((Get-Date) - $updated).TotalMinutes -le $GateFailOpenFreshMinutes) {
                    $gateBad = $true
                    $gateMsg = "Gate meloloskan buku TANPA pengecekan (middleware dan Koha tidak terjangkau dari aplikasi gate), sejak $($kv['since'])."
                }
            }
        } catch {
            Write-Log "gate status file unreadable: $($_.Exception.Message)"
        }
    }
    $state.gate = Update-CheckState -Sub $state.gate -IsBad $gateBad -Name "Gate Tanpa Pengecekan" `
        -BadMessage $gateMsg -DelayMinutes 0 -ReminderEveryMinutes $ReminderMinutes -Severity "Critical"

    Save-State $state

    Write-Log ("check: go={0} slims={1} csharp={2} electron={3} gate={4}" -f `
        $state.go.status, $state.slims.status, $state.csharp.status, $state.electron.status, $state.gate.status)

} catch {
    Write-Log "WATCHDOG SCRIPT ERROR: $($_.Exception.Message)"
}
