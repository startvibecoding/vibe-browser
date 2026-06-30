#!/bin/bash

# Example: Form automation workflow (bash / zsh)
# This script demonstrates filling out and submitting a form
# macOS/Linux: Run as ./form-automation.sh [url]
# Windows: Use form-automation.ps1 instead

set -e

URL="${1:-https://example.com/login}"
SESSION="form-demo"

echo "Opening form page..."
vibe-browser open "$URL" --session "$SESSION"

echo "Taking snapshot to see form elements..."
vibe-browser snapshot -i --session "$SESSION"

echo "Filling form fields..."
vibe-browser fill "input[name=email]" --value "user@example.com" --session "$SESSION"
vibe-browser fill "input[name=password]" --value "securepassword" --session "$SESSION"

echo "Submitting form..."
vibe-browser click "button[type=submit]" --session "$SESSION"

echo "Waiting for navigation..."
vibe-browser wait url "/dashboard" --session "$SESSION"

echo "Taking screenshot of result..."
vibe-browser screenshot -o form-result.png --session "$SESSION"

echo "Closing browser..."
vibe-browser close --session "$SESSION"

echo "Done! Screenshot saved to form-result.png"
