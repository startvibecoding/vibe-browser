#!/bin/bash

# Example: Screenshot workflow
# This script demonstrates taking various types of screenshots

set -e

URL="${1:-https://example.com}"
SESSION="screenshot-demo"
OUTPUT_DIR="${2:-./screenshots}"

mkdir -p "$OUTPUT_DIR"

echo "Opening page..."
vibe-browser open "$URL" --session "$SESSION"

echo "Waiting for page to load..."
vibe-browser wait ms 1000 --session "$SESSION"

echo "Taking viewport screenshot..."
vibe-browser screenshot -o "$OUTPUT_DIR/viewport.png" --session "$SESSION"

echo "Taking full page screenshot..."
vibe-browser screenshot --full-page -o "$OUTPUT_DIR/fullpage.png" --session "$SESSION"

echo "Taking snapshot to find specific elements..."
vibe-browser snapshot -i --session "$SESSION"

echo "Scrolling down..."
vibe-browser scroll --y 500 --session "$SESSION"

echo "Taking screenshot after scroll..."
vibe-browser screenshot -o "$OUTPUT_DIR/scrolled.png" --session "$SESSION"

echo "Closing browser..."
vibe-browser close --session "$SESSION"

echo "Done! Screenshots saved to $OUTPUT_DIR/"
ls -la "$OUTPUT_DIR/"
