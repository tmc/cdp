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
cdp --port 9222

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
- **Connect to existing Chrome**: Use `--port` to connect to Chrome started with `--remote-debugging-port`.
- **Connect to a specific tab**: Use `--tab <id>` with the tab ID from `http://localhost:9222/json/list`.

Browser discovery order: Brave > Chrome Canary > Chrome > Chrome Beta > Chromium > Edge.

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
