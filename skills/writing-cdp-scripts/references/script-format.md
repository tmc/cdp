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
cat script.txtar | cdpscript -

# With options
cdp run -v script.txtar                          # Verbose logging
cdp run -o /tmp/output script.txtar              # Output dir for artifacts
cdpscript --headless --timeout 45s script.txtar  # Browser/selector timeout
cdpscript --tab <id> --port 9222 script.txtar    # Connect to existing tab
cdpscript script.txtar --help                    # Script-scoped help
```

## Unix Tool Contract

`cdpscript` and `cdp run` are normal command-line programs:

- arguments after the script path are exposed as `${ARG1}` through `${ARGN}`;
  `${ARGC}` is the count.
- environment variables are available as `${NAME}`.
- `--help` after the script path prints the txtar header and usage for that
  script, not the Go flag help.
- relative artifact paths are written under the output directory when `-o` or
  `--output` is set.
- `--tab <target-id> --port <port>` attaches to an existing DevTools tab.
- path `-` reads the txtar archive from stdin.

Exit codes:

| Code | Meaning |
|---|---|
| 0 | Script ran to completion; all assertions passed |
| 1 | Runtime error |
| 2 | Usage error |
| 3 | Assertion failed |
| 130 | Interrupted |

## Script Structure

A txtar file has an optional comment section followed by `-- filename --`
delimited files. The comment section can hold a shebang:

```text
#!/usr/bin/env cdpscript
# My Test Script
#
# What this script does.
#
# Usage:
#   BASE_URL=https://example.com cdpscript script.txtar

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

- **extra files** - Extracted into the script workdir before execution
- **\*.js** - JavaScript files loaded with `jsfile`
- **external \*.cdp files** - Helper scripts loaded from disk with `source`

## Header Comments

Use the txtar comment section for human-facing information such as purpose,
inputs, and examples. It is not parsed as runtime configuration. Runtime
inputs should come from CLI flags, environment variables, positional
arguments, or runner options.

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
click coord:100,200           # Click viewport coordinates
fill <selector> <text>        # Fill input field
fill @e5 Hello World          # Fill by accessibility ref
type <selector> <text>        # Alias for fill
drag <source> <target> [n]    # Drag between selectors or coord:x,y points
dblclick <selector>           # Real double click (not a synthetic event)
set-range <selector> <value>  # Set an input[type=range] and fire input/change
mouse down <sel|coord:x,y>    # Press and hold
mouse move <sel|coord:x,y>    # Move; carries the button while held
mouse up [<sel|coord:x,y>]    # Release; defaults to the last position
select <selector> <option>    # Select option by value or text
upload <selector> <file>...   # Set file input files
hover <selector>              # Hover over element
press Enter                   # Press key (Enter, Tab, Escape, ArrowDown, etc.)
scroll down 500               # Scroll page by pixels; directions: up/down/left/right
scroll <selector>             # Scroll an element into view
```

`upload` accepts absolute paths, files embedded in the txtar archive, and
relative paths from the process working directory. Use absolute paths for
caller-supplied files passed through `${ARG1}`.

`click coord:x,y` uses viewport coordinates. It is the right fallback for
visible controls inside iframes or shadow DOM when selectors or accessibility
refs are not the best fit.

### Dialogs
```
dialog accept                 # Accept the next JavaScript dialog
dialog dismiss                # Dismiss the next JavaScript dialog
dialog accept response text   # Accept next prompt with text
```

Place `dialog` before the action that opens `alert`, `confirm`, or `prompt`.

### Emulation
```
viewport 390 640              # Set browser viewport size
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

### Gestures and mid-gesture assertions

`drag` is atomic — press, interpolate, release — which cannot express an
assertion taken partway through. The `mouse` primitives can:

```
mouse down '#node'
assert visible '.drag-ghost'
mouse move coord:400,120
assert text '#preview' 'Stage 2'
mouse up
```

A move issued while the button is held carries the button, so the page sees a
drag rather than a hover. `mouse up` with no target releases wherever the
pointer last was.

Quote any argument that contains `#`: an unquoted `#` begins a comment, so
`wait #main` parses as a bare `wait`. Single quotes are removed before the
command sees its arguments.

### Assertions
```
assert exists <selector>                  # Element exists in DOM
assert text <selector> <expected>         # Element text contains expected
assert visible <selector>                 # Element is visible
assert status <url-substr> <code>         # Last matching response has status
assert response <url-substr> <text>       # Last matching response body contains text
assert header <url-substr> <name> <text>  # Last matching response header contains text
```

### Output
```
screenshot output.png         # Full-page screenshot (saved to output dir)
pdf output.pdf                # Save page as PDF
download-dir downloads        # Allow downloads into output dir/downloads
wait-download report.csv 10s  # Wait until a downloaded file exists
log Hello World               # Print message to stdout
```

### Network
```
block *ads*                   # Block URLs matching pattern
cookie get [name]             # Print cookies as JSON, or one cookie value
cookie set <name> <value> [domain] [path]
cookie clear [name]           # Clear all cookies, or one cookie on current URL
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

Use `${VAR_NAME}` to reference environment variables. Values can come from the
runner or from commands that set variables:

```text
-- main.cdp --
goto ${BASE_URL}/login
fill #username ${USERNAME}
```

Commands like `extract`, `title`, and `url` set environment variables
(`${EXTRACTED}`, `${TITLE}`, `${URL}`) that subsequent commands can reference.
Top-level script arguments are available as `${ARG1}`, `${ARG2}`, and
`${ARGC}`.

Example:

```text
-- main.cdp --
goto ${BASE_URL}/users/${ARG1}
log Running user flow ${ARG1} of ${ARGC}
```

## Conditions

Scripts support conditions based on engine state:

```
[has-tab] log Connected to existing tab     # Only if connected to tab
```

cdpscript intentionally keeps loops, retries, and polling policy in the caller:
use the shell, Go tests, or an MCP agent loop to repeat a script. Inside a script,
use rsc.io/script guards (`[cond]`), `!` for expected failure, and `?` for
allowed failure.

## Complete Example

```
# Login Flow Test
#
# Tests the login flow with screenshots at each step.
#
# Usage:
#   BASE_URL=https://staging.example.com cdpscript login.txtar

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
