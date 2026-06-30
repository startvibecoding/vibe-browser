# Example: Data extraction workflow (PowerShell)
# This script demonstrates extracting data from a web page
# Usage: .\data-extraction.ps1 [url]

param(
    [string]$URL = "https://example.com"
)

$Session = "extract-demo"

Write-Host "Opening page..."
vibe-browser open $URL --session $Session

Write-Host "Getting page title..."
$Title = vibe-browser get title --session $Session
Write-Host "Title: $Title"

Write-Host "Getting page URL..."
$URLCurrent = vibe-browser get url --session $Session
Write-Host "URL: $URLCurrent"

Write-Host "Taking snapshot to see page structure..."
vibe-browser snapshot -i --session $Session

Write-Host "Extracting text from specific elements..."
vibe-browser get text "h1" --session $Session
vibe-browser get text "p" --session $Session

Write-Host "Extracting links..."
vibe-browser get attr "a" href --session $Session

Write-Host "Evaluating JavaScript for complex extraction..."
vibe-browser eval "document.querySelectorAll('a').length" --session $Session

Write-Host "Taking screenshot..."
vibe-browser screenshot -o extract-result.png --session $Session

Write-Host "Closing browser..."
vibe-browser close --session $Session

Write-Host "Done! Data extracted and screenshot saved."
