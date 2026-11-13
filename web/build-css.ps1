# Builds web/dist/app.css with the Tailwind v4 standalone CLI (no Node needed).
# The CLI (~120MB) is downloaded once into web/tools/ and is gitignored.
#   .\build-css.ps1          one-off minified build
#   .\build-css.ps1 -Watch   rebuild on every change
param([switch]$Watch)

$ErrorActionPreference = 'Stop'
$version = 'v4.2.4'
$cli     = Join-Path $PSScriptRoot 'tools\tailwindcss.exe'

if (-not (Test-Path $cli)) {
    New-Item -ItemType Directory -Force (Split-Path $cli) | Out-Null
    $url = "https://github.com/tailwindlabs/tailwindcss/releases/download/$version/tailwindcss-windows-x64.exe"
    Write-Host "Downloading Tailwind CLI $version ..."
    Invoke-WebRequest -Uri $url -OutFile $cli
}

$cliArgs = @('-i', (Join-Path $PSScriptRoot 'src\input.css'), '-o', (Join-Path $PSScriptRoot 'dist\app.css'))
if ($Watch) { $cliArgs += '--watch' } else { $cliArgs += '--minify' }

& $cli @cliArgs
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
