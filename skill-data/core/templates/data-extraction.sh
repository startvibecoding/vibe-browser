#!/bin/bash

# Example: Data extraction workflow
# This script demonstrates extracting data from a web page

set -e

URL="${1:-https://example.com}"
SESSION="extract-demo"

echo "Opening page..."
vibe-browser open "$URL" --session "$SESSION"

echo "Getting page title..."
TITLE=$(vibe-browser get title --session "$SESSION")
echo "Title: $TITLE"

echo "Getting page URL..."
URL_CURRENT=$(vibe-browser get url --session "$SESSION")
echo "URL: $URL_CURRENT"

echo "Taking snapshot to see page structure..."
vibe-browser snapshot -i --session "$SESSION"

echo "Extracting text from specific elements..."
vibe-browser get text "h1" --session "$SESSION"
vibe-browser get text "p" --session "$SESSION"

echo "Extracting links..."
vibe-browser get attr "a" href --session "$SESSION"

echo "Evaluating JavaScript for complex extraction..."
vibe-browser eval "document.querySelectorAll('a').length" --session "$SESSION"

echo "Taking screenshot..."
vibe-browser screenshot -o extract-result.png --session "$SESSION"

echo "Closing browser..."
vibe-browser close --session "$SESSION"

echo "Done! Data extracted and screenshot saved."
