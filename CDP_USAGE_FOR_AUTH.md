# CDP Tool Usage for Authenticated Downloads

## Overview

The CDP (Chrome DevTools Protocol) CLI tool provides powerful capabilities for browser automation, particularly useful for authenticated resource downloads like NotebookLM audio files.

## Quick Start

```bash
# Build CDP tool
cd /Volumes/tmc/go/src/github.com/tmc/misc/chrome-to-har
go build ./cmd/cdp

# List available Chrome profiles
./cmd/cdp/cdp --list-profiles
```

## Use Cases for NLM Audio Downloads

### 1. Profile-Based Authentication

Use your existing Chrome profile with all cookies and authentication:

```bash
# Open browser with your profile (interactive mode)
./cmd/cdp/cdp --use-profile Default --url https://notebooklm.google.com --interactive
```

### 2. Capture Network Traffic (HAR)

Record all network requests to understand authentication flow:

```bash
# Capture HAR file while browsing NotebookLM
./cmd/cdp/cdp --use-profile Default \
  --har /tmp/notebooklm-session.har \
  --url https://notebooklm.google.com \
  --interactive

# After capturing audio download, examine the HAR file
cat /tmp/notebooklm-session.har | jq '.log.entries[] | select(.request.url | contains("lh3.googleusercontent.com"))'
```

### 3. Monitor Specific URL Patterns

Watch for specific CDN URLs (like Google Cloud Storage audio files):

```bash
# Monitor for audio file downloads
./cmd/cdp/cdp --use-profile Default \
  --monitor-url-pattern "lh3\.googleusercontent\.com.*\\.mp3" \
  --url https://notebooklm.google.com \
  --wait-for-url-change
```

### 4. Execute JavaScript for Extraction

Extract audio URLs directly from the page:

```bash
# Get all audio element sources
./cmd/cdp/cdp --use-profile Default \
  --url https://notebooklm.google.com \
  --js 'Array.from(document.querySelectorAll("audio")).map(a => a.src).join("\n")'

# Or more complex extraction
./cmd/cdp/cdp --use-profile Default \
  --url https://notebooklm.google.com \
  --js '
    const audioElements = document.querySelectorAll("audio");
    const results = [];
    audioElements.forEach(audio => {
      results.push({
        src: audio.src,
        currentSrc: audio.currentSrc,
        duration: audio.duration
      });
    });
    JSON.stringify(results, null, 2);
  '
```

## Complete Workflow Example

### Step 1: Discover Audio URL Pattern

```bash
# Start CDP with profile and HAR capture
./cmd/cdp/cdp --use-profile Default \
  --har /tmp/nlm-traffic.har \
  --url https://notebooklm.google.com \
  --interactive

# In the browser: Navigate to your notebook and play audio
# Then exit CDP (Ctrl+C)

# Analyze captured traffic
cat /tmp/nlm-traffic.har | jq -r '.log.entries[].request.url' | grep -i "audio\|mp3\|googleusercontent"
```

### Step 2: Extract Authentication Headers

```bash
# Find the exact headers used for authenticated request
cat /tmp/nlm-traffic.har | jq '.log.entries[] | select(.request.url | contains("lh3.googleusercontent.com")) | .request.headers'
```

### Step 3: Test with CDP JavaScript

```bash
# Fetch the audio URL with proper auth context
./cmd/cdp/cdp --use-profile Default \
  --url https://notebooklm.google.com \
  --js '
    fetch("https://lh3.googleusercontent.com/YOUR_AUDIO_URL")
      .then(r => r.blob())
      .then(blob => console.log("Audio size:", blob.size))
      .catch(e => console.error("Fetch failed:", e));
  '
```

## Integration with Your Code

Your existing `DownloadWithBrowser` implementation in `internal/auth/auth.go` follows similar patterns:

```go
// Your current approach (which is correct!)
func (a *Authenticator) DownloadWithBrowser(url, profileName string) ([]byte, error) {
    // 1. Find profile path
    profilePath := a.findProfilePath(profileName)

    // 2. Create temp copy
    tempDir := copyProfile(profilePath)

    // 3. Launch Chrome with profile
    ctx := chromedp.NewContext(context.Background(),
        chromedp.UserDataDir(tempDir))

    // 4. Capture response via CDP
    chromedp.ListenTarget(ctx, func(ev interface{}) {
        if resp, ok := ev.(*network.EventResponseReceived); ok {
            // Capture response body
        }
    })
}
```

### CDP Can Help You:

1. **Discover URLs**: Use CDP interactively to find the exact audio URLs
2. **Validate Headers**: Capture HAR to see what headers are actually needed
3. **Test Approaches**: Quickly test different authentication strategies
4. **Debug Issues**: See exactly what Chrome is doing vs. your code

## CDP Features Being Enhanced

See beads for tracked improvements:

```bash
bd show chrome-to-har-62  # CDP Profile + HAR Workflow Documentation (P0)
bd show chrome-to-har-61  # CDP Response Body Capture (P1)
bd show chrome-to-har-60  # CDP Script Format Implementation (P1)
```

## Advanced: CDP Interactive Mode

CDP supports an interactive REPL with enhanced commands:

```bash
# Start interactive mode
./cmd/cdp/cdp --use-profile Default --enhanced --interactive

# Then use commands like:
@goto https://notebooklm.google.com
@wait #content
@click button.play-audio
@screenshot /tmp/playing.png
```

## Troubleshooting

### Issue: "Profile is locked"
Chrome must be closed before using profile. Or use `--connect-existing` to attach to running Chrome.

### Issue: "Response body not captured"
CDP currently captures HAR metadata but not response bodies. This is tracked in chrome-to-har-61.

### Issue: "Authentication failed"
Make sure you're using the correct profile name. List profiles with `--list-profiles`.

## Related Tools

- `churl`: Chrome-powered curl (just got --har flag added!)
- `chdb`: Full Chrome debugger
- `chrome-to-har`: Main HAR capture tool

## References

- **[Comprehensive CDP Profile + HAR Workflow Guide](docs/CDP_PROFILE_HAR_WORKFLOW.md)** - Full documentation for authenticated network capture workflows (NEW!)
- CDP Roadmap: `/Volumes/tmc/go/src/github.com/tmc/misc/chrome-to-har/cmd/cdp/ROADMAP.md`
- CDP Script Format: `/Volumes/tmc/go/src/github.com/tmc/misc/chrome-to-har/cmd/cdp/CDP_SCRIPT_FORMAT.md`

---

**Note**: This file is part of the P0 Bead chrome-to-har-62 work on documenting CDP Profile + HAR workflows. For comprehensive examples and advanced usage patterns, see the full guide linked above.
