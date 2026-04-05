# Chrome Extension Development Tools

## Context

cdp controls the browser but has no tools for managing Chrome extensions. Extension developers using cdp as an MCP server can't reload, inspect, or debug their extensions without leaving the session. Adding a small set of extension management tools creates a tight inner loop: edit code → reload → test → check console — all via MCP.

## Tools (~5 tools, ~300 lines)

### 1. list_extensions

List installed extensions with status.

```
Tool: list_extensions
Inputs:
  enabled_only: bool (optional, default false)

Output: [{
  id, name, version, enabled, description,
  type: "extension" | "app" | "theme",
  permissions: [string],
  has_background: bool
}]
```

**Implementation**: `Runtime.evaluate` with `chrome.management.getAll()` on a chrome:// page context. The `chrome.management` API is available in pages with appropriate permissions — we may need to navigate to `chrome://extensions` first or use `Target.createTarget` with a `chrome-extension://` URL.

**Alternative**: Use `SystemInfo.getProcessInfo` or enumerate CDP targets — extension service workers show up as separate targets with `type: "service_worker"` and URLs starting with `chrome-extension://`.

**File**: `cmd/cdp/mcp_extension_tools.go`

### 2. reload_extension

Reload an unpacked extension by ID. Essential for the dev inner loop.

```
Tool: reload_extension
Inputs:
  id: string (extension ID)
```

**Implementation**: Two approaches:
- **Preferred**: Navigate to `chrome://extensions`, use `Runtime.evaluate` to call `chrome.developerPrivate.reload(id)` (available on the extensions page).
- **Fallback**: `chrome.management.setEnabled(id, false)` then `setEnabled(id, true)`.
- **Cleanest**: Use the CDP `Extensions` domain if available (Chrome 148+ may have it), or dispatch click on the extension's reload button via DOM manipulation on `chrome://extensions`.

**File**: `cmd/cdp/mcp_extension_tools.go`

### 3. install_extension

Load an unpacked extension from a local path.

```
Tool: install_extension
Inputs:
  path: string (absolute path to unpacked extension directory)
```

**Implementation**: There's no direct CDP command for this. Options:
- **Best**: Use `chrome.developerPrivate.loadUnpacked(path)` via `Runtime.evaluate` on `chrome://extensions` page. This API is available when developer mode is enabled.
- **Alternative**: Document that users should launch cdp with `--chrome-flags="--load-extension=/path/to/ext"` for persistent loading.

**File**: `cmd/cdp/mcp_extension_tools.go`

### 4. extension_console

Get console output and errors from an extension's service worker.

```
Tool: extension_console
Inputs:
  id: string (extension ID)
  clear: bool (optional)

Output: {
  messages: [{type, text, timestamp}],
  errors: [{text, url, line, column, stack}]
}
```

**Implementation**: Extensions run as separate CDP targets. Use `Target.getTargets()` to find the service worker target matching `chrome-extension://<id>/`. Attach to it via `Target.attachToTarget`, then listen for `Runtime.consoleAPICalled` and `Runtime.exceptionThrown` events on that target.

This is the most complex tool — it requires managing a secondary CDP target attachment. chromedp supports this via `chromedp.NewContext` with `chromedp.WithTargetID`.

**File**: `cmd/cdp/mcp_extension_tools.go`

### 5. extension_evaluate

Evaluate JavaScript in the context of an extension's service worker.

```
Tool: extension_evaluate
Inputs:
  id: string (extension ID)
  expression: string (JS to evaluate)

Output: { result: any }
```

**Implementation**: Same target attachment as extension_console — find the service worker target, attach, then `Runtime.evaluate` in that context.

**File**: `cmd/cdp/mcp_extension_tools.go`

## The Dev Loop

An agent developing a Chrome extension uses these tools together:

1. `install_extension {path: "/path/to/my-ext"}` — load unpacked
2. `navigate {url: "https://test-site.com"}` — go to test page
3. `extension_console {id: "abc123"}` — check for errors
4. `screenshot` — verify extension UI renders
5. *Agent edits extension code*
6. `reload_extension {id: "abc123"}` — hot-reload
7. `navigate {url: "https://test-site.com"}` — re-test
8. `extension_console {id: "abc123"}` — check errors again
9. Repeat 5-8

## Target Discovery

Key insight: Chrome exposes extension targets via `Target.getTargets()`. Each extension service worker appears as:

```json
{
  "targetId": "...",
  "type": "service_worker",
  "title": "Service Worker chrome-extension://abcdef.../",
  "url": "chrome-extension://abcdef.../background.js",
  "attached": false
}
```

Extension pages (popup, options, sidepanel) appear as:

```json
{
  "type": "page",
  "url": "chrome-extension://abcdef.../popup.html"
}
```

We can use this to find and attach to any extension target.

## Implementation Notes

- All tools go in a single new file: `cmd/cdp/mcp_extension_tools.go`
- The `chrome.management` and `chrome.developerPrivate` APIs require navigating to a privileged page (`chrome://extensions`). Cache this — don't navigate away from the user's page.
- For extension_console/extension_evaluate, use `Target.getTargets` + `chromedp.NewContext(ctx, chromedp.WithTargetID(tid))` to attach to the service worker.
- Add unit tests for target URL matching (`chrome-extension://id/` extraction).

## Complexity

| Tool | Lines | Difficulty |
|------|-------|-----------|
| list_extensions | ~40 | Low — target enumeration |
| reload_extension | ~40 | Low-medium — needs chrome:// page |
| install_extension | ~50 | Medium — developerPrivate API |
| extension_console | ~80 | Medium — secondary target attachment |
| extension_evaluate | ~50 | Medium — reuses console's target attach |
| **Total** | **~260** | |
