# Native Messaging Host Roadmap

## Overview
Native messaging host for Chrome extensions, enabling AI proxy functionality and enhanced browser capabilities.

## Current State (v1.0)

✅ **Core Features**: Chrome Native Messaging Protocol, message read/write with retry logic, exponential backoff with jitter, ping/pong health checking, status reporting, AI request handling (simulated), structured logging, retry statistics

## Future Development

Features should be tracked as beads when they become priorities. Key areas for potential development:

### Core Improvements
- Message validation (JSON schema, size limits, rate limiting)
- Configuration (file support, env vars, runtime reload)
- Logging & monitoring (structured logging, levels, rotation, metrics)

### Security
- Extension verification (ID validation, manifest check, permissions)
- Resource limits (memory, CPU throttling, rate limiting)
- Sandboxing (process isolation, capability-based security)

### AI Proxy Features
- AI service integration (OpenAI, Anthropic, Gemini, local models)
- Request handling (streaming, cancellation, queuing, context mgmt)
- Caching & optimization (response cache, token tracking, cost optimization)

### Chrome Automation
- Browser control (launch instances, tab mgmt, JS execution, screenshots)
- Data access (storage, cookies, IndexedDB, history)

### Session Management
- Multi-session support (identification, isolation, concurrent sessions)
- State management (data storage, sync, cleanup)

### Integration
- Protocol support (WebSocket, gRPC, GraphQL)
- Tool integration (cdp, chdb, churl, Puppeteer/Playwright bridge)

### Observability
- Monitoring (health checks, metrics, error tracking, distributed tracing)
- Debugging (interactive debugger, message replay, profiling)

## Message Types

**Current**: `ping` → `pong`, `ai_request` → `ai_response`, `status` → `status_response`

**Planned**: chrome_launch/navigate/execute/screenshot, storage/cookie ops, file ops, config mgmt, session mgmt, streaming support

## Use Cases
1. AI assistant extensions (proxy AI API calls)
2. Browser automation (control Chrome from extensions)
3. Local data access (files and databases)
4. System integration (browser with desktop apps)

## Related Projects
- **Chrome Native Messaging**: Official protocol
- **cdp/chdb/churl**: Related browser tools

---

**Note**: Track specific features as beads when they become active priorities. This roadmap is intentionally high-level.

**Last Updated**: 2025-01-17
**Status**: Early Stage - Enhancement on demand
