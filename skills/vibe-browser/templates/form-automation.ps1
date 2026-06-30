# Example: Form automation workflow (PowerShell)
# This script demonstrates filling out and submitting a form
# Usage: .\form-automation.ps1 [url]

param(
    [string]$URL = "https://example.com/login"
)

$Session = "form-demo"

Write-Host "Opening form page..."
vibe-browser open $URL --session $Session

Write-Host "Taking snapshot to see form elements..."
vibe-browser snapshot -i --session $Session

Write-Host "Filling form fields..."
vibe-browser fill "input[name=email]" --value "user@example.com" --session $Session
vibe-browser fill "input[name=password]" --value "securepassword" --session $Session

Write-Host "Submitting form..."
vibe-browser click "button[type=submit]" --session $Session

Write-Host "Waiting for navigation..."
vibe-browser wait url "/dashboard" --session $Session

Write-Host "Taking screenshot of result..."
vibe-browser screenshot -o form-result.png --session $Session

Write-Host "Closing browser..."
vibe-browser close --session $Session

Write-Host "Done! Screenshot saved to form-result.png"
