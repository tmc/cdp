# CDP CLI Basics

The `cdp` tool (`cmd/cdp`) provides Chrome DevTools Protocol interaction via an interactive REPL or scripted automation using txtar-based scripts.

## Building

```bash
go build -o cdp ./cmd/cdp
```

## Modes of Operation

### 1. Interactive REPL

Launch the interactive shell to type CDP commands directly:

```bash
# Launch with headless Chrome (default)
cdp

# Connect to an already-running Chrome with remote debugging
cdp --remote-host localhost --remote-port 9222 --shell

# Verbose output for debugging
cdp -v
```

At the `cdp>` prompt, type commands like `goto`, `click`, `screenshot`, etc.

### 2. Script Mode (txtar)

Execute automation scripts bundled as txtar archives:

```bash
cdp run script.txtar
cdp run -v --output /tmp/artifacts script.txtar
cdp run --tab <tab-id> --port 9222 script.txtar
```

## Browser Connection

The cdp tool can:
- **Launch a new browser**: Default behavior, launches headless Chrome/Brave/Chromium.
- **Connect to existing Chrome**: Use `--remote-host` and `--remote-port` to connect to Chrome started with `--remote-debugging-port`.
- **Connect to a specific tab**: Use `--tab <id>` with the tab ID from `http://localhost:9222/json/list`.

Browser discovery order: Brave > Chrome Canary > Chrome > Chrome Beta > Chromium > Edge.

## Existing Browser Attach

Use `cdp attach` before guessing at tab IDs. It probes DevTools endpoints and prints exact commands for attachable page targets:

```bash
cdp attach
cdp attach -port 9222
cdp attach -host localhost -port 9222 -format json
```

The text output includes commands shaped like:

```bash
cdp --remote-host localhost --remote-port 9222 --tab <target-id> --shell
```

For one-shot inspection against a known tab, use the same host, port, and tab ID:

```bash
cdp --remote-host localhost --remote-port 9222 --tab <target-id> --await --format json --js 'document.title'
```

For txtar automation against an existing tab, use the script command's `--tab` and `--port` flags:

```bash
cdp run --tab <target-id> --port 9222 script.txtar
cdpscript --tab <target-id> --port 9222 script.txtar
```

`--remote-tab <id-or-url>` selects a tab by ID or URL and takes effect only with `--remote-host`; `--tab` wins when both are set. For agent workflows, prefer `cdp attach` output or the explicit `--remote-host --remote-port --tab` form so the target is visible and reproducible.

## Profiles And Auth State

There are two supported ways to work with logged-in browser state:

```bash
cdp --list-profiles
cdp --use-profile "Default" --url https://example.com --shell
cdp --use-profile "Default" --cookie-domains example.com --har /tmp/session.har --url https://example.com
```

`--use-profile` copies the named browser profile into a temporary working directory and launches Chrome/Brave against that copy. It is useful for reusing cookies without mutating the original profile, and `--cookie-domains` narrows copied cookies when that is enough.

`--use-profile` does not attach to an already-running browser using that profile. When the task depends on the exact live logged-in browser state, use `cdp attach` and the `--remote-host --remote-port --tab` command it prints.

## Core Commands (Interactive & Script)

### Navigation
| Command | Aliases | Description |
|---------|---------|-------------|
| `goto <url>` | `go`, `nav` | Navigate to URL |
| `reload` | `refresh`, `r` | Reload page |
| `back` | `b` | Go back |
| `forward` | `f`, `fwd` | Go forward |
| `stop` | `s` | Stop loading |

### DOM Interaction
| Command | Description |
|---------|-------------|
| `click <selector>` | Click an element |
| `fill <selector> <text>` | Fill input field |
| `type <selector> <text>` | Type into element |
| `hover <selector>` | Hover over element |
| `press <key>` | Press keyboard key (Enter, Tab, etc.) |
| `clear <selector>` | Clear input field |
| `focus <selector>` | Focus element |
| `submit <selector>` | Submit a form |

### Page Info
| Command | Description |
|---------|-------------|
| `title` | Get page title |
| `url` | Get current URL |
| `html [selector]` | Get HTML content |
| `text <selector>` | Get text content |
| `attr <selector> <attr>` | Get attribute value |
| `source` | Get full page source |
| `render [selector]` | Render page/element as markdown |

### JavaScript Execution
| Command | Description |
|---------|-------------|
| `js <code>` | Execute JavaScript |
| `eval <code>` | Evaluate and print result |
| `jsfile <path>` | Execute JS from file |

### Output
| Command | Description |
|---------|-------------|
| `screenshot [file]` | Capture screenshot |
| `pdf [file]` | Save as PDF |
| `log <message>` | Print message |

### Emulation
| Command | Description |
|---------|-------------|
| `mobile` | Emulate mobile (375x812) |
| `desktop` | Reset to desktop (1920x1080) |
| `tablet` | Emulate tablet (768x1024) |
| `viewport <w> <h>` | Set viewport size |
| `darkmode` | Enable dark mode |
| `lightmode` | Enable light mode |
| `offline` | Simulate offline |
| `online` | Reset to online |

### Network
| Command | Description |
|---------|-------------|
| `cookies` | Get all cookies |
| `setcookie <name> <val>` | Set a cookie |
| `deletecookie <name>` | Delete cookie |
| `clearcookies` | Clear all cookies |
| `block <pattern>` | Block URL pattern |

### Storage
| Command | Description |
|---------|-------------|
| `localStorage` | Get all localStorage |
| `setLocal <key> <val>` | Set localStorage item |
| `getLocal <key>` | Get localStorage item |
| `clearLocal` | Clear localStorage |
| `sessionStorage` | Get all sessionStorage |

### Waiting
| Command | Description |
|---------|-------------|
| `wait <selector>` | Wait for element |
| `wait <duration>` | Wait for time (e.g., `2s`, `500ms`) |

## Help System

In interactive mode:
- `help` - Show all commands
- `help <command>` - Detailed help for a command
- `list` - List all commands
- `search <term>` - Search for commands

## Examples

```
# Navigate and extract title
goto https://example.com
wait h1
title

# Fill a form
goto https://example.com/login
fill #username user@test.com
fill #password secret123
click button[type="submit"]
wait #dashboard

# Take mobile screenshot
mobile
goto https://example.com
screenshot mobile-view.png
desktop
```
