# Platform Integration Ideas: axmcp + tmc/apple

*2026-04-05 — Ideas from xcmcp/axmcp and tmc/apple that could enhance cdp/ndp*

## Source Projects

- **axmcp** (`github.com/tmc/xcmcp/cmd/axmcp`) — macOS Accessibility MCP server. 25+ tools for AX tree inspection, UI interaction, OCR, screenshot diffing. Uses Apple Accessibility framework via purego.
- **tmc/apple** (`github.com/tmc/apple`) — cgo-free Go bindings for 43+ Apple frameworks. ScreenCaptureKit, Vision (OCR), NaturalLanguage, WebKit, AppKit, Security (keychain).

## Ideas Worth Adapting

### 1. OCR Fallback for Element Interaction

**axmcp pattern**: When AX tree queries fail (custom UIs, canvas, WebGL, iframes), fall back to Apple Vision OCR on a screenshot. `ax_ocr_click` finds visible text by OCR and clicks its center coordinates.

**cdp adaptation**: Add `ocr_click` tool that:
1. Takes a screenshot via Page.captureScreenshot
2. Runs OCR (Apple Vision via tmc/apple, or Tesseract for portability)
3. Finds the target text in OCR results
4. Clicks at the text's center coordinates via Input.dispatchMouseEvent

**Why**: Canvas-heavy apps (Figma, Google Docs canvas mode, WebGL games) have no DOM to query. OCR is the only way to find interactive text. Also useful for PDF viewers, embedded images with text.

**Dependency**: `github.com/tmc/apple/vision` on macOS. For cross-platform, could shell out to `tesseract` or use a Go OCR lib.

**Effort**: ~150 lines + vision dependency

### 2. Action Screenshot Diff

**axmcp pattern**: `ax_action_screenshot` captures before/after screenshots around an action, returns a diff PNG highlighting changed pixels. `ax_ocr_action_diff` does OCR before/after with word-level diff.

**cdp adaptation**: Add `action_diff` tool that:
1. Screenshots before
2. Executes an action (click, type, navigate)
3. Screenshots after
4. Returns: before image, after image, diff image (changed pixels in red), and optionally OCR diff

**Why**: Agents can verify their actions had the expected visual effect. "I clicked the button — did the modal open?" Without this, agents must re-snapshot and reason about the entire page.

**Effort**: ~200 lines (screenshot + pixel diff + action execution)

### 3. OCR-Based Page Understanding

**axmcp pattern**: `ax_ocr` returns text with coordinates (local + screen), confidence scores, and ASCII spatial layout rendering.

**cdp adaptation**: Add `ocr_page` tool that:
1. Screenshots the current viewport
2. Runs OCR
3. Returns structured results: `[{text, x, y, w, h, confidence}]`
4. Optional: ASCII spatial layout rendering (like axmcp does)

**Why**: Supplements page_snapshot for visual content. Catches text in images, canvas, SVG, custom-rendered fonts, iframes that resist CDP inspection. The spatial layout is particularly useful for agents understanding page structure.

**Effort**: ~100 lines + vision dependency

### 4. Window-Level Browser Control

**tmc/apple pattern**: ScreenCaptureKit enumerates all windows. AppKit can move, resize, raise, minimize windows by PID.

**cdp adaptation**: Add tools for browser window management:
- `list_browser_windows` — enumerate Chrome/Brave windows (not just CDP targets, but actual OS windows)
- `move_browser_window` — reposition/resize the browser window
- `raise_browser_window` — bring to front
- `screenshot_browser_window` — OS-level screenshot (captures DevTools, extensions panel, Chrome UI — things CDP can't see)

**Why**: CDP screenshots only capture the page content. OS-level screenshots capture the full browser chrome, DevTools panels, extension popups, permission dialogs. Essential for debugging automation that involves browser UI.

**Dependency**: macOS only (ScreenCaptureKit + AppKit). Could abstract behind interface for future Linux/Windows support.

**Effort**: ~200 lines + apple framework dependency

### 5. Keychain Integration for Credentials

**tmc/apple pattern**: Security framework provides keychain read/write. Encrypted credential storage at OS level.

**cdp adaptation**: Add `credential_store` tool that:
- Stores site credentials in macOS Keychain (not plaintext files)
- Retrieves credentials by domain for auto-fill
- Works with save_state/load_state for complete session management

**Why**: agent-browser has an auth vault. Our save_state handles cookies but not passwords. Keychain is the right place for credentials on macOS — encrypted, per-user, survives reinstalls.

**Dependency**: macOS only. `github.com/tmc/apple/security`

**Effort**: ~100 lines

### 6. NaturalLanguage Embeddings for Smart Element Matching

**tmc/apple pattern**: NaturalLanguage framework provides word embeddings with cosine distance — find semantically similar words without an LLM call.

**cdp adaptation**: Enhance element matching in find_element / @ref resolution:
- When exact text match fails, use NLP embeddings to find semantically closest element
- "click the login button" matches `<button>Sign In</button>` via embedding similarity
- Useful for stale ref recovery: if role+name fails, try semantic similarity

**Why**: Agents describe elements in natural language. Exact matching is brittle. Semantic matching bridges the gap without round-tripping to an LLM.

**Dependency**: macOS only. Would need fallback (substring/fuzzy match) on other platforms.

**Effort**: ~80 lines

### 7. Vision-Based Visual Regression

**tmc/apple pattern**: Vision framework does face detection, object detection, document segmentation.

**cdp adaptation**: Add `visual_check` tool that:
- Captures screenshot
- Runs Vision document segmentation to identify regions (header, nav, content, footer)
- Compares against a baseline screenshot using structural similarity
- Reports which regions changed

**Why**: More intelligent than pixel diff. Understands page structure, ignores minor rendering differences (font smoothing, animations), focuses on meaningful changes.

**Dependency**: macOS only. Vision framework.

**Effort**: ~200 lines

### 8. Multi-App Coordination

**axmcp pattern**: Operates across any macOS app. Can switch between apps, query multiple windows, send keystrokes to background apps.

**cdp adaptation**: When cdp MCP server and axmcp MCP server both run, an agent can:
- Use cdp for in-browser automation
- Use axmcp for browser-external actions (file dialogs, OS permissions, Finder interactions)
- Coordinate: cdp downloads file → axmcp opens in Finder → axmcp drags to another app

**Implementation**: Not a code change — document the two-server pattern (like Electron docs). Add a `docs/multi-mcp-patterns.md` describing cdp + axmcp + ndp coordination.

**Effort**: Documentation only

## Priority Assessment

| Idea | Value | Effort | Platform | Priority |
|------|-------|--------|----------|----------|
| OCR fallback clicking | High | ~150 lines | macOS (portable w/ tesseract) | 1 |
| Action screenshot diff | High | ~200 lines | Cross-platform | 2 |
| OS-level browser screenshots | High | ~200 lines | macOS | 3 |
| OCR page understanding | Medium | ~100 lines | macOS (portable) | 4 |
| Multi-MCP coordination docs | Medium | Docs only | N/A | 5 |
| Keychain credentials | Medium | ~100 lines | macOS only | 6 |
| NLP smart element matching | Low-medium | ~80 lines | macOS only | 7 |
| Vision visual regression | Low | ~200 lines | macOS only | 8 |

## Architecture Notes

- macOS-only features should be behind build tags (`//go:build darwin`)
- Use interfaces so portable fallbacks can be swapped in (tesseract OCR, pixel diff without Vision)
- The `github.com/tmc/apple` dependency is already available in the same GOPATH
- axmcp's tool patterns (screenshot → OCR → click) map cleanly to our MCP tool model
- Consider a shared `internal/vision/` package if multiple tools need OCR
