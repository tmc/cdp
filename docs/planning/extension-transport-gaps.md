# Extension Tools: Transport Gaps & Implementation Plan

## Problem

The CDP Extensions domain (`Extensions.loadUnpacked`, `Extensions.getExtensions`, `Extensions.uninstall`, storage APIs) requires Chrome to be launched with `--remote-debugging-pipe` transport. We use `--remote-debugging-port` (WebSocket), so the Extensions domain is unavailable.

Additionally, DevTools-only extensions (like our coverage panel — `devtools_page` manifest key, no `background` service worker) don't create CDP-visible targets, so `extension_console` and `extension_evaluate` can't attach to them.

## Research Findings (E794, 2026-04-05)

### Pipe transport: dead end

- **chromedp has NO pipe transport** — entire transport layer is WebSocket-only (gobwas/ws)
- Open issue chromedp/chromedp#1607, PR #1609 unmerged
- No Go CDP library has production pipe support
- CDP Extensions domain is inaccessible from our stack

### --load-extension: deprecated

- Launch-time only — cannot add paths after Chrome starts
- **Being removed from branded Chrome in Chrome 137** — must use CDP or WebDriver BiDi
- For go:embed distribution, runtime install via developerPrivate is the future-proof path

### chrome.developerPrivate: our primary API

Available only on `chrome://extensions` page. Key methods:
- `getExtensionsInfo()` — richer than chrome.management.getAll, includes devtools-only exts
- `loadUnpacked(path)` — may trigger file picker without `--enable-unsafe-extension-debugging`
- `reload(id, options)` — works reliably (already using this)
- `packDirectory(path, keyPath)` — pack .crx
- `updateExtensionConfiguration(config)` — enable/disable, incognito, etc.
- `addHostPermission(id, host)` / `removeHostPermission(id, host)`

### Service worker targets: accessible

- Extension SWs appear as `{"type": "service_worker", "url": "chrome-extension://ID/background.js"}`
- Attachable via `Target.attachToTarget` / `chromedp.WithTargetID`
- `Runtime.evaluate` in SW context gives full access to extension APIs (chrome.storage, chrome.management, etc.)
- Chrome 125+ keeps SW alive during debug sessions

### DevTools panel extensions: no CDP targets

- DevTools-only extensions create no new CDP targets when DevTools is open
- **Fix**: Add a minimal background service worker to our coverage extension (creates a target even if SW does nothing)

## Implementation Plan

### Status

| Tool | Approach | Status |
|------|----------|--------|
| list_extensions | developerPrivate.getExtensionsInfo() on chrome://extensions | Needs fallback wired |
| install_extension | developerPrivate.loadUnpacked() + --enable-unsafe-extension-debugging | Needs testing |
| reload_extension | developerPrivate.reload() on chrome://extensions | Done |
| uninstall_extension | chrome.management.uninstall() in SW context | Not built |
| extension_console | Target.attachToTarget on SW + Runtime.consoleAPICalled | Works for SW exts, broken for devtools-only |
| extension_evaluate | Target.attachToTarget on SW + Runtime.evaluate | Works for SW exts, broken for devtools-only |
| get_extension_storage | Runtime.evaluate in SW: chrome.storage.local.get() | Not built |
| set_extension_storage | Runtime.evaluate in SW: chrome.storage.local.set() | Not built |
| clear_extension_storage | Runtime.evaluate in SW: chrome.storage.local.clear() | Not built |

### Implementation tasks

1. **Wire JS fallback into list_extensions** — when CDP domain fails, fall through to developerPrivate.getExtensionsInfo() via runOnExtensionsPage()
2. **Wire JS fallback into install_extension** — developerPrivate.loadUnpacked(path), document --enable-unsafe-extension-debugging requirement
3. **Add uninstall_extension** — chrome.management.uninstall(id) via SW target context
4. **Add storage tools** (get/set/clear) — Runtime.evaluate in SW context with chrome.storage.local.*
5. **Add minimal background SW to coverage extension** — creates CDP target for console/evaluate attachment
6. **Update go:embed distribution** — extract to ~/.cdp/extensions/, use developerPrivate.loadUnpacked() for runtime install (future-proofs against --load-extension removal)

## Files

- `cmd/cdp/mcp_extension_tools.go` — current implementation
- `cmd/cdp/mcp_extension_tools_test.go` — tests
- `extension/coverage/manifest.json` — our coverage extension (devtools-only, needs SW added)
- `docs/planning/extension-dev-tools.md` — original design
- `docs/known-issues.md` — issues #4 and #5
- `/tmp/collab-E794-ext-research.md` — full research report
