# Claude Code Debugging Tools

This directory contains debugging utilities for diagnosing stuck Claude Code sessions. These tools are separate from the core chrome-to-har codebase.

## Tools

- **`nodejs-claude-debugger.js`** - Comprehensive Node.js debugging tool
- **`claude-debug-attach.sh`** - Quick debug attachment script
- **`CLAUDE_DEBUG_GUIDE.md`** - Complete debugging guide

## Quick Usage

```bash
# List all Claude processes
./claude-debug-attach.sh list

# Auto-detect and analyze stuck processes
./claude-debug-attach.sh auto

# Generate comprehensive report for PID
node nodejs-claude-debugger.js report <PID>

# Attach debugger to process
./claude-debug-attach.sh attach <PID>
```

See `CLAUDE_DEBUG_GUIDE.md` for detailed documentation.

## Note

These are debugging utilities only - they do not affect the core chrome-to-har functionality.