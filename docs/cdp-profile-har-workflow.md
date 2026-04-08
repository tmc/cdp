# CDP Profile + HAR Workflow: Capturing Authenticated Network Traffic

This guide demonstrates how to use the CDP (Chrome DevTools Protocol) tool with Chrome profiles and HAR recording to capture and analyze authenticated network traffic, including extracting responses from authenticated downloads.

## Overview

The CDP tool provides a powerful combination of features for working with authenticated sessions:

- **Profile Support**: Use existing Chrome profiles with cookies, session data, and credentials
- **HAR Recording**: Capture complete network traffic in HTTP Archive format
- **URL Monitoring**: Watch for specific URL patterns and extract response data
- **JavaScript Extraction**: Run custom scripts to extract data directly from the page

## Quick Start

### 1. List Available Chrome Profiles

First, discover what Chrome profiles you have available:

```bash
cdp --list-profiles
```

Output:
```
Available Chrome profiles:
==========================
[1] Default
[2] Profile 1
[3] Work Profile

Use with: cdp -use-profile "Default" -js "document.title"
```

### 2. Launch CDP with a Profile and HAR Recording

Use a specific profile and capture network traffic:

```bash
cdp --use-profile "Default" \
    --har /tmp/session.har \
    --url https://example.com \
    --interactive
```

This launches an interactive session where you can browse normally. All network traffic is captured to the HAR file.

## Use Case 1: Capturing Authenticated API Requests

### Scenario: Downloading NotebookLM Audio Files

NotebookLM requires authentication. You can use CDP to capture the audio download URL and authentication headers.

#### Step 1: Record Network Traffic

```bash
# Launch CDP with your Google account profile
cdp --use-profile "Default" \
    --har /tmp/notebooklm-traffic.har \
    --url https://notebooklm.google.com
```

#### Step 2: Perform Your Action

In the browser:
1. Navigate to your NotebookLM notebook
2. Play the audio to generate the download request
3. Exit CDP (Ctrl+C)

#### Step 3: Analyze the HAR File

Extract audio URLs from the HAR:

```bash
# Find all requests to audio CDN
cat /tmp/notebooklm-traffic.har | jq -r '.log.entries[] | select(.request.url | contains("googleusercontent.com")) | .request.url'

# Output:
# https://lh3.googleusercontent.com/d/1ABC.../audio.mp3?auth_token=xyz...
```

Extract authentication headers:

```bash
# View headers used for authenticated request
cat /tmp/notebooklm-traffic.har | jq '.log.entries[] | select(.request.url | contains("lh3.googleusercontent.com")) | .request.headers'

# Output:
# [
#   {
#     "name": "Authorization",
#     "value": "Bearer gAA..."
#   },
#   {
#     "name": "Cookie",
#     "value": "session_id=..."
#   }
# ]
```

View response status and headers:

```bash
# Check response details
cat /tmp/notebooklm-traffic.har | jq '.log.entries[] | select(.request.url | contains("lh3.googleusercontent.com")) | {url: .request.url, status: .response.status, contentType: .response.headers[] | select(.name == "Content-Type")}'
```

## Use Case 2: Monitoring Specific URL Patterns

### Scenario: Tracking API Calls to a Specific Domain

Use `--monitor-url-pattern` to watch for requests matching a regex pattern:

```bash
# Monitor for API requests
cdp --use-profile "Work Profile" \
    --monitor-url-pattern "api\.example\.com/v[0-9]+/.*" \
    --url https://app.example.com \
    --wait-for-url-change
```

### Combined: Monitor + HAR + Extract

Monitor specific patterns while recording everything:

```bash
# Capture all traffic to HAR while monitoring for specific pattern
cdp --use-profile "Work Profile" \
    --har /tmp/full-session.har \
    --monitor-url-pattern "checkout|payment" \
    --url https://app.example.com \
    --interactive
```

Then analyze the monitored URLs:

```bash
# Extract payment-related URLs and their status codes
cat /tmp/full-session.har | jq '.log.entries[] | select(.request.url | test("checkout|payment")) | {url: .request.url, status: .response.status, time: .time}'
```

## Use Case 3: Extracting Response Data

### Option 1: Analyzing HAR File After Capture

After capturing with `--har`, analyze responses:

```bash
# Extract all POST requests with their response status
cat /tmp/session.har | jq '.log.entries[] | select(.request.method == "POST") | {url: .request.url, status: .response.status, mimeType: .response.content.mimeType}'

# Find failed requests (4xx, 5xx)
cat /tmp/session.har | jq '.log.entries[] | select(.response.status >= 400) | {url: .request.url, status: .response.status}'

# Calculate request timings
cat /tmp/session.har | jq '.log.entries[] | {url: .request.url, time: .time}' | jq -s 'sort_by(.time) | reverse | .[0:10]'  # Top 10 slowest
```

### Option 2: JavaScript-Based Extraction

Extract data directly from the page using JavaScript:

#### Extract All Links

```bash
cdp --use-profile "Default" \
    --url https://example.com \
    --js 'Array.from(document.querySelectorAll("a")).map(a => ({text: a.textContent, href: a.href}))'
```

#### Extract Authenticated Download URLs

```bash
cdp --use-profile "Default" \
    --url https://example.com/downloads \
    --js '
      Array.from(document.querySelectorAll("[data-download-url]"))
        .map(el => ({
          name: el.textContent,
          url: el.getAttribute("data-download-url"),
          size: el.getAttribute("data-file-size")
        }))
    '
```

#### Extract Form Data Before Submission

```bash
cdp --use-profile "Work Profile" \
    --url https://api.example.com/login \
    --js '
      {
        username: document.querySelector("input[name=username]")?.value,
        rememberMe: document.querySelector("input[name=remember]")?.checked,
        formAction: document.querySelector("form")?.action
      }
    '
```

### Option 3: CSS Selector-Based Extraction

Use the `--extract` flag for simpler CSS selector extraction:

```bash
# Extract all heading text
cdp --use-profile "Default" \
    --url https://example.com \
    --extract "h1"  # extracts text by default

# Extract with different modes
cdp --use-profile "Default" \
    --url https://example.com \
    --extract "a" \
    --extract-mode "html"  # get full HTML of elements

# Extract attribute values
cdp --use-profile "Default" \
    --url https://example.com \
    --extract "a[href*=api]" \
    --extract-mode "attr:href"  # get href attributes
```

## Complete Workflow Example: Audio Download

Here's a complete workflow for downloading authenticated audio files:

### Step 1: Set Up Profile (One Time)

```bash
# List your available profiles
cdp --list-profiles

# Note: Make sure you're already logged in to the service (NotebookLM, etc.)
# in your browser using that profile
```

### Step 2: Discover Audio URL

```bash
# Record network traffic while playing audio
cdp --use-profile "Default" \
    --har /tmp/audio-discovery.har \
    --url https://notebooklm.google.com

# Navigate to notebook and play audio, then Ctrl+C
```

### Step 3: Extract URL and Headers

```bash
# Find the audio CDN request
cat /tmp/audio-discovery.har | jq '.log.entries[] | select(.request.url | contains("mp3") or contains("audio")) | {url: .request.url, status: .response.status}'

# Extract headers (you'll need these for direct download)
cat /tmp/audio-discovery.har | jq '.log.entries[] | select(.request.url | contains("lh3.googleusercontent")) | .request.headers'
```

### Step 4: Verify Access with CDP

```bash
# Test that you can fetch the audio with the profile's cookies
cdp --use-profile "Default" \
    --url https://notebooklm.google.com \
    --js '
      fetch("https://lh3.googleusercontent.com/your-audio-id/audio.mp3")
        .then(r => r.blob())
        .then(blob => console.log("Audio fetched:", blob.size, "bytes"))
        .catch(e => console.error("Error:", e.message))
    '
```

## Combining Multiple Features

### Advanced Example: Full Authentication Analysis

```bash
# Capture everything, monitor auth endpoints, extract tokens
cdp --use-profile "Work Profile" \
    --har /tmp/full-auth-flow.har \
    --monitor-url-pattern "auth.*" \
    --url https://app.example.com/login \
    --interactive
```

Then analyze:

```bash
# 1. Find all auth endpoints
cat /tmp/full-auth-flow.har | jq '.log.entries[] | select(.request.url | contains("auth")) | .request.url'

# 2. Extract auth headers
cat /tmp/full-auth-flow.har | jq '.log.entries[] | select(.response.headers[] | select(.name == "Set-Cookie")) | {url: .request.url, cookies: .response.headers[] | select(.name == "Set-Cookie")}'

# 3. Find token endpoints
cat /tmp/full-auth-flow.har | jq '.log.entries[] | select(.request.method == "POST" and (.response.status == 200 or .response.status == 201)) | {url: .request.url, status: .response.status}'

# 4. Calculate total request time
cat /tmp/full-auth-flow.har | jq '[.log.entries[].time] | add'
```

## Feature Combinations

### Profile + HAR + JavaScript Extraction

Combine all features for maximum capability:

```bash
#!/bin/bash

# 1. Record session
cdp --use-profile "Default" \
    --har /tmp/session.har \
    --url "https://app.example.com" \
    --js '
      // Wait for page to fully load
      await new Promise(r => setTimeout(r, 2000));

      // Extract data
      {
        title: document.title,
        apiUrls: Array.from(document.querySelectorAll("[data-api]")).map(el => el.getAttribute("data-api")),
        tokens: Array.from(document.querySelectorAll("meta[name*=token]")).map(el => ({name: el.name, value: el.content}))
      }
    '

# 2. Analyze HAR separately
echo "=== Captured Network Requests ==="
cat /tmp/session.har | jq '.log.entries[] | {url: .request.url, status: .response.status, time: .time}' | jq -s 'sort_by(-.time) | .[0:5]'
```

## Troubleshooting

### Issue: "Profile is locked"

**Solution**: Close the browser using that profile, or use `--connect-existing`:

```bash
# Option 1: Close Chrome first, then use profile
cdp --use-profile "Default" --har /tmp/session.har --url https://example.com

# Option 2: Connect to already-running Chrome
chrome --remote-debugging-port=9222 --profile-directory="Default"
cdp --debug-port 9222 --har /tmp/session.har
```

### Issue: HAR file is empty or missing requests

**Solution**: Ensure you wait long enough for requests to complete:

```bash
# Use interactive mode and manually verify requests
cdp --use-profile "Default" --har /tmp/session.har --url https://example.com --interactive

# Then in the browser, interact with the page, then Ctrl+C
```

### Issue: Authentication not working

**Cause**: The profile might not have valid cookies/session
**Solution**:

1. Verify the profile has active authentication:
   ```bash
   cdp --use-profile "Default" \
       --url https://example.com \
       --js 'document.cookie'  # Check if cookies exist
   ```

2. Log in interactively first:
   ```bash
   # Open the browser and log in manually
   cdp --use-profile "Default" --url https://example.com
   ```

3. If still failing, try creating a fresh session:
   ```bash
   cdp --profile-dir /tmp/fresh-profile \
       --url https://example.com/login
   ```

### Issue: Response body not captured in HAR

**Note**: CDP HAR captures metadata but the response body capture is being tracked in a separate feature (see `bd show chrome-to-har-61` for details).

**Workaround**: Extract what you need using JavaScript while recording:

```bash
cdp --use-profile "Default" \
    --har /tmp/metadata.har \
    --url https://example.com/api \
    --js 'Array.from(document.querySelectorAll("script[type=application/json]")).map(s => JSON.parse(s.textContent))'
```

## Real-World Examples

### Example 1: Capture GitLab API Traffic

```bash
cdp --use-profile "Work Profile" \
    --har /tmp/gitlab.har \
    --url https://gitlab.com/api/graphql \
    --js 'console.log("Captured API calls"); fetch("/api/v4/user").then(r => r.json()).then(d => console.log(d))'
```

### Example 2: Download Authenticated PDF

```bash
# Step 1: Find the PDF URL
cdp --use-profile "Work Profile" \
    --har /tmp/pdf-flow.har \
    --url https://app.example.com/reports

# Step 2: Extract PDF URL from HAR
PDF_URL=$(cat /tmp/pdf-flow.har | jq -r '.log.entries[] | select(.request.url | contains(".pdf")) | .request.url' | head -1)

# Step 3: Download using curl with cookies from Chrome
curl --cookie "$(cat /tmp/cookies.txt)" "$PDF_URL" -o report.pdf
```

### Example 3: Monitor WebSocket Connections

```bash
cdp --use-profile "Default" \
    --har /tmp/websocket-session.har \
    --url https://app.example.com/realtime \
    --js '
      // Log WebSocket connections
      const ws = new WebSocket("wss://app.example.com/ws");
      ws.onmessage = (event) => console.log("WebSocket message:", event.data);
    '
```

## Related Documentation

- **Main CDP Documentation**: See `docs/cdp.md`
- **HAR Specification**: https://w3c.github.io/web-performance/specs/HAR/Overview.html
- **Chrome DevTools Protocol**: https://chromedevtools.github.io/devtools-protocol/
- **Chrome Profile Management**: https://support.google.com/chrome/answer/2364824

## See Also

- Bead chrome-to-har-61: CDP Response Body Capture
- Bead chrome-to-har-60: CDP Script Format Implementation
- Bead chrome-to-har-73: Brave Profile + Debug Port Workaround
