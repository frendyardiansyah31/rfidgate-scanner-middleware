<#
.SYNOPSIS
  Build rfid-middleware.exe and assemble everything needed for deployment into .\deploy

.DESCRIPTION
  Output folder (recreated on every run):

    deploy\
      rfid-middleware.exe
      config.yaml
      install.bat / uninstall.bat
      DEPLOYMENT_GUIDE.md
      web\dist\...            (dashboard frontend)
      watchdog\...            (only with -IncludeWatchdog; state/log files are never copied)

  edge.db is NOT copied by default: it is created and back-filled automatically
  on first run. Use -IncludeDb to ship the current one.

.EXAMPLE
  .\build-deploy.ps1
  .\build-deploy.ps1 -IncludeWatchdog
  .\build-deploy.ps1 -SkipBuild        # reuse the existing rfid-middleware.exe
#>
[CmdletBinding()]
param(
    [switch]$SkipBuild,
    [switch]$IncludeWatchdog,
    [switch]$IncludeDb,
    # Source of the deployed config.yaml. Default: the Koha template (no secrets).
    # Pass -ConfigFile config.yaml to ship the local one instead.
    [string]$ConfigFile = 'config.koha.example.yaml',
    [string]$OutDir = 'deploy'
)

$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
Set-Location $root

$deploy = Join-Path $root $OutDir
$exe    = Join-Path $root 'rfid-middleware.exe'

function Step($msg) { Write-Host "==> $msg" -ForegroundColor Cyan }
function Need($path, $what) {
    if (-not (Test-Path $path)) { throw "$what not found: $path" }
}

# 1. Build
if (-not $SkipBuild) {
    Step 'go build'
    & go build -o $exe .
    if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
}
Need $exe 'rfid-middleware.exe'

# 2. Required inputs
Need (Join-Path $root $ConfigFile)                   $ConfigFile
Need (Join-Path $root 'install.bat')                 'install.bat'
Need (Join-Path $root 'uninstall.bat')               'uninstall.bat'
Need (Join-Path $root 'web\dist\index.html')         'web\dist\index.html'

# 3. Fresh output folder (refuse to wipe anything outside the repo)
Step "prepare $deploy"
if ((Split-Path -Parent $deploy) -ne $root) { throw "OutDir must be a direct child of the repo: $deploy" }
if (Test-Path $deploy) { Remove-Item $deploy -Recurse -Force }
New-Item -ItemType Directory -Path $deploy | Out-Null

# 4. Copy
Step 'copy files'
Copy-Item $exe                                  $deploy
Copy-Item (Join-Path $root $ConfigFile)         (Join-Path $deploy 'config.yaml')
Copy-Item (Join-Path $root 'install.bat')       $deploy
Copy-Item (Join-Path $root 'uninstall.bat')     $deploy
if (Test-Path (Join-Path $root 'DEPLOYMENT_GUIDE.md')) {
    Copy-Item (Join-Path $root 'DEPLOYMENT_GUIDE.md') $deploy
}

New-Item -ItemType Directory -Path (Join-Path $deploy 'web') | Out-Null
Copy-Item (Join-Path $root 'web\dist') (Join-Path $deploy 'web\dist') -Recurse

if ($IncludeWatchdog) {
    $wd = Join-Path $deploy 'watchdog'
    New-Item -ItemType Directory -Path $wd | Out-Null
    Get-ChildItem (Join-Path $root 'watchdog') -Filter *.ps1 | Copy-Item -Destination $wd
    # Template only: the real telegram.json holds the bot token and is created on the target PC.
    Copy-Item (Join-Path $root 'watchdog\telegram.example.json') $wd
}

if ($IncludeDb) {
    Need (Join-Path $root 'edge.db') 'edge.db'
    Copy-Item (Join-Path $root 'edge.db') $deploy
}

# 5. Summary
Step 'result'
Get-ChildItem $deploy -Recurse -File | ForEach-Object {
    '{0,10:N0} KB  {1}' -f ($_.Length / 1KB), $_.FullName.Substring($deploy.Length + 1)
}

Write-Host ''
Write-Host "Done: $deploy" -ForegroundColor Green
Write-Host 'Next: copy the folder to the target PC (e.g. C:\rfid-middleware), fill koha_api.base_url and koha_api.api_key in config.yaml, run install.bat as Administrator.'
Write-Host 'Note: once the API key is filled in, config.yaml holds a secret - do not commit it or share the folder.' -ForegroundColor Yellow
