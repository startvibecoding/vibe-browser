# Snapshot and Element References

Understanding accessibility tree snapshots and element selection.

## Snapshot Types

### Full Snapshot

```bash
vibe-browser snapshot
```

Shows the complete accessibility tree including all elements.

### Interactive Snapshot (Recommended)

```bash
vibe-browser snapshot --interactive
```

Shows only interactive elements (buttons, inputs, links, etc.). This is the preferred mode for AI agents as it reduces noise.

### Compact Snapshot

```bash
vibe-browser snapshot --interactive --compact
```

Removes empty structural nodes for cleaner output.

## Snapshot Output Format

```
Page: Example - Login
URL: https://example.com/login

[1] heading "Log in"
[2] form
  [3] textbox Email
  [4] textbox Password
  [5] button "Continue"
  [6] link "Forgot password?"
```

Each element has:
- Number in brackets `[N]` for reference
- Role (heading, button, textbox, link, etc.)
- Name or label text
- Additional attributes when relevant

## Selecting Elements

### By CSS Selector (Recommended)

```bash
vibe-browser click "button.submit"
vibe-browser fill "input[name=email]" "user@example.com"
vibe-browser get text "h1.title"
```

### By Text Content

```bash
vibe-browser click "button:has-text('Submit')"
vibe-browser get text "p:has-text('Welcome')"
```

### By Role and Name

```bash
vibe-browser click "button[aria-label='Close']"
vibe-browser fill "input[placeholder='Search']" "query"
```

## Best Practices

1. **Always snapshot first** to see available elements
2. **Use CSS selectors** over text matching for reliability
3. **Re-snapshot after page changes** as refs become stale
4. **Wait for elements** before interacting:
   ```bash
   vibe-browser wait selector "button.submit"
   vibe-browser click "button.submit"
   ```

## Common Selector Patterns

```bash
# By ID
vibe-browser click "#submit-btn"

# By class
vibe-browser fill ".email-input" "user@test.com"

# By attribute
vibe-browser click "[data-testid='login-button']"
vibe-browser fill "[type='email']" "user@test.com"

# By form structure
vibe-browser fill "form input[name='email']" "user@test.com"
vibe-browser click "form button[type='submit']"

# Descendant selectors
vibe-browser get text ".card .title"
vibe-browser click ".modal .close-button"
```

## Supported Browsers

The snapshot feature works with all supported browsers on macOS, Linux, and Windows:

| Browser | macOS | Linux | Windows |
|---------|-------|-------|--------|
| Chrome | ✅ | ✅ | ✅ |
| Chromium | ✅ | ✅ | ✅ |
| Brave | ✅ | ✅ | ✅ |
| Edge | ✅ | ✅ | ✅ |
| Chrome Canary | ✅ | — | — |

### Platform-Specific Browser Paths

#### macOS
- Chrome: `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`
- Chromium: `/Applications/Chromium.app/Contents/MacOS/Chromium`
- Brave: `/Applications/Brave Browser.app/Contents/MacOS/Brave Browser`
- Edge: `/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge`
- Chrome Canary: `/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary`

#### Linux
- Chrome: `google-chrome`, `google-chrome-stable`, `/usr/bin/google-chrome`
- Chromium: `chromium`, `chromium-browser`, `/usr/bin/chromium`
- Brave: `brave-browser`, `/usr/bin/brave-browser`
- Edge: `microsoft-edge`, `/usr/bin/microsoft-edge`

#### Windows
- Chrome: `%PROGRAMFILES%\Google\Chrome\Application\chrome.exe`
- Chromium: `%LOCALAPPDATA%\Chromium\Application\chrome.exe`
- Brave: `%LOCALAPPDATA%\BraveSoftware\Brave-Browser\Application\brave.exe`
- Edge: `%PROGRAMFILES%\Microsoft\Edge\Application\msedge.exe`

Specify a custom browser path with `--executable-path` or the `CHROME_PATH` environment variable if auto-detection fails.
