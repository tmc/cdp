# HAR & HARL Logging

Network traffic capture using HAR (HTTP Archive) format and HARL (HAR Lines / NDJSON streaming) with both the `cdp` script engine and `chrome-to-har`.

## Overview

Two tools handle HAR capture:

| Tool | Use Case |
|------|----------|
| `chrome-to-har` | Navigate to a URL and capture network traffic to a HAR file |
| `cdp run` | Script engine: tag-based HAR recording within automation scripts |

## chrome-to-har: Full HAR Capture

### Building

```bash
go build ./cmd/chrome-to-har
```

### Basic Usage

```bash
# Capture all traffic for a page load
chrome-to-har -url https://example.com -output page.har

# With verbose logging
chrome-to-har -url https://example.com -output page.har -verbose

# Stream entries as NDJSON (HARL)
chrome-to-har -url https://example.com -stream
```

### Key Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-url` | | URL to navigate to |
| `-output` | `output.har` | Output HAR file path |
| `-verbose` | false | Verbose logging |
| `-stream` | false | Stream entries as NDJSON (one JSON object per line) |
| `-profile` | | Chrome profile path to use |
| `-cookie-domains` | | Comma-separated domains to copy cookies from |
| `-filter` | | jq expression to filter HAR entries |
| `-timeout` | 180 | Global timeout in seconds |

### HAR File Format

Standard HAR 1.2 format:

```json
{
  "log": {
    "version": "1.2",
    "creator": { "name": "chrome-to-har", "version": "1.0" },
    "pages": [],
    "entries": [
      {
        "startedDateTime": "2024-01-15T10:30:00.000Z",
        "request": {
          "method": "GET",
          "url": "https://example.com/",
          "headers": [...]
        },
        "response": {
          "status": 200,
          "headers": [...],
          "content": { "size": 1234, "mimeType": "text/html" }
        },
        "time": 150.5
      }
    ]
  }
}
```

### HARL Streaming (NDJSON)

With `-stream`, each HAR entry is written as a single JSON line as it completes:

```bash
chrome-to-har -url https://example.com -stream > traffic.jsonl
```

Each line is a complete HAR entry JSON object. Useful for:
- Real-time monitoring of network requests
- Piping to `jq` for filtering
- Processing large captures without loading everything into memory

```bash
# Stream and filter for API calls
chrome-to-har -url https://example.com -stream | jq 'select(.request.url | contains("/api/"))'

# Count requests by content type
chrome-to-har -url https://example.com -stream | jq -r '.response.content.mimeType' | sort | uniq -c
```

This repository's `.gitignore` excludes `*.har.jsonl`.

### Domain-Separated Output

`cdp` can write separate JSONL files per page domain and request host.
`--harl` turns on streaming; `--output-dir` routes the stream into per-host
files instead of `--harl-file`:

```bash
# Output goes to <dir>/<page-domain>/<hostname>.jsonl
cdp --url https://example.com --harl --output-dir ./har-data
```

## CDP Script Engine: Tagged HAR Recording

In cdp scripts, HAR recording is tag-based. You tag network activity sections, add annotations, then write the HAR file.

### Commands

| Command | Description |
|---------|-------------|
| `tag <name>` | Start tagging network requests with a label |
| `tag` | Clear current tag |
| `note <text>` | Add timestamped note to HAR |
| `capture screenshot <desc>` | Embed screenshot in HAR annotations |
| `capture dom <desc>` | Embed DOM snapshot in HAR annotations |
| `har <filename>` | Write HAR file with all recorded data |

### How It Works

1. The first `tag` command initializes the HAR recorder and enables network event capture.
2. Subsequent network requests are tagged with the current tag name.
3. Notes and captures are stored as annotations with timestamps.
4. The `har` command writes everything to a file.

### Basic Example

```
# HAR Recording Example

-- main.cdp --
# Start recording - tags network requests as "homepage"
tag homepage
goto https://example.com
wait body
note Homepage loaded successfully

# Tag login flow separately
tag login
goto https://example.com/login
wait '#login-form'
capture screenshot Login page
fill '#email' test@example.com
fill '#password' secret
click button[type="submit"]
wait '#dashboard'
capture screenshot Dashboard loaded
note Login flow complete

# Clear tag
tag

# Write HAR file with all tagged data
har session.har
log HAR saved
```

### Tag Ranges

Tags create named ranges in the HAR timeline. This lets you filter and analyze specific parts of a session:

```
tag api-calls
# ... network activity here is tagged "api-calls" ...
tag page-load
# ... network activity here is tagged "page-load" ...
tag
# ... network activity here is untagged ...
har output.har
```

### Annotations

Annotations add non-network records to the HAR file:

```
# Text annotation with timestamp
note User clicked login button

# Base64 PNG embedded in HAR
capture screenshot After clicking login

# HTML snapshot in HAR
capture dom Form state before submission
```

These are stored in HAR custom fields.

### Complete Test Example with HAR

```
# E2E Checkout Flow with HAR
#
# Records full network activity for the checkout flow.
#
# Usage:
#   BASE_URL=https://staging.shop.example.com cdpscript checkout-har.txtar

-- main.cdp --
# Phase 1: Browse products
tag browse
goto ${BASE_URL}/products
wait .product-grid
capture screenshot Product listing
note Viewing product catalog

# Phase 2: Add to cart
tag cart
click .product-card:first-child .add-to-cart
wait .cart-count
capture screenshot Item added to cart
note Added first product to cart

# Phase 3: Checkout
tag checkout
goto ${BASE_URL}/checkout
wait '#checkout-form'
capture screenshot Checkout form

fill '#name' Test User
fill '#email' test@example.com
fill '#card' 4242424242424242
capture screenshot Form filled
note Checkout form completed

click '#place-order'
wait .order-confirmation
capture screenshot Order confirmed
capture dom Order confirmation page
note Order placed successfully

tag
har checkout-flow.har
log E2E checkout test complete with HAR recording
```

## Analyzing HAR Files

### With jq

```bash
# List all URLs
jq '.log.entries[].request.url' output.har

# Find slow requests (>1 second)
jq '.log.entries[] | select(.time > 1000) | {url: .request.url, time: .time}' output.har

# Response status codes
jq '.log.entries[] | {url: .request.url, status: .response.status}' output.har

# Total transfer size
jq '[.log.entries[].response.content.size] | add' output.har
```

### With chrome-to-har Filtering

```bash
# Omit tracking/analytics URLs
chrome-to-har -url https://example.com \
  -filter 'select(.request.url | test("analytics|tracking|ads") | not)' -output clean.har

# Strip cookies from the recorded entries
chrome-to-har -url https://example.com \
  -filter 'del(.request.cookies, .response.cookies)' -output clean.har
```

## Recorder Internals

The HAR recorder (`internal/recorder/recorder.go`) builds entries from
`Network.requestWillBeSent`, `Network.responseReceived`, and
`Network.loadingFinished`, and fetches response bodies asynchronously. It also
records tag ranges and annotations, streams NDJSON, writes per-domain files,
and applies jq filters and Go templates to entries.
