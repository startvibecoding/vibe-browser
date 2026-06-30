# Example: Screenshot workflow (PowerShell)
# This script demonstrates taking various types of screenshots
# Usage: .\screenshot-workflow.ps1 [url] [output-dir]

param(
    [string]$URL = "https://example.com",
    [string]$OutputDir = ".\screenshots"
)

$Session = "screenshot-demo"

if (-not (Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Path $OutputDir | Out-Null
}

Write-Host "Opening page..."
vibe-browser open $URL --session $Session

Write-Host "Waiting for page to load..."
vibe-browser wait ms 1000 --session $Session

Write-Host "Taking viewport screenshot..."
vibe-browser screenshot -o "$OutputDir\viewport.png" --session $Session

Write-Host "Taking full page screenshot..."
vibe-browser screenshot --full-page -o "$OutputDir\fullpage.png" --session $Session

Write-Host "Taking snapshot to find specific elements..."
vibe-browser snapshot -i --session $Session

Write-Host "Scrolling down..."
vibe-browser scroll --y 500 --session $Session

Write-Host "Taking screenshot after scroll..."
vibe-browser screenshot -o "$OutputDir\scrolled.png" --session $Session

Write-Host "Closing browser..."
vibe-browser close --session $Session

Write-Host "Done! Screenshots saved to $OutputDir\"
Get-ChildItem $OutputDir
