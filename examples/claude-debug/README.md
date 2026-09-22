# Claude Code Debugging Tools

This directory contains debugging utilities for diagnosing stuck Claude Code sessions. They are not part of the `cdp` commands.

## Tools

- `nodejs-claude-debugger.js`: Node.js process diagnostics and reports
- `claude-debug-attach.sh`: attach, trace, and monitor helper
- `debugging.md`: debugging guide

## Quick Usage

```bash
# List all Claude processes
./claude-debug-attach.sh list

# Auto-detect and analyze stuck processes
./claude-debug-attach.sh auto

# Generate a report for PID
node nodejs-claude-debugger.js report <PID>

# Attach debugger to process
./claude-debug-attach.sh attach <PID>
```

See `debugging.md` for detailed documentation.