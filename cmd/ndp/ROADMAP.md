# NDP - Node Debug Protocol CLI Tool Roadmap

## Overview
NDP provides unified debugging for Node.js and Chrome applications using the Chrome DevTools Protocol, bringing V8 Inspector features to the command line.

## Current State (v1.0.0)

✅ **Core Features**: Node.js process attachment via Inspector Protocol, Chrome/Chromium debugging, session management, CPU/heap profiling, breakpoint management, watch expressions, interactive REPL, target discovery, V8 Inspector integration

## Future Development

Most planned features would benefit from being tracked as beads when needed. Key areas for potential development:

### Debugging Enhancements
- Advanced breakpoints (hit counts, groups, templates)
- V8 console improvements (multi-line REPL, autocomplete, history)
- Source management (source maps, pretty-print, search)

### Node.js Specific
- Process management (start with flags, attach by PID/name, cluster debugging)
- Module analysis (require cache, dependency tree, hot reload)
- Performance tools (event loop monitoring, async traces, memory leaks)

### Chrome/Browser Features
- Tab management (group operations, filtering, session save/restore)
- Extension debugging (background pages, content scripts)
- Network features (request interception, WebSocket monitoring, HAR export)

### Integration
- IDE support (VS Code DAP, Vim/NeoVim, Emacs)
- CI/CD debugging (automated debugging, test failure analysis)
- Container debugging (Kubernetes, Docker, remote SSH)

### Advanced Features
- Sampling & profiling (timeline, allocation tracking, flame graphs)
- Memory analysis (heap diff, retainer analysis, GC monitoring)
- Runtime modification (hot code replacement, dynamic instrumentation)

### UI Options
- TUI (split-pane interface, variable panels, call stack viz)
- Web dashboard (remote interface, team collaboration, session replay)

## Related Tools
- **node-inspect**: Built-in Node.js debugger (simpler)
- **cdp**: CDP CLI tool (related)
- **chdb**: Chrome debugging (related)
- **chrome-remote-interface**: Low-level CDP library

## Use Cases
1. Interactive debugging during development
2. Debug live Node.js services in production
3. Investigate test failures
4. Profile and optimize applications
5. Understand Node.js/V8 internals
6. Script debugging workflows

---

**Note**: Create beads for specific features as they become priorities. This roadmap is intentionally high-level to avoid duplication with the bead tracker.

**Last Updated**: 2025-01-17
**Status**: Stable - Enhancement on demand
**Version**: 1.0.0
