# Shared helpers for watchdog.ps1 and heal.ps1 (dot-sourced; ASCII only on purpose,
# Windows PowerShell 5.1 reads BOM-less files as ANSI).
#
# Expects the caller to define: $AppDir, $DefaultHealthUrl, and a Write-Log function.

# Works out where the Go middleware listens, so a port change in config.yaml
# does not leave the watchdog probing the wrong port. Order:
#   1. server-info.txt  - what the running service actually bound ("Bind : host:port")
#   2. config.yaml      - server.address / server.port (Go defaults: 127.0.0.1 / 8080)
#   3. $DefaultHealthUrl
function Resolve-HealthUrl {
    $hostPort = $null

    $infoFile = Join-Path $AppDir "server-info.txt"
    if (Test-Path $infoFile) {
        $m = Select-String -Path $infoFile -Pattern '^Bind\s*:\s*(\S+)' | Select-Object -First 1
        if ($m) { $hostPort = $m.Matches[0].Groups[1].Value }
    }

    if (-not $hostPort) {
        $cfgFile = Join-Path $AppDir "config.yaml"
        if (Test-Path $cfgFile) {
            $text  = Get-Content $cfgFile -Raw
            $block = [regex]::Match($text, '(?m)^server:[ \t]*\r?\n((?:[ \t]+.*(?:\r?\n|$))+)').Groups[1].Value
            $addr  = [regex]::Match($block, '(?m)^\s*address:\s*"?([^"\s#]+)').Groups[1].Value
            $port  = [regex]::Match($block, '(?m)^\s*port:\s*(\d+)').Groups[1].Value
            if (-not $addr) { $addr = "127.0.0.1" }
            if (-not $port) { $port = "8080" }
            $hostPort = "${addr}:${port}"
        }
    }

    if (-not $hostPort) { return $DefaultHealthUrl }

    $idx = $hostPort.LastIndexOf(":")
    $addr = $hostPort.Substring(0, $idx)
    $port = $hostPort.Substring($idx + 1)
    # A wildcard bind is reachable locally via loopback.
    if ($addr -in @("", "0.0.0.0", "::", "[::]")) { $addr = "127.0.0.1" }
    return "http://${addr}:${port}/health"
}

# Telegram settings live in telegram.json next to the scripts (NOT in the repo:
# it holds the bot token; see telegram.example.json). Environment variables
# RFID_TG_TOKEN / RFID_TG_CHAT_ID / RFID_TG_MACHINE override the file.
# Returns $null when Telegram is not configured, which simply disables it.
function Get-TelegramConfig {
    $cfg = [ordered]@{ bot_token = ""; chat_id = ""; machine_name = $env:COMPUTERNAME; api_base = "https://api.telegram.org" }

    $file = Join-Path $PSScriptRoot "telegram.json"
    if (Test-Path $file) {
        try {
            $j = Get-Content -Path $file -Raw | ConvertFrom-Json
            foreach ($k in @("bot_token", "chat_id", "machine_name", "api_base")) {
                if ($j.PSObject.Properties.Name -contains $k -and $j.$k) { $cfg[$k] = [string]$j.$k }
            }
        } catch {
            Write-Log "telegram.json unreadable: $($_.Exception.Message)"
        }
    }
    if ($env:RFID_TG_TOKEN)   { $cfg.bot_token    = $env:RFID_TG_TOKEN }
    if ($env:RFID_TG_CHAT_ID) { $cfg.chat_id      = $env:RFID_TG_CHAT_ID }
    if ($env:RFID_TG_MACHINE) { $cfg.machine_name = $env:RFID_TG_MACHINE }

    if (-not $cfg.bot_token -or -not $cfg.chat_id) { return $null }
    return $cfg
}

# Sends one Telegram message. Never throws: a failed notification must not break
# the watchdog. The bot token is never written to the log (it is part of the URL,
# so error text is scrubbed too). Returns $true on success.
function Send-Telegram {
    param([Parameter(Mandatory)][string]$Text)

    $cfg = Get-TelegramConfig
    if (-not $cfg) { return $false }

    try {
        # Windows PowerShell 5.1 on older Windows defaults to TLS 1.0, which Telegram rejects.
        [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    } catch { }

    $prefix = "[{0}] " -f $cfg.machine_name
    $payload = @{ chat_id = $cfg.chat_id; text = ($prefix + $Text) } | ConvertTo-Json -Compress
    $bytes   = [System.Text.Encoding]::UTF8.GetBytes($payload)
    $url     = "{0}/bot{1}/sendMessage" -f $cfg.api_base.TrimEnd("/"), $cfg.bot_token

    try {
        $resp = Invoke-RestMethod -Uri $url -Method Post -Body $bytes `
            -ContentType "application/json; charset=utf-8" -TimeoutSec 10
        if ($resp.ok -eq $false) {
            Write-Log ("telegram: API said not ok: {0}" -f ($resp.description -replace [regex]::Escape($cfg.bot_token), "***"))
            return $false
        }
        Write-Log "telegram: sent"
        return $true
    } catch {
        $msg = $_.Exception.Message -replace [regex]::Escape($cfg.bot_token), "***"
        Write-Log "telegram: send failed: $msg"
        return $false
    }
}
