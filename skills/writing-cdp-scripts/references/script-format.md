# CDP Script Format (txtar)

CDP scripts use Go's txtar (text archive) format to bundle automation logic and supporting files into a single file. The script engine is built on `rsc.io/script`.

## Running Scripts

```bash
# Build either entry point
go build -o cdp ./cmd/cdp
go build -o cdpscript ./cmd/cdpscript

# Run a script
cdp run script.txtar
cdpscript script.txtar
cdpscript script.txtar one two

# With options
cdp run -v script.txtar                          # Verbose logging
cdp run -o /tmp/output script.txtar              # Output dir for artifacts
cdpscript --tab <id> --port 9222 script.txtar    # Connect to existing tab
```

## Script Structure

A txtar file has an optional comment section followed by `-- filename --`
delimited files. The comment section can hold a shebang:

```text
#!/usr/bin/env cdpscript
-- meta.yaml --
name: My Test Script
description: What this script does
browser: chrome
headless: true
timeout: 30s
env:
  BASE_URL: "https://example.com"

-- main.cdp --
# Main automation script goes here
goto ${BASE_URL}
wait h1
screenshot result.png

-- helper.js --
// JavaScript files accessible via jsfile command
document.querySelector('#foo').click();
```

### Required Files

- **main.cdp** - The main script to execute (required)

### Optional Files

- **meta.yaml** - Script metadata and configuration
- **extra files** - Extracted into the script workdir before execution
- **\*.js** - JavaScript files loaded with `jsfile`
- **external \*.cdp files** - Helper scripts loaded from disk with `source`

## Metadata (meta.yaml)

```yaml
name: Script Name
description: What it does
version: "1.0"
browser: chrome          # chrome, brave, chromium, edge
profile: "Profile 1"    # Chrome profile name to copy
headless: true           # Run headless (default: false in meta)
timeout: 30s             # Overall timeout
env:                     # Environment variables
  BASE_URL: "https://example.com"
  USERNAME: "test@test.com"
```

Only these fields are read by the engine today.

## Script Commands Reference

### Navigation
```
goto <url>                    # Navigate to URL
back                          # Go back in history
forward                       # Go forward
reload                        # Reload page
```

### Waiting
```
wait <selector>               # Wait for element to appear
wait 2s                       # Wait for duration
wait 500ms                    # Millisecond precision
wait h1                       # Wait for an h1 element
```

### DOM Interaction
```
click <selector>              # Click element
click @e3                     # Click by accessibility ref
fill <selector> <text>        # Fill input field
fill @e5 Hello World          # Fill by accessibility ref
type <selector> <text>        # Alias for fill
hover <selector>              # Hover over element
press Enter                   # Press key (Enter, Tab, Escape, ArrowDown, etc.)
```

### JavaScript
```
js document.title             # Execute single-line JS
js window.scrollTo(0, 500)    # Execute any JS
jsfile helper.js              # Execute JS from embedded file
```

### Extraction
```
extract <selector>            # Extract text, print it, set ${EXTRACTED}
title                         # Print page title, set ${TITLE}
url                           # Print current URL, set ${URL}
render [selector]             # Render page/element as markdown, set ${RENDERED}
render --term [selector]      # Render as terminal-formatted text
```

### Assertions
```
assert exists <selector>                  # Element exists in DOM
assert text <selector> <expected>         # Element text contains expected
assert visible <selector>                 # Element is visible
```

### Output
```
screenshot output.png         # Full-page screenshot (saved to output dir)
pdf output.pdf                # Save page as PDF
log Hello World               # Print message to stdout
```

### Network
```
block *ads*                   # Block URLs matching pattern
```

### Accessibility Snapshots
```
snapshot                      # Full accessibility tree with refs
snapshot -i                   # Interactive elements only
snapshot --compact            # Remove structural noise
snapshot --depth 3            # Limit depth
snapshot --selector #main     # Scope to CSS selector
```

Snapshot output includes refs like `@e1`, `@e2` that you can use with click/fill:
```
snapshot -i
# Output:
#   - button "Submit" [ref=e1]
#   - textbox "Email" [ref=e2]
click @e1
fill @e2 test@example.com
```

### Source Command (Include Scripts)
```
source examples/lib/screenshot.cdp   # Execute inline
source -x examples/lib/screenshot.cdp
source -as send-msg examples/lib/screenshot.cdp
send-msg "Hello"                     # Call registered command
```

Sourced scripts receive arguments as `${ARG1}`, `${ARG2}`, etc., and `${ARGC}`
for count.

### HAR Recording & Tagging (Advanced)
```
tag login-flow                # Start tagging network requests
note Starting login           # Add note to HAR
capture screenshot Login page # Capture screenshot into HAR
capture dom Before submit     # Capture DOM snapshot into HAR
tag                           # Clear tag
har output.har                # Write HAR file
```

## Variables

Use `${VAR_NAME}` to reference environment variables set in meta.yaml:

```yaml
-- meta.yaml --
env:
  BASE_URL: "https://example.com"
  USERNAME: "admin"

-- main.cdp --
goto ${BASE_URL}/login
fill #username ${USERNAME}
```

Commands like `extract`, `title`, and `url` set environment variables
(`${EXTRACTED}`, `${TITLE}`, `${URL}`) that subsequent commands can reference.
Top-level script arguments are available as `${ARG1}`, `${ARG2}`, and
`${ARGC}`.

## Conditions

Scripts support conditions based on engine state:

```
[headless] screenshot headless-only.png     # Only run if headless
[has-tab] log Connected to existing tab     # Only if connected to tab
```

## Complete Example

```
-- meta.yaml --
name: Login Flow Test
description: Tests the login flow with screenshots at each step
browser: brave
headless: true
timeout: 60s
env:
  BASE_URL: "https://staging.example.com"

-- main.cdp --
# Navigate to login page
goto ${BASE_URL}/login
wait #login-form
screenshot 01-login-page.png

# Fill credentials
fill #email test@example.com
fill #password testpass123
screenshot 02-filled-form.png

# Submit
click button[type="submit"]
wait #dashboard
screenshot 03-dashboard.png

# Verify
assert text h1 Welcome
title
log Login test passed

-- verify.js --
// Additional verification script
(function() {
  var token = localStorage.getItem('auth_token');
  return token ? 'authenticated' : 'not authenticated';
})();
```

## Shared Libraries

Place reusable `.cdp` and `.js` files in `examples/lib/`:

```
examples/lib/
  screenshot.cdp      # screenshot $ARG1 + log
  wait-for-load.cdp   # Standard page load wait
  enable-fc.js        # JS helper for enabling features
```

Use them with `source`:
```
source examples/lib/screenshot.cdp
source -as snap examples/lib/screenshot.cdp
snap login-page.png
```
