# cdp go-team-review run

- timestamp: 20260506-144036
- slug:      20260506-144036
- notebook:  a862fc39-6861-4e8c-b28b-6b449f8bb933
- repo:      /Volumes/tmc/go/src/github.com/tmc/cdp
- branch:    main
- head:      d24180e2eebcf44b0850703751b20371aed0e511
- path scope: (whole repo)

## Focuses
- **topology** — Package topology and dependency graph
- **naming-packages** — Package names
- **naming-symbols** — Exported symbol naming
- **api-design** — Public API design
- **consistency** — Cross-package consistency
- **smells** — Code smells and anti-patterns
- **cmd-cdp-deep-dive** — cmd/cdp deep dive (REPL + MCP + recorders)
- **cdpscript-cdpscripttest** — cdpscript / cdpscripttest split
- **docs-discipline** — Documentation discipline (doc.go, godoc, in-tree docs)

## Recent commits
```
d24180e (HEAD -> main) docs/planning: add cdp-cleanup design doc v3
525f18e internal/sources: arm incremental before Debugger.enable replay burst
323950f examples/claude-debug: lowercase debugging guide filename
40d08e5 internal/browser: rename README_TEST.md to testing.md
8e920ba cmd/churl: promote proxy docs to docs/churl-proxy.md
d611f11 cmd/churl: move roadmap and mirror design notes under docs/internal/roadmap
ba99631 cmd: move per-binary ROADMAP files under docs/internal/roadmap
7ce95f4 cdpscripttest: emit coverage json per fixture
6821ad6 cmd/cdp: test analyze_trace core web vitals
31b8d54 cmd/cdp: add trace analysis mcp tools
d98c231 cdpscript: expose script usage and argv fixtures
5a43d3b internal/recorder: capture WebSocket frames in HAR/JSONL output
8d2c3dc internal/sources,cmd/cdp: route fetches through per-event session context
b12eb06 cmd/cdp,internal/sources: re-attach source listener on tab switch
f09ebc6 internal/sources: add CDP_SOURCES_DEBUG knob for HandleEvent tracing
fcc1e34 cmd/cdp,internal/sources: register source listener before Debugger.enable
4333f57 internal/sources: write file:// sources under the file/ bucket
eaccd06 cmd/cdp: attach to existing target for plain shell+remote
d4021b9 docs: add browser-harness gap analysis and improvements
1f2e3e3 docs: add cdp-harness plan series
```
