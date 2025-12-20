# Chrome-to-HAR Debugging Tools Suite

This repository now includes three powerful debugging tools that extend the original Chrome-to-HAR functionality:

1. **CDP** - Enhanced Chrome DevTools Protocol CLI
2. **NDP** - Node Debug Protocol (unified Node.js and Chrome debugging)
3. **CHDB** - Chrome Debugger (comprehensive Chrome DevTools access)

## Tool Overview

### CDP (Chrome DevTools Protocol CLI)
The original enhanced CDP tool with improved commands and aliases.

**Key Features:**
- Interactive CDP command execution
- Enhanced command aliases
- Network recording and HAR generation
- Screenshot and PDF generation
- Browser discovery and management

### NDP (Node Debug Protocol)
Unified debugging for both Node.js and Chrome applications.

**Key Features:**
- Node.js debugging with V8 Inspector Protocol
- Chrome debugging capabilities
- Interactive REPL with multi-target support
- Breakpoint management across platforms
- Session persistence and scripting
- Watch expressions and variable inspection

### CHDB (Chrome Debugger)
Comprehensive Chrome debugging with full DevTools access.

**Key Features:**
- Complete Chrome DevTools functionality from CLI
- DevTools-in-DevTools capability
- Interactive console sessions
- Real-time network monitoring
- DOM inspection and manipulation
- Performance profiling (CPU and heap)
- Target management and creation

## Installation

### Build All Tools
```bash
# Build all debugging tools
go build ./cmd/cdp
go build ./cmd/ndp
go build ./cmd/chdb

# Install globally
go install ./cmd/cdp
go install ./cmd/ndp
go install ./cmd/chdb
```

### Individual Installation
```bash
# Install specific tools
go install github.com/tmc/misc/chrome-to-har/cmd/chdb@latest
go install github.com/tmc/misc/chrome-to-har/cmd/ndp@latest
```

## Quick Start Guide

### 1. Chrome Debugging (CHDB)

Start Chrome with debugging:
```bash
google-chrome --remote-debugging-port=9222
```

Basic CHDB usage:
```bash
# List Chrome targets
chdb list

# Execute JavaScript
chdb exec "document.title"

# Open DevTools interface
chdb devtools

# Interactive console
chdb console

# Monitor network activity
chdb monitor --duration 30s
```

### 2. Node.js Debugging (NDP)

Start Node.js with debugging:
```bash
node --inspect=9229 app.js
```

Basic NDP usage:
```bash
# List debuggable processes
ndp node list

# Attach to running Node.js
ndp node attach 9229

# Interactive REPL
ndp repl

# Set breakpoints
ndp node break app.js:42 --condition "user.id === 123"
```

### 3. Enhanced CDP

```bash
# Enhanced interactive mode
cdp --enhanced

# Execute with improved commands
cdp --command "goto https://example.com"

# List browsers with auto-discovery
cdp --list-browsers
```

## Workflow Examples

### Full-Stack Debugging

Debug both Node.js backend and Chrome frontend:

```bash
# Terminal 1: Start Node.js with debugging
node --inspect=9229 server.js

# Terminal 2: Start Chrome with debugging
google-chrome --remote-debugging-port=9222

# Terminal 3: Debug Node.js
ndp node attach 9229

# Terminal 4: Debug Chrome
chdb attach 9222

# Terminal 5: Unified debugging
ndp repl  # Switch between Node and Chrome targets
```

### Web Performance Analysis

```bash
# 1. Open target page
chdb navigate https://example.com

# 2. Start monitoring
chdb monitor --duration 60s &

# 3. Take baseline screenshot
chdb screenshot baseline.png

# 4. Profile performance
chdb profile cpu --duration 30s --output perf.json

# 5. Analyze heap usage
chdb profile heap --output heap.json

# 6. Inspect critical elements
chdb inspect ".performance-critical"
```

### Automated Testing Integration

```bash
#!/bin/bash
# test-automation.sh

# Start services
node --inspect=9229 test-server.js &
SERVER_PID=$!

google-chrome --headless --remote-debugging-port=9222 &
CHROME_PID=$!

# Wait for services
sleep 2

# Run tests with debugging
chdb navigate http://localhost:3000
chdb exec "window.runTests()" --tab $(chdb list | grep localhost | cut -d' ' -f1)

# Capture results
chdb screenshot test-result.png
chdb exec "JSON.stringify(window.testResults)" > test-results.json

# Monitor network during tests
chdb monitor --duration 30s > network-log.txt

# Cleanup
kill $SERVER_PID $CHROME_PID
```

## Architecture and Design

### Shared Internal Packages

The tools share common functionality through internal packages:

```
internal/
├── cdp/              # Shared CDP session management
│   └── session.go    # Session creation and management
└── debugger/         # Shared debugging utilities
    └── breakpoints.go # Breakpoint management
```

### Tool-Specific Implementations

Each tool builds on the shared foundation:

- **CDP**: Enhanced with better command parsing and aliases
- **NDP**: Adds Node.js V8 Inspector Protocol support
- **CHDB**: Provides comprehensive Chrome DevTools access

## Integration Examples

### With Build Tools

```bash
# Webpack integration
npm run build && chdb navigate http://localhost:8080 && chdb screenshot build.png

# Vite integration
npm run dev &
sleep 3
chdb navigate http://localhost:5173
chdb console  # Interactive debugging
```

### With Testing Frameworks

```bash
# Jest with Node debugging
ndp node start --inspect-brk test-runner.js
ndp node break test.spec.js:25

# Playwright with Chrome debugging
chdb attach 9222
chdb monitor --duration 60s  # Monitor during test run
```

### With CI/CD

```yaml
# GitHub Actions example
- name: Debug Test Failures
  run: |
    google-chrome --headless --remote-debugging-port=9222 &
    npm test
    chdb screenshot failure-state.png
    chdb exec "console.log(window.testState)" > debug-state.json
```

## Configuration

### Environment Variables

```bash
# Default ports
export CDP_PORT=9222
export NDP_NODE_PORT=9229
export CHDB_PORT=9222

# Debugging preferences
export DEBUG_VERBOSE=true
export DEBUG_TIMEOUT=60
```

### Configuration Files

Create `.chdbrc` for persistent settings:
```json
{
  "defaultPort": 9222,
  "verbose": true,
  "timeout": 60,
  "autoOpenDevTools": false
}
```

## Advanced Features

### Session Persistence (NDP)

```bash
# Save debugging session
ndp session save production-debug

# Load saved session
ndp session load production-debug

# List saved sessions
ndp session list
```

### DevTools-in-DevTools (CHDB)

```bash
# Open DevTools for DevTools
chdb new "chrome://inspect"
chdb devtools --tab <devtools-tab-id>
```

### Network Interception

```bash
# Monitor and modify network requests
chdb monitor &
chdb exec "
fetch('/api/data')
  .then(r => r.json())
  .then(data => console.log('API Response:', data))
"
```

## Troubleshooting

### Common Issues

1. **Port conflicts**: Use `--port` flag to specify different ports
2. **Permission issues**: Ensure Chrome has proper permissions
3. **Target not found**: Use `list` commands to verify available targets

### Debug Mode

Enable verbose output for all tools:
```bash
chdb --verbose list
ndp --verbose repl
cdp --verbose --command "help"
```

## Contributing

The debugging tools are designed to be extensible. To add new features:

1. Add shared functionality to `internal/` packages
2. Implement tool-specific features in `cmd/` directories
3. Update documentation and examples

## Future Enhancements

Planned features include:
- WebDriver integration
- Mobile debugging support
- Multi-browser session management
- Advanced profiling visualizations
- IDE integrations