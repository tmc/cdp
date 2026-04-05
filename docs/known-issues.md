# Known Issues — cdp MCP Tools

## Active Issues

### 1. click/type_text/hover/focus/press_key timeout units ambiguous

**Symptom**: Agent passes `timeout: 5000` intending 5 seconds (milliseconds), but `interactionCtx` treats it as 5000 seconds (83 minutes). Tool hangs indefinitely.

**Root cause**: `ClickInput.Timeout` is `int` with no documented units. `interactionCtx()` at `mcp_tools.go:240` multiplies by `time.Second`, so the value is interpreted as seconds. But LLMs and most web APIs use milliseconds.

**Fix options**:
- A) Change `interactionCtx` to treat values > 60 as milliseconds (heuristic)
- B) Change field to `timeout_ms` and document as milliseconds
- C) Keep as seconds but add `timeout_ms` as alternative, prefer it when set
- D) Add validation: clamp to reasonable range (1-120 seconds), reject >120

**Recommendation**: Option A is pragmatic — any timeout value > 60 is almost certainly milliseconds. Add: `if timeoutSec > 60 { timeout = time.Duration(timeoutSec) * time.Millisecond }`. Also update tool descriptions to say "timeout in seconds".

**Observed**: 2026-04-05, A998 session. `click(selector: "a[href='/explore']", timeout: 5000)` hung for 6+ minutes on github.com.

**Files**: `cmd/cdp/mcp_tools.go:240-246`

### 2. click can hang on elements that trigger navigation

**Symptom**: `click` on a link that navigates the page may hang if the navigation changes the DOM before chromedp's click action completes. The element becomes stale mid-action.

**Workaround**: Use `evaluate` with `document.querySelector('a').click()` for navigation-triggering clicks, or use `navigate` directly if the URL is known.

**Files**: `cmd/cdp/mcp_tools.go:248-270`

### 3. Coverage snapshot on minimal-JS pages returns empty

**Symptom**: `get_coverage` after `start_coverage` on server-rendered pages (e.g., Hacker News) returns 0 files because there's little/no JS to profile.

**Not a bug**: Expected behavior — V8 coverage only tracks JavaScript execution. Document this in tool description.

**Observed**: 2026-04-05, A998 session on news.ycombinator.com.
