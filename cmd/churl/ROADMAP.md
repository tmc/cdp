# Churl - Chrome-Powered curl Roadmap

## Overview
Churl is a curl-like tool using Chrome/Chromium for JavaScript-enabled web fetching with a familiar curl interface.

## Current State (v1.0)

✅ **Core Features**: HTTP methods, custom headers, POST data, multiple output formats (HTML/HAR/Text/JSON), Chrome profile support with cookie filtering, headless/headful modes, proxy support (HTTP/HTTPS/SOCKS5), JS execution, CSS selector extraction, URL/domain blocking, WebSocket monitoring, wait strategies, remote Chrome, browser auto-discovery, basic auth, **--har flag** for separate HAR output

## Active Development (Tracked in Beads)

### High Priority - Website Mirroring
**wget-compatible SPA-aware mirroring** - See `MIRROR_DESIGN.md` for full spec

Beads exist for:
- wget-compatible flags (`-m, -r, -l, -k, -p, -np`)
- Basic mirroring infrastructure
- SPA framework detection (React, Vue, Angular)
- Link discovery system
- Advanced SPA mirroring features

### Authentication & Content
- **Authentication-aware downloader** (bead: chrome-to-har-55)
- **Content extraction pipeline** (bead exists)
- Browser session-based downloads with auto cookie/auth injection
- OAuth/SSO support, multi-step authentication

### Testing & Quality
- **Test assertions & validation** (bead exists)
- **Performance & metrics** (bead exists)
- **Security testing features** (bead exists)

### User Experience
- **Browser state management** (bead exists)
- **Page interaction features** (bead exists) - form filling, clicking, hovering
- **Enhanced output formatting** (bead exists)
- **Recording & replay** (bead exists)

### Integration
- **CI/CD integration** (bead exists) - JUnit XML, TAP format, exit codes

## curl Compatibility Goals

**Target**: 90%+ curl flag compatibility

Planned flags (see beads for details):
- `-A/--user-agent`, `-e/--referer`, `-b/--cookie`, `-c/--cookie-jar`
- `-I/--head`, `-i/--include`, `-s/--silent`, `-S/--show-error`
- `--retry`, `--retry-delay`, `--max-time`, `--connect-timeout`
- `-C/--continue-at`, `--limit-rate`, `--compressed`

Chrome-specific additions:
- `--viewport`, `--device`, `--screenshot`, `--pdf`
- `--lighthouse`, `--coverage`, `--disable-images/css/fonts`

## Future Considerations

**Advanced Web**: AJAX completion detection, custom stability conditions, service worker handling

**Network & Security**: Request/response modification, certificate validation, CSP reporting, CORS detection

**AI Features**: Natural language to selectors, auto wait conditions, content summarization

**Monitoring**: Performance thresholds, content change detection, availability monitoring

## Use Cases

**Scraping**: `churl --extract-selector "article" URL`
**Testing**: `churl -X POST -d "data" URL --wait-for "#done"`
**Monitoring**: `churl --output-format har URL | jq '.log.pages[0].pageTimings'`
**Debugging**: `churl --har traffic.har --profile Default URL`

## Related Tools
- **curl**: Original inspiration
- **cdp**: Chrome DevTools Protocol CLI
- **chrome-to-har**: HAR capture tool

---

**Note**: Most roadmap items are tracked as beads. Run `bd list | grep -i churl` to see all work items.

**Last Updated**: 2025-01-17
**Status**: Active Development
