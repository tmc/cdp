# Chrome DevTools Node.js Debugging Capabilities Reference

## Overview

Chrome DevTools provides comprehensive debugging capabilities for Node.js processes through the Chrome DevTools Protocol (CDP). This reference documents all available features, domains, methods, and integration patterns for Node.js debugging as of 2025.

## Table of Contents

1. [Core Debugging Features](#core-debugging-features)
2. [Advanced Features](#advanced-features)
3. [V8 Inspector Protocol Domains](#v8-inspector-protocol-domains)
4. [Session Management](#session-management)
5. [Integration Features](#integration-features)
6. [Implementation Examples](#implementation-examples)

## Core Debugging Features

### 1. Breakpoint Management

#### Line Breakpoints
- **Set by line number**: `Debugger.setBreakpointByUrl`
- **Conditional breakpoints**: Support for JavaScript expressions as conditions
- **Hit count tracking**: Automatic tracking of breakpoint hits
- **Enable/disable**: Toggle breakpoints without removing them
- **Temporary breakpoints**: One-time breakpoints that auto-remove

#### Function Breakpoints
- **Function entry**: Break when entering specific functions
- **Method chaining**: Support for object method breakpoints
- **Constructor breakpoints**: Break on object instantiation
- **Async function support**: Handle async/await and Promise breakpoints

#### Event Breakpoints
- **DOM events**: Break on specific DOM events
- **Exception breakpoints**: Break on caught/uncaught exceptions
- **Custom events**: Application-specific event breakpoints

#### Logpoints (Non-breaking Breakpoints)
- **Console logging**: Output values without stopping execution
- **Expression evaluation**: Log complex expressions at specific points
- **Performance monitoring**: Track values over time without performance impact

### 2. Step-through Execution

#### Basic Stepping
- **Step Into** (`Debugger.stepInto`): Enter function calls
- **Step Over** (`Debugger.stepOver`): Execute current line, skip function internals
- **Step Out** (`Debugger.stepOut`): Complete current function and return to caller
- **Continue** (`Debugger.resume`): Resume normal execution

#### Advanced Stepping
- **Step into async**: Follow execution through async/await calls
- **Skip file patterns**: Avoid stepping into library code
- **Custom step filters**: User-defined stepping behavior

### 3. Call Stack Inspection

#### Stack Frame Analysis
- **Frame navigation**: Move up/down the call stack
- **Local variables**: Inspect variables in each frame
- **Closure inspection**: Access closure variables and scope chain
- **this context**: Examine 'this' binding at each level

#### Stack Trace Features
- **Async stack traces**: Full async call chains
- **Source map support**: Map compiled code back to source
- **Error stack enhancement**: Detailed error location information

### 4. Variable Inspection and Scope Analysis

#### Scope Hierarchy
- **Local scope**: Function-level variables
- **Closure scope**: Captured variables from outer functions
- **Global scope**: Global object properties
- **Module scope**: ES6 module-level variables

#### Variable Features
- **Live editing**: Modify variable values during debugging
- **Type inspection**: Detailed type information
- **Property enumeration**: Object property listing
- **Prototype chain**: Full prototype inspection

### 5. Watch Expressions

#### Expression Monitoring
- **Real-time evaluation**: Continuous expression monitoring
- **Complex expressions**: Support for any JavaScript expression
- **Conditional watches**: Only evaluate under certain conditions
- **Performance tracking**: Monitor expression evaluation time

### 6. Console Evaluation

#### Interactive Console
- **Context-aware evaluation**: Execute in current call frame context
- **Multi-line expressions**: Support for complex code blocks
- **History management**: Command history and recall
- **Auto-completion**: Intelligent code completion

## Advanced Features

### 1. Profiling Capabilities

#### CPU Profiling
- **Call tree analysis**: Hierarchical function call analysis
- **Sampling profiler**: Statistical performance sampling
- **Flame graphs**: Visual performance representation
- **Hot path identification**: Find performance bottlenecks

```javascript
// CDP Methods for CPU Profiling
Profiler.enable()
Profiler.start()
Profiler.stop() // Returns profile data
Profiler.setSamplingInterval(interval)
```

#### Memory Profiling
- **Heap snapshots**: Complete memory state capture
- **Memory allocation tracking**: Track object creation/destruction
- **Memory leak detection**: Identify unreferenced objects
- **Garbage collection monitoring**: GC event tracking

```javascript
// CDP Methods for Memory Profiling
HeapProfiler.enable()
HeapProfiler.takeHeapSnapshot()
HeapProfiler.startSampling()
HeapProfiler.stopSampling()
```

### 2. Coverage Analysis

#### Code Coverage
- **Function-level coverage**: Track function execution
- **Statement coverage**: Line-by-line execution tracking
- **Branch coverage**: Conditional path analysis
- **Real-time updates**: Live coverage during execution

```javascript
// CDP Methods for Coverage
Profiler.startPreciseCoverage()
Profiler.takePreciseCoverage()
Profiler.getBestEffortCoverage()
Profiler.stopPreciseCoverage()
```

### 3. Live Editing/Hot Reload

#### Source Modification
- **Runtime code changes**: Modify functions during execution
- **Variable updates**: Change variable values in real-time
- **Breakpoint injection**: Add breakpoints to running code
- **Module reloading**: Hot-swap entire modules

### 4. Network Monitoring

#### Request Tracking
- **HTTP/HTTPS requests**: Monitor all network traffic
- **WebSocket connections**: Real-time WebSocket monitoring
- **Request/response headers**: Full header inspection
- **Timing analysis**: Request timing breakdown

### 5. Timeline/Performance Analysis

#### Execution Timeline
- **Event loop monitoring**: Track event loop performance
- **Async operation tracking**: Monitor Promise/async execution
- **Frame timing**: Track rendering performance (when applicable)
- **Custom markers**: User-defined performance markers

## V8 Inspector Protocol Domains

### 1. Debugger Domain

The core debugging domain providing breakpoint and execution control.

#### Key Methods
```javascript
// Enable/disable debugging
Debugger.enable()
Debugger.disable()

// Execution control
Debugger.pause()
Debugger.resume()
Debugger.stepInto()
Debugger.stepOver()
Debugger.stepOut()

// Breakpoint management
Debugger.setBreakpoint(location)
Debugger.setBreakpointByUrl(lineNumber, url, condition)
Debugger.removeBreakpoint(breakpointId)
Debugger.setBreakpointsActive(active)

// Advanced debugging
Debugger.evaluateOnCallFrame(callFrameId, expression)
Debugger.getScriptSource(scriptId)
Debugger.setPauseOnExceptions(state)
Debugger.restartFrame(callFrameId)
```

#### Key Events
```javascript
// Execution events
Debugger.paused // Fired when execution pauses
Debugger.resumed // Fired when execution resumes

// Script events
Debugger.scriptParsed // New script loaded
Debugger.scriptFailedToParse // Script parsing failed

// Breakpoint events
Debugger.breakpointResolved // Breakpoint successfully set
```

### 2. Runtime Domain

Manages JavaScript execution context and object inspection.

#### Key Methods
```javascript
// JavaScript evaluation
Runtime.evaluate(expression, contextId, returnByValue)
Runtime.callFunctionOn(objectId, functionDeclaration, arguments)

// Execution context
Runtime.enable()
Runtime.disable()
Runtime.setAsyncCallStackDepth(maxDepth)

// Object management
Runtime.getProperties(objectId, ownProperties, accessorPropertiesOnly)
Runtime.releaseObject(objectId)
Runtime.releaseObjectGroup(objectGroupName)

// Memory and heap
Runtime.getHeapUsage()
Runtime.queryObjects(prototypeObjectId)

// Compilation and execution
Runtime.compileScript(expression, sourceURL, persistScript)
Runtime.runScript(scriptId, contextId)
```

#### Key Events
```javascript
// Console events
Runtime.consoleAPICalled // console.log, console.error, etc.
Runtime.exceptionThrown // Unhandled exceptions

// Context events
Runtime.executionContextCreated // New execution context
Runtime.executionContextDestroyed // Context cleanup
Runtime.executionContextsCleared // All contexts cleared

// Inspection events
Runtime.inspectRequested // Object inspection requested
```

### 3. Profiler Domain

Provides CPU profiling and code coverage capabilities.

#### Key Methods
```javascript
// CPU profiling
Profiler.enable()
Profiler.disable()
Profiler.start()
Profiler.stop()
Profiler.setSamplingInterval(interval)

// Code coverage
Profiler.startPreciseCoverage(callCount, detailed)
Profiler.stopPreciseCoverage()
Profiler.takePreciseCoverage()
Profiler.getBestEffortCoverage()
```

#### Key Events
```javascript
// Console profiling events
Profiler.consoleProfileStarted
Profiler.consoleProfileFinished

// Coverage events
Profiler.preciseCoverageDeltaUpdate // Live coverage updates
```

### 4. HeapProfiler Domain

Specialized memory profiling and heap analysis.

#### Key Methods
```javascript
// Heap profiling
HeapProfiler.enable()
HeapProfiler.disable()
HeapProfiler.takeHeapSnapshot()

// Sampling heap profiler
HeapProfiler.startSampling(samplingInterval)
HeapProfiler.stopSampling()
HeapProfiler.getSamplingProfile()

// Object tracking
HeapProfiler.startTrackingHeapObjects(trackAllocations)
HeapProfiler.stopTrackingHeapObjects(reportProgress)
HeapProfiler.getObjectByHeapObjectId(objectId)
HeapProfiler.getHeapObjectId(objectId)
```

#### Key Events
```javascript
// Heap snapshot events
HeapProfiler.addHeapSnapshotChunk // Heap snapshot data chunks
HeapProfiler.reportHeapSnapshotProgress // Progress updates

// Object tracking events
HeapProfiler.heapStatsUpdate // Heap statistics updates
HeapProfiler.lastSeenObjectId // Object ID tracking
```

### 5. Console Domain

Manages console interaction and logging.

#### Key Methods
```javascript
// Console control
Console.enable()
Console.disable()
Console.clearMessages()
```

#### Key Events
```javascript
// Message events
Console.messageAdded // New console message
Console.messageRepeatCountUpdated // Message repetition
```

### 6. Network Domain

Monitors network requests and responses (limited in Node.js context).

#### Key Methods
```javascript
// Network monitoring
Network.enable()
Network.disable()
Network.setUserAgentOverride(userAgent)
```

#### Key Events
```javascript
// Request/response events
Network.requestWillBeSent // Outgoing requests
Network.responseReceived // Incoming responses
Network.loadingFinished // Request completion
Network.loadingFailed // Request failures
```

## Session Management

### 1. Multiple Process Handling

#### Process Discovery
- **Automatic detection**: Discover running Node.js processes with --inspect
- **Port scanning**: Find processes on different debug ports
- **Process metadata**: Get process information (PID, version, command line)

#### Session Creation
- **WebSocket connections**: Establish debugging connections
- **Target management**: Handle multiple debug targets
- **Session isolation**: Separate debugging sessions per process

### 2. Auto-reconnection on Restarts

#### Connection Management
- **Heartbeat monitoring**: Detect connection loss
- **Automatic reconnection**: Reconnect when process restarts
- **State preservation**: Maintain breakpoints across reconnections
- **Graceful degradation**: Handle partial feature availability

### 3. State Persistence

#### Breakpoint Persistence
- **Save/restore breakpoints**: Persist breakpoints to file
- **Session state**: Maintain debugging state across connections
- **Workspace integration**: IDE/editor integration support

## Integration Features

### 1. File Mapping and Source Maps

#### Source Map Support
- **Automatic detection**: Find and load source maps
- **Multi-level mapping**: Handle transpiled code chains (TypeScript → ES6 → ES5)
- **Dynamic mapping**: Handle runtime-generated source maps
- **Source map validation**: Verify mapping accuracy

#### File Resolution
- **Module resolution**: Map Node.js modules to source files
- **Path normalization**: Handle different path formats
- **Virtual file support**: Support for in-memory modules

### 2. Module Resolution

#### Node.js Module System
- **CommonJS support**: Handle require() calls
- **ES6 module support**: Import/export resolution
- **Dynamic imports**: Track dynamic import() calls
- **Module caching**: Understanding module cache behavior

### 3. Error Handling and Exception Management

#### Exception Processing
- **Stack trace enhancement**: Add source map information
- **Async error tracking**: Follow async error propagation
- **Unhandled rejection monitoring**: Track Promise rejections
- **Custom error handling**: Application-specific error processing

## Implementation Examples

### 1. Basic Debugging Session Setup

```go
// Basic Chrome debugger setup (from existing codebase)
func (cd *ChromeDebugger) Attach(ctx context.Context, port string) error {
    // Verify Chrome is available
    url := fmt.Sprintf("http://localhost:%s/json/version", port)
    resp, err := http.Get(url)
    if err != nil {
        return errors.Wrapf(err, "cannot connect to Chrome on port %s", port)
    }
    defer resp.Body.Close()

    // Enable necessary domains
    return cd.enableDomains(ctx)
}

func (cd *ChromeDebugger) enableDomains(ctx context.Context) error {
    return chromedp.Run(cd.session.ChromeCtx,
        chromedp.ActionFunc(func(ctx context.Context) error {
            // Enable Page, Runtime, Network, DOM, and Debugger domains
            if err := page.Enable().Do(ctx); err != nil {
                return err
            }
            if err := runtime.Enable().Do(ctx); err != nil {
                return err
            }
            if err := network.Enable().Do(ctx); err != nil {
                return err
            }
            if err := dom.Enable().Do(ctx); err != nil {
                return err
            }
            _, err := debugger.Enable().Do(ctx)
            return err
        }),
    )
}
```

### 2. Breakpoint Management

```go
// Breakpoint management (from existing codebase)
func (bm *BreakpointManager) SetBreakpoint(ctx context.Context, location string, condition string) error {
    bp, err := bm.parseLocation(location)
    if err != nil {
        return errors.Wrap(err, "invalid breakpoint location")
    }

    bp.Condition = condition
    bp.Enabled = true

    // Set breakpoint based on type
    switch bp.Type {
    case BreakpointTypeLine:
        err = bm.setLineBreakpoint(ctx, bp)
    case BreakpointTypeFunction:
        err = bm.setFunctionBreakpoint(ctx, bp)
    }

    if err != nil {
        return err
    }

    // Store breakpoint
    bm.mu.Lock()
    bm.breakpoints[bp.ID] = bp
    bm.mu.Unlock()

    return nil
}
```

### 3. CPU Profiling

```go
// CPU profiling implementation (from existing codebase)
func (p *Profiler) ProfileCPU(ctx context.Context, duration string, outputFile string, targetID string) error {
    dur, err := time.ParseDuration(duration)
    if err != nil {
        return errors.Wrap(err, "invalid duration format")
    }

    // Start profiling
    if err := p.startCPUProfiling(ctx); err != nil {
        return errors.Wrap(err, "failed to start CPU profiling")
    }

    // Wait for duration
    select {
    case <-time.After(dur):
    case <-ctx.Done():
        return ctx.Err()
    }

    // Stop profiling and get results
    profile, err := p.stopCPUProfiling(ctx)
    if err != nil {
        return errors.Wrap(err, "failed to stop CPU profiling")
    }

    // Save profile
    if outputFile != "" {
        if err := p.saveCPUProfile(outputFile); err != nil {
            return errors.Wrap(err, "failed to save CPU profile")
        }
    }

    return nil
}
```

### 4. Event Monitoring

```go
// Event monitoring setup (from existing codebase)
func (cd *ChromeDebugger) startEventMonitoring() {
    chromedp.ListenTarget(cd.session.ChromeCtx, func(ev interface{}) {
        switch ev := ev.(type) {
        case *runtime.EventConsoleAPICalled:
            cd.handleConsoleMessage(ev)
        case *runtime.EventExceptionThrown:
            cd.handleException(ev)
        case *page.EventJavascriptDialogOpening:
            cd.handleDialog(ev)
        case *network.EventRequestWillBeSent:
            cd.handleNetworkRequest(ev)
        case *debugger.EventPaused:
            cd.handleDebuggerPaused(ev)
        }
    })
}
```

## Connection Methods

### 1. Node.js Inspector
```bash
# Start Node.js with inspector
node --inspect app.js                    # Default port 9229
node --inspect=9230 app.js              # Custom port
node --inspect-brk app.js               # Break on start
node --inspect=0.0.0.0:9229 app.js      # Bind to all interfaces
```

### 2. Chrome DevTools UI
```
chrome://inspect/
```

### 3. Programmatic Connection
```javascript
// Using WebSocket connection
const WebSocket = require('ws');
const ws = new WebSocket('ws://localhost:9229/...');

// Using chrome-remote-interface
const CDP = require('chrome-remote-interface');
const client = await CDP({port: 9229});
```

## Best Practices

### 1. Performance Considerations
- **Selective domain enabling**: Only enable needed CDP domains
- **Breakpoint optimization**: Use conditional breakpoints to reduce overhead
- **Profile duration management**: Limit profiling duration to avoid memory issues
- **Event filtering**: Filter events to reduce processing overhead

### 2. Security Considerations
- **Local-only binding**: Bind inspector to localhost only in production
- **Authentication**: Use authentication for remote debugging
- **Network isolation**: Isolate debugging traffic from application traffic

### 3. Development Workflow
- **Source map generation**: Always generate source maps for production debugging
- **Symbol preservation**: Keep function names in production for better debugging
- **Error handling**: Implement comprehensive error handling for debugging tools
- **Documentation**: Document debugging setup and common procedures

## Limitations and Considerations

### 1. Node.js Specific Limitations
- **Single-threaded debugging**: Node.js main thread debugging only
- **Worker thread limitations**: Limited worker thread debugging support
- **Native module debugging**: C++ addons require separate debugging tools
- **Cluster debugging**: Each cluster worker needs separate debugging session

### 2. Performance Impact
- **Debugging overhead**: Debugging adds execution overhead
- **Memory usage**: Profiling can increase memory usage significantly
- **Network overhead**: Remote debugging adds network latency

### 3. Compatibility
- **Node.js version requirements**: Inspector available in Node.js 6.3+
- **Protocol version compatibility**: Ensure CDP version compatibility
- **Source map support**: Requires proper source map generation
- **Operating system differences**: Some features may vary by OS

This comprehensive reference provides the foundation for implementing robust Node.js debugging tools using the Chrome DevTools Protocol. The existing codebase already implements many of these features, providing a solid foundation for further development.