# Claude Code Node.js Debugging Guide

## Overview
This guide provides comprehensive techniques for diagnosing and debugging stuck Claude Code sessions using Node.js debugging capabilities.

## Quick Diagnosis

### 1. List Claude Processes
```bash
# Find all Claude processes
ps aux | grep -i claude | grep -v grep

# Using our debug script
./claude-debug-attach.sh list
```

### 2. Identify Stuck Process
Signs of a stuck process:
- CPU usage < 1% when should be active
- Shows "esc to interrupt" but unresponsive
- No response to keyboard input

### 3. Quick Analysis
```bash
# Automatic detection and analysis
./claude-debug-attach.sh auto

# Analyze specific PID
./claude-debug-attach.sh analyze <PID>
```

## Debugging Techniques

### Technique 1: Node.js Inspector

Enable the built-in Node.js debugger:

```bash
# Send SIGUSR1 to enable inspector
kill -USR1 <PID>

# Or use our tool
./claude-debug-attach.sh attach <PID>
```

Connect to debugger:
1. **Chrome DevTools**: Navigate to `chrome://inspect`
2. **Command Line**: `node inspect localhost:9229`
3. **VS Code**: Use "Attach to Node Process" configuration

### Technique 2: Stack Trace Analysis

Get current stack trace without attaching debugger:

```bash
# macOS: Use sample command
sample <PID> 1 -f

# Using our script
./claude-debug-attach.sh trace <PID>

# Alternative: Use lldb
echo -e "attach <PID>\nbt all\ndetach\nquit" | lldb
```

### Technique 3: Process Monitoring

Monitor CPU and memory usage over time:

```bash
# Monitor for 10 seconds
./claude-debug-attach.sh monitor <PID> 10

# Manual monitoring
while true; do
  ps -p <PID> -o pcpu,pmem,state
  sleep 1
done
```

### Technique 4: File Descriptor Analysis

Check what files and network connections are open:

```bash
# List all file descriptors
lsof -p <PID>

# Network connections only
lsof -p <PID> -i

# Check for pipe/socket blocks
lsof -p <PID> | grep -E "PIPE|SOCK"
```

### Technique 5: Code Injection

Inject diagnostic code into running process:

```bash
# Prepare injection code
./claude-debug-attach.sh inject <PID>

# Then in debugger console:
require('/tmp/claude-inject.js')
```

## Using the Node.js Debugger Tool

### Installation
```bash
# No installation needed - uses built-in Node.js modules
node nodejs-claude-debugger.js --help
```

### Commands

#### List all processes
```bash
node nodejs-claude-debugger.js list
```

#### Diagnose stuck process
```bash
node nodejs-claude-debugger.js diagnose <PID>
```

#### Generate comprehensive report
```bash
node nodejs-claude-debugger.js report <PID> --output report.txt
```

#### Monitor process
```bash
node nodejs-claude-debugger.js monitor <PID> --duration 15000
```

## Common Issues and Solutions

### Issue: Process shows 0% CPU
**Possible Causes:**
- Deadlock in synchronous code
- Blocked on I/O operation
- Waiting for user input that isn't arriving

**Diagnosis:**
```bash
# Check stack trace for blocking calls
./claude-debug-attach.sh trace <PID> | grep -E "poll|select|kevent|read|write"

# Check for mutex/lock issues
./claude-debug-attach.sh trace <PID> | grep -E "mutex|lock|semaphore"
```

### Issue: High Memory Usage
**Diagnosis:**
```bash
# Check memory allocation
node nodejs-claude-debugger.js diagnose <PID> | grep -A5 "memoryUsage"

# Monitor memory over time
while true; do ps -p <PID> -o rss,vsz; sleep 2; done
```

### Issue: Unresponsive to Input
**Possible Causes:**
- Event loop blocked
- Input stream disconnected
- Terminal settings corrupted

**Diagnosis:**
```bash
# Check if stdin is connected
lsof -p <PID> | grep "0u"

# Check terminal settings
stty -a

# Reset terminal if needed
reset
```

## Advanced Debugging

### Using Chrome DevTools

1. Enable inspector:
```bash
kill -USR1 <PID>
```

2. Open Chrome and navigate to `chrome://inspect`

3. Click "inspect" on the target process

4. Use DevTools features:
   - **Profiler**: Record CPU profile to find bottlenecks
   - **Memory**: Take heap snapshots to find leaks
   - **Console**: Execute code in process context
   - **Sources**: Set breakpoints and step through code

### Profiling Performance

```javascript
// In debugger console
const v8Profiler = require('v8-profiler-next');

// Start CPU profiling
v8Profiler.startProfiling('MyProfile');

// Stop after 10 seconds
setTimeout(() => {
  const profile = v8Profiler.stopProfiling('MyProfile');
  profile.export((err, result) => {
    require('fs').writeFileSync('profile.cpuprofile', result);
    console.log('Profile saved to profile.cpuprofile');
    profile.delete();
  });
}, 10000);
```

### Memory Analysis

```javascript
// In debugger console
const v8 = require('v8');
const fs = require('fs');

// Generate heap snapshot
const heapSnapshot = v8.writeHeapSnapshot();
console.log('Heap snapshot written:', heapSnapshot);

// Get heap statistics
console.log('Heap Stats:', v8.getHeapStatistics());
```

## Emergency Recovery

If Claude Code is completely stuck:

### 1. Soft Recovery
```bash
# Try to interrupt with Ctrl+C
# Then ESC key
# Then Ctrl+D
```

### 2. Debug and Diagnose
```bash
# Get PID
ps aux | grep claude | head -1

# Full diagnosis
./claude-debug-attach.sh analyze <PID>
node nodejs-claude-debugger.js report <PID> --output stuck-report.txt
```

### 3. Hard Recovery
```bash
# Force quit (last resort)
kill -TERM <PID>

# If still stuck
kill -KILL <PID>
```

## Preventive Measures

### Enable Debug Logging
```bash
# Set environment variables before starting Claude
export NODE_OPTIONS="--trace-warnings --trace-uncaught"
export DEBUG="*"
claude
```

### Start with Inspector Enabled
```bash
# Start Claude with inspector from beginning
NODE_OPTIONS="--inspect" claude
```

### Monitor from Start
```bash
# Run monitoring in background
./claude-debug-attach.sh monitor <PID> 3600 &
```

## Report Format

When reporting stuck session issues, include:

1. **Process Info**
```bash
ps -p <PID> -o pid,ppid,user,state,pcpu,pmem,vsz,rss,etime,command
```

2. **Stack Trace**
```bash
./claude-debug-attach.sh trace <PID> > stacktrace.txt
```

3. **File Descriptors**
```bash
lsof -p <PID> > file-descriptors.txt
```

4. **Diagnostic Report**
```bash
node nodejs-claude-debugger.js report <PID> --output full-report.txt
```

## Tools Summary

| Tool | Purpose | Usage |
|------|---------|-------|
| `claude-debug-attach.sh` | Quick debugging | `./claude-debug-attach.sh analyze <PID>` |
| `nodejs-claude-debugger.js` | Comprehensive analysis | `node nodejs-claude-debugger.js report <PID>` |
| Chrome DevTools | Visual debugging | `chrome://inspect` |
| `kill -USR1` | Enable inspector | `kill -USR1 <PID>` |
| `sample` | Stack sampling (macOS) | `sample <PID> 1` |
| `lsof` | File descriptor analysis | `lsof -p <PID>` |

## Best Practices

1. **Always capture diagnostic data before killing process**
2. **Use soft interrupts before hard kills**
3. **Monitor CPU/memory patterns to predict issues**
4. **Enable debug logging for critical sessions**
5. **Keep diagnostic reports for pattern analysis**

## References

- [Node.js Debugging Guide](https://nodejs.org/en/docs/guides/debugging-getting-started/)
- [Chrome DevTools Protocol](https://chromedevtools.github.io/devtools-protocol/)
- [V8 Inspector API](https://nodejs.org/api/inspector.html)
- [Node.js Diagnostics](https://nodejs.org/en/docs/guides/diagnostics/)