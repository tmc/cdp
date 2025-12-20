# Chrome-to-HAR Implementation TODO

## Overview
This document tracks the implementation status of new features across the chrome-to-har project suite.

**Last Updated**: 2025-01-10

---

## Priority 1: CDP Script Format (Q1 2025)

### Design ✅
- [x] Design txtar-based script format
- [x] Define shell-like DSL syntax
- [x] Specify metadata format
- [x] Design assertion system
- [x] Document examples
- [x] Create `CDP_SCRIPT_FORMAT.md`

### Implementation
- [ ] **Core Parser**:
  - [ ] Txtar file parser
  - [ ] Metadata YAML parser
  - [ ] Command DSL lexer/parser
  - [ ] Variable substitution
  - [ ] Include/import resolution

- [ ] **Command Execution Engine**:
  - [x] Navigation commands (goto, back, forward, reload)
  - [x] Wait commands (for selector, until condition, duration)
  - [x] Interaction commands (click, fill, type, hover, press, select, scroll)
  - [x] Extraction commands (extract, save)
  - [x] Assertion commands (assert selector, status, errors)
  - [x] Network commands (capture, mock, block, throttle)
  - [x] Output commands (screenshot, pdf, har)
  - [x] JavaScript execution (js blocks, js files)
  - [ ] Control flow (if, for, include)

- [ ] **Browser Integration**:
  - [x] Browser lifecycle management
  - [x] Session state tracking
  - [x] Network interception setup
  - [x] HAR recording integration
  - [x] DevTools connection

- [ ] **Testing & Validation**:
  - [ ] Assertion engine implementation
  - [ ] DOM assertions
  - [ ] Network assertions
  - [ ] Performance assertions
  - [ ] Console log assertions
  - [ ] Visual regression (screenshot comparison)

- [ ] **CLI Integration**:
  - [ ] `cdp script <file>` command
  - [ ] Shebang support (`#!/usr/bin/env cdp script`)
  - [ ] Variable override flags
  - [ ] Matrix testing support
  - [ ] Watch mode
  - [ ] Parallel execution

- [ ] **Documentation & Examples**:
  - [ ] Tutorial/guide
  - [ ] Example scripts repository
  - [ ] Integration with CI/CD guide
  - [ ] VS Code extension (syntax highlighting)

**Estimated Effort**: 4-6 weeks
**Owner**: @tmc
**Status**: Design Complete, Implementation Pending

---

## Priority 2: Churl Mirror Feature (Q1-Q2 2025)

### Design ✅
- [x] Research existing tools (wget, HTTrack, ArchiveBox)
- [x] Design SPA-aware crawling strategy
- [x] Design link discovery methods
- [x] Design state preservation
- [x] Create `MIRROR_DESIGN.md`
- [x] Create `MIRROR_RESEARCH.md`

### Phase 1: Basic Mirroring
- [ ] **Core Infrastructure**:
  - [ ] URL queue with priority
  - [ ] Visited URL tracking (deduplication)
  - [ ] Depth tracking
  - [ ] Asset registry
  - [ ] Directory structure generation

- [ ] **wget-Compatible Flags**:
  - [ ] `-m, --mirror` (shorthand)
  - [ ] `-r, --recursive`
  - [ ] `-l, --level=NUM`
  - [ ] `-k, --convert-links`
  - [ ] `-p, --page-requisites`
  - [ ] `-np, --no-parent`
  - [ ] `-H, --span-hosts`

- [ ] **Accept/Reject Filters**:
  - [ ] `-A, --accept=LIST`
  - [ ] `-R, --reject=LIST`
  - [ ] `-D, --domains=LIST`
  - [ ] `--exclude-domains=LIST`
  - [ ] `--accept-regex=REGEX`
  - [ ] `--reject-regex=REGEX`

- [ ] **Link Discovery**:
  - [ ] HTML href extraction
  - [ ] CSS url() extraction
  - [ ] JavaScript src extraction
  - [ ] Sitemap.xml parsing
  - [ ] robots.txt handling

### Phase 2: SPA Support
- [ ] **Framework Detection**:
  - [ ] React Router detection
  - [ ] Vue Router detection
  - [ ] Angular Router detection
  - [ ] Generic SPA detection

- [ ] **Route Discovery**:
  - [ ] Router config parsing (React)
  - [ ] Router config parsing (Vue)
  - [ ] Router config parsing (Angular)
  - [ ] Link click simulation
  - [ ] History API monitoring

- [ ] **State Preservation**:
  - [ ] localStorage capture
  - [ ] sessionStorage capture
  - [ ] IndexedDB export
  - [ ] Redux state capture
  - [ ] Vuex state capture

- [ ] **Stability Detection**:
  - [ ] Network idle detection
  - [ ] DOM mutation observation
  - [ ] Framework-specific ready checks
  - [ ] Configurable stability timeout

### Phase 3: Advanced Features
- [ ] **API Capture & Replay**:
  - [ ] HAR recording per route
  - [ ] API response storage
  - [ ] Request/response rewriting
  - [ ] Mock API generation

- [ ] **WebSocket Capture**:
  - [ ] WebSocket frame recording
  - [ ] Message replay generation
  - [ ] Connection state tracking

- [ ] **Offline Mirror Generation**:
  - [ ] Link rewriting engine
  - [ ] Base href injection
  - [ ] Service worker generation
  - [ ] Index/sitemap generation
  - [ ] Metadata file creation

- [ ] **Screenshot Collection**:
  - [ ] Per-route screenshots
  - [ ] Viewport variations
  - [ ] Visual index generation

### Phase 4: Polish & Performance
- [ ] **Optimization**:
  - [ ] Parallel downloads
  - [ ] Connection pooling
  - [ ] Asset deduplication
  - [ ] Incremental updates
  - [ ] Resume support

- [ ] **User Experience**:
  - [ ] Progress reporting
  - [ ] ETA calculation
  - [ ] Bandwidth limiting
  - [ ] Quota enforcement
  - [ ] Summary statistics

**Estimated Effort**: 8-12 weeks
**Owner**: @tmc
**Status**: Design Complete, Implementation Pending

---

## Priority 3: Enhanced Integration & Testing

### Cross-Tool Integration
- [ ] **cdp + churl Integration**:
  - [ ] Use cdp scripts in churl operations
  - [ ] Share browser instances
  - [ ] Unified configuration

- [ ] **chdb Integration**:
  - [ ] Share debugging infrastructure
  - [ ] Common browser pool
  - [ ] Unified target management

- [ ] **ndp Integration**:
  - [ ] Node.js process debugging from CDP
  - [ ] Unified V8 inspector interface

### Testing Infrastructure
- [ ] **Unit Tests**:
  - [ ] CDP script parser tests
  - [ ] Mirroring link discovery tests
  - [ ] State preservation tests
  - [ ] Link rewriting tests

- [ ] **Integration Tests**:
  - [ ] End-to-end CDP script execution
  - [ ] Full mirror workflows
  - [ ] SPA framework tests (React, Vue, Angular)
  - [ ] API mocking tests

- [ ] **Performance Tests**:
  - [ ] Benchmark CDP script execution
  - [ ] Mirror performance with large sites
  - [ ] Memory usage profiling
  - [ ] Concurrency stress tests

**Estimated Effort**: 4 weeks
**Owner**: @tmc
**Status**: Planning

---

## Priority 4: Documentation & Community

### Documentation
- [ ] **User Guides**:
  - [ ] CDP scripting guide
  - [ ] Mirror usage guide
  - [ ] Best practices document
  - [ ] Troubleshooting guide

- [ ] **Video Tutorials**:
  - [ ] CDP basics (5 min)
  - [ ] Writing automation scripts (15 min)
  - [ ] Mirroring SPAs (10 min)
  - [ ] Advanced features (20 min)

- [ ] **API Documentation**:
  - [ ] CDP script command reference
  - [ ] Mirror flag reference
  - [ ] Go package documentation

### Examples Repository
- [ ] **CDP Script Examples**:
  - [ ] Login flow automation
  - [ ] Data scraping
  - [ ] API testing
  - [ ] Visual regression
  - [ ] Performance testing

- [ ] **Mirror Examples**:
  - [ ] Static site mirror
  - [ ] React app mirror
  - [ ] Documentation site mirror
  - [ ] Blog mirror with API

### Community Building
- [ ] Set up GitHub Discussions
- [ ] Create issue templates
- [ ] Contributor guidelines
- [ ] Code of conduct
- [ ] First good issues labels

**Estimated Effort**: 3 weeks
**Owner**: @tmc
**Status**: Planning

---

## Future Enhancements (Q3-Q4 2025)

### Advanced CDP Features
- [ ] Visual recording to CDP script
- [ ] CDP script debugger/stepper
- [ ] Plugin system for custom commands
- [ ] DAP (Debug Adapter Protocol) support

### Advanced Mirroring
- [ ] Distributed mirroring
- [ ] Delta/incremental mirroring
- [ ] Visual diff comparison
- [ ] Performance budgets
- [ ] Accessibility checks
- [ ] SEO analysis

### IDE Integration
- [ ] VS Code extension
  - [ ] Syntax highlighting for .cdp files
  - [ ] IntelliSense/autocomplete
  - [ ] Integrated debugging
  - [ ] Test runner
- [ ] Vim/Neovim plugin
- [ ] JetBrains plugin

### Cloud Services
- [ ] Hosted mirror service
- [ ] Scheduled mirroring
- [ ] Change notifications
- [ ] Team collaboration

---

## Implementation Guidelines

### Code Style
- Follow existing Go conventions (see `CLAUDE.md`)
- Use Russ Cox style
- Write tests first (TDD where appropriate)
- Document exported functions

### Git Workflow
- Create feature branches
- Use `git-auto-commit-message --auto`
- Add git notes with commit metadata
- Never force push to main

### Testing
- Minimum 70% code coverage for new features
- Integration tests for user-facing features
- Performance benchmarks for critical paths

### Documentation
- Update ROADMAP.md as features complete
- Update this TODO.md weekly
- Document breaking changes
- Provide migration guides

---

## Dependencies & Prerequisites

### External Dependencies
- Go 1.21+
- Chrome/Chromium browser
- chromedp library (current)
- txtar parsing library (new)
- YAML parser (new)

### Internal Dependencies
- Browser management (`internal/browser`)
- Chrome profiles (`internal/chromeprofiles`)
- HAR recording (`internal/recorder`)
- Blocking system (`internal/blocking`)

---

## Success Metrics

### CDP Script Format
- [ ] 50+ example scripts
- [ ] Used in 5+ projects
- [ ] <100ms script parse time
- [ ] <10ms command execution overhead

### Churl Mirror
- [ ] Successfully mirror 10 popular SPA sites
- [ ] <30s for typical documentation site
- [ ] <5% link breakage rate
- [ ] Functional offline replay

### Community
- [ ] 1000+ GitHub stars
- [ ] 10+ contributors
- [ ] 100+ weekly downloads
- [ ] Active Discord/discussions

---

## Questions & Decisions

### Open Questions
1. Should CDP scripts support TypeScript/ESM for JavaScript blocks?
2. Should mirroring support WARC format output?
3. Should we support browser extensions in mirrors?
4. How to handle authentication in automated mirrors?

### Decisions Made
- ✅ Use txtar format for CDP scripts (portable, single file)
- ✅ Prioritize wget flag compatibility for mirrors
- ✅ Focus on modern SPAs (React, Vue, Angular) first
- ✅ Browser automation is core feature, not optional

---

**Note**: This is a living document. Update weekly as features progress.
