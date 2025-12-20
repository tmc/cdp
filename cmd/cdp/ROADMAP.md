# CDP - Chrome DevTools Protocol CLI Tool Roadmap

## Overview
CDP is a command-line tool for Chrome DevTools Protocol interaction, providing interactive and scripted browser automation.

## Current State (v1.0)

✅ **Core Features**: Interactive REPL, remote Chrome connection, profile management, HAR recording, JS execution, 60+ command aliases, enhanced Playwright-like commands, screenshot/PDF capture

## Active Development (Tracked in Beads)

### High Priority
- **CDP Script Format Implementation** - See `CDP_SCRIPT_FORMAT.md`
  - Core parser (beads: chrome-to-har-*)
  - Command execution engine
  - Browser integration
  - Testing & validation

### Stability & UX
- Exit codes for scripting (bead exists)
- Machine-parseable error messages (bead exists)
- Browser discovery improvements
- Profile detection fixes (Brave support - beads exist)

### Feature Requests (Beadified)
- `--output` flag for saving content
- `--headers` flag for custom HTTP headers
- `--cookie` flag for cookie setting
- `--screenshot` flag for headless captures
- `--extract` for CSS selector extraction

## Future Considerations

**Session Management**: Save/restore browser states, breakpoint persistence, multi-tab coordination

**Advanced Debugging**: Step debugging, stack traces, variable watching, performance profiling

**Network Features**: Request/response modification, throttling presets, certificate analysis

**Multi-Browser**: Firefox, Safari/WebKit, Edge support

**Integration**: Playwright/Puppeteer compatibility, testing frameworks, Chrome Extension API

## Related Tools
- **churl**: Chrome-powered HTTP client (uses CDP)
- **chdb**: Full-featured Chrome debugger
- **ndp**: Node.js debugger

---

**Note**: Most roadmap items are tracked as beads in the issue tracker. Run `bd list | grep -i cdp` to see all CDP-related work items.

**Last Updated**: 2025-01-17
**Status**: Active Development
