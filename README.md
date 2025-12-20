# chrome-to-har

[![Go Reference](https://pkg.go.dev/badge/github.com/tmc/misc/chrome-to-har.svg)](https://pkg.go.dev/github.com/tmc/misc/chrome-to-har)

A comprehensive suite of Chrome/Chromium automation tools for network traffic capture, web scraping, and browser automation.

## Tools Overview

### chrome-to-har - Network Traffic Capture

Records browser activity and generates HAR (HTTP Archive) files with advanced features:

- Core HAR capture functionality with complete network traffic recording
- Content stability detection (network idle, DOM complete, resource loading)
- URL/domain blocking with glob patterns, domains, and regex
- Differential capture mode to compare captures and generate reports
- Interactive JavaScript CLI for executing commands in browser context
- Proxy support (HTTP/HTTPS and SOCKS5)
- WebSocket monitoring with real-time frame capture
- Mobile device emulation with comprehensive device profiles
- Visual testing with screenshot capture
- Performance tracking and metrics

### churl - Chrome-powered curl

A curl alternative with JavaScript/SPA support and comprehensive mirroring:

- URL fetching with full JavaScript execution
- Multiple output formats (HTML, HAR, Text, JSON)
- HTTP method support (GET, POST, PUT, DELETE)
- Custom headers and cookie handling with profiles
- Proxy support with authentication
- Wait strategies (network idle, DOM ready, CSS selectors)
- Script injection before/after page load
- Element extraction with CSS selectors (`--extract`)
- WebSocket monitoring and message injection
- URL/domain blocking for ads/trackers
- Website mirroring with wget-compatible options
- Remote Chrome connection support

### cdp - Chrome DevTools Protocol CLI

Low-level interface to Chrome DevTools Protocol:

- JavaScript execution with `--js` flag
- Stdin support for piping JavaScript code
- Element extraction with `--extract` flag
- Profile support with cookies and session data
- Tab management and listing
- Browser auto-discovery
- HAR capture with `--har` flag
- WebSocket monitoring
- Remote Chrome connection
- Unix-friendly exit codes for scripting
- Multiple output formats (text, json, tsv)

## Quick Start

### Basic Examples

```bash
# Record network traffic
chrome-to-har https://example.com

# Fetch page with JavaScript execution
churl https://example.com

# Execute JavaScript in headless mode
cdp --headless --js "document.title" --url "https://example.com"

# Extract content using CSS selectors
cdp --headless --extract "h1" --url "https://example.com"
```

### Advanced Examples

```bash
# Differential capture with blocking
chrome-to-har --diff --baseline baseline.har \
  --block-domains ads.example.com \
  --wait-for-stability https://example.com

# Mirror website with SPA support
churl --mirror --recursive --level=2 \
  --page-requisites --convert-links \
  --block-ads https://example.com

# Extract content with custom headers
churl -H "Authorization: Bearer token" \
  --output-format=json \
  --extract-selector "article p" \
  https://api.example.com

# WebSocket monitoring
churl --ws-enabled --ws-wait-for first_message \
  --ws-stats wss://echo.websocket.org

# Execute JavaScript from stdin
echo "document.querySelector('title').textContent" | \
  cdp --js - --url "https://example.com"
```
## Installation

<details>
<summary><b>Prerequisites: Go Installation</b></summary>

You'll need Go 1.21 or later. [Install Go](https://go.dev/doc/install) if you haven't already.

<details>
<summary><b>Setting up your PATH</b></summary>

After installing Go, ensure that `$HOME/go/bin` is in your PATH:

<details>
<summary><b>For bash users</b></summary>

Add to `~/.bashrc` or `~/.bash_profile`:
```bash
export PATH="$PATH:$HOME/go/bin"
```

Then reload your configuration:
```bash
source ~/.bashrc
```

</details>

<details>
<summary><b>For zsh users</b></summary>

Add to `~/.zshrc`:
```bash
export PATH="$PATH:$HOME/go/bin"
```

Then reload your configuration:
```bash
source ~/.zshrc
```

</details>

</details>

</details>

### Install

```bash
go install github.com/tmc/misc/chrome-to-har@latest
```

### Run directly

```bash
go run github.com/tmc/misc/chrome-to-har@latest [arguments]
```

## CDP - Chrome DevTools Protocol CLI

The `cdp` command provides a low-level interface to Chrome DevTools Protocol for browser automation and testing.

### Quick Start with Profiles and HAR Recording

For authenticated workflows, use CDP with Chrome profiles and HAR recording:

```bash
# List available profiles
cdp --list-profiles

# Use a profile and record network traffic
cdp --use-profile "Default" \
    --har /tmp/session.har \
    --url https://example.com

# Extract JavaScript data
cdp --use-profile "Default" \
    --url https://example.com/api \
    --js 'Array.from(document.querySelectorAll("a")).map(a => a.href)'
```

**See full workflow guide**: [`docs/CDP_PROFILE_HAR_WORKFLOW.md`](docs/CDP_PROFILE_HAR_WORKFLOW.md) - Complete examples for capturing authenticated network traffic, monitoring URL patterns, and extracting responses.

### Exit Codes

`cdp` follows Unix conventions for exit codes, making it suitable for use in shell scripts and CI/CD pipelines:

- **0** - Success
- **1** - General error (JavaScript errors, profile issues, etc.)
- **2** - Command line usage error
- **3** - Browser launch or connection failed
- **4** - Page navigation failed
- **5** - Operation timed out

### Examples

```bash
# Execute JavaScript in headless Chrome
cdp --headless --js "document.title" --url "https://example.com"

# Check exit code for scripting
if cdp --headless --js "document.title" --url "https://example.com"; then
    echo "Success"
else
    echo "Failed with exit code $?"
fi

# Connect to existing Chrome instance
cdp --remote-host localhost --remote-port 9222 --js "document.title"
```

For more information, run `cdp --help` or `cdp -help`.

## Churl - Chrome-powered curl

The `churl` command provides a curl-like interface with full JavaScript/SPA support and website mirroring capabilities.

### Installation

```bash
# Install churl (included with chrome-to-har)
go install github.com/tmc/misc/chrome-to-har/cmd/churl@latest

# Or build from source
cd cmd/churl && go build
```

### Basic Usage

```bash
# Fetch a page (default: HTML output)
churl https://example.com

# Different output formats
churl --output-format=text https://example.com
churl --output-format=json https://example.com
churl --output-format=har https://example.com > output.har

# Save to file
churl -o output.html https://example.com

# Save separate HAR file (works with any output format)
churl --har traffic.har https://example.com
```

### Website Mirroring

```bash
# Mirror mode (recursive download with link conversion)
churl --mirror https://example.com

# Recursive download with depth limit
churl --recursive --level=2 https://example.com

# Download page requisites (images, CSS, JS)
churl --page-requisites https://example.com

# Full mirror with all assets and converted links
churl -m -p -k https://example.com

# wget-compatible options
churl -r -l 3 -k -p -P ./downloads https://example.com
```

### Advanced Features

```bash
# Custom headers and authentication
churl -H "Authorization: Bearer token" \
  -H "Accept: application/json" \
  https://api.example.com

# POST/PUT/DELETE with data
churl -X POST -d '{"key":"value"}' \
  -H "Content-Type: application/json" \
  https://api.example.com/endpoint

# Extract content with CSS selectors
churl --extract-selector "article p" \
  --output-format=json \
  https://news.example.com

# Block ads and trackers
churl --block-ads --block-tracking https://example.com

# Custom blocking rules
churl --block-domain ads.example.com \
  --block-pattern "*/analytics/*" \
  https://example.com

# WebSocket support
churl --ws-enabled \
  --ws-wait-for first_message \
  --ws-stats \
  wss://echo.websocket.org

# Send WebSocket messages
churl --ws-enabled \
  --ws-send "Hello" \
  --ws-send "World" \
  wss://echo.websocket.org

# Script injection
churl --script-before "window.test = true" \
  --script-after "console.log(window.test)" \
  https://example.com

# Wait strategies
churl --wait-for "#content" https://example.com
churl --wait-network-idle https://example.com

# Use Chrome profile with cookies
churl --profile "Default" https://authenticated-site.com

# Connect to remote Chrome
churl --remote-host localhost \
  --remote-port 9222 \
  https://example.com

# Proxy support
churl --proxy http://proxy.example.com:8080 \
  https://example.com

churl --socks5-proxy socks5://proxy.example.com:1080 \
  https://example.com
```

### Mirroring Options Reference

```bash
# Directory control
-P, --directory-prefix DIR    Save files to directory
-x, --force-directories       Create directory hierarchy
-nH, --no-host-directories    Don't create host-based directories
-nd, --no-directories         Flat directory structure
--cut-dirs N                  Ignore N remote directory components

# Download control
-r, --recursive               Recursive download
-l N, --level N              Maximum recursion depth (0=infinite)
-m, --mirror                 Mirror mode (-r -k shortcut)
-p, --page-requisites        Download all assets
-k, --convert-links          Convert links to relative paths
-np, --no-parent             Don't ascend to parent directory

# File selection
-A, --accept LIST            Accept file extensions
-R, --reject LIST            Reject file extensions
--accept-regex REGEX         Accept URLs matching regex
--reject-regex REGEX         Reject URLs matching regex

# Domain control
-D, --domains LIST           Accept only these domains
--exclude-domains LIST       Reject these domains
-H, --span-hosts            Follow links to other domains

# Download behavior
-c, --continue              Resume partial downloads
-nc, --no-clobber           Don't re-download existing files
-N, --timestamping          Only download newer files
-w N, --wait N              Wait N seconds between downloads
--limit-rate N              Limit download speed (bytes/sec)
-Q N, --quota N             Maximum total download size
```

For more information, run `churl --help`.

