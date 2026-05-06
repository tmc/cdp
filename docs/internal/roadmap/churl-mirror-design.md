# Churl Mirror - SPA-Aware Website Mirroring

## Overview
A wget-like mirroring capability for `churl` that understands SPAs (Single Page Applications), JavaScript-rendered content, and modern web applications.

## Problem Statement
Traditional tools like wget cannot properly mirror modern SPAs because:
1. Content is rendered by JavaScript, not in initial HTML
2. Routes are client-side (pushState/replaceState)
3. Assets are loaded dynamically
4. API calls happen after page load
5. WebSocket connections for real-time updates
6. Service workers intercept requests

## Design Goals
1. **SPA-Aware**: Understand and navigate client-side routes
2. **Complete Capture**: Get all dynamically loaded resources
3. **Functional Offline**: Rewrite URLs for local browsing
4. **Framework Agnostic**: Work with React, Vue, Angular, etc.
5. **Configurable**: Control depth, scope, and filtering
6. **Efficient**: Avoid duplicate downloads

## Command Interface

### Basic Usage
```bash
# Mirror a SPA with default settings
churl --mirror https://example.com

# wget-compatible shorthand
churl -m https://example.com
```

### Key Flags (wget-compatible)

#### Core Mirroring Flags
```bash
-m, --mirror              # Shortcut for -r -l inf -k --wait-for-stable
-r, --recursive           # Enable recursive downloading
-l, --level=NUM           # Maximum recursion depth (0 = infinite)
-k, --convert-links       # Convert links to relative paths for offline use
-p, --page-requisites     # Download all assets needed to display page
-np, --no-parent          # Don't ascend to parent directory
-H, --span-hosts          # Follow links to other domains
```

#### Accept/Reject Filters
```bash
-A, --accept=LIST         # Accept file extensions (e.g., html,js,css)
-R, --reject=LIST         # Reject file extensions (e.g., mp4,zip)
-D, --domains=LIST        # Accept only these domains
--exclude-domains=LIST    # Exclude these domains
-I, --include-dirs=LIST   # Include only these directories
-X, --exclude-dirs=LIST   # Exclude these directories
--accept-regex=REGEX      # Accept URLs matching regex
--reject-regex=REGEX      # Reject URLs matching regex
```

#### Download Control
```bash
-w, --wait=SECONDS        # Wait between downloads (polite crawling)
-nc, --no-clobber         # Don't re-download existing files
-N, --timestamping        # Only download newer files
-c, --continue            # Resume partial downloads
--limit-rate=RATE         # Limit download speed
-Q, --quota=SIZE          # Maximum total download size
```

#### Output Options
```bash
-P, --directory-prefix=DIR  # Save files to DIR
-nd, --no-directories       # Don't create directory hierarchy
-x, --force-directories     # Force creation of directories
--cut-dirs=NUM             # Ignore NUM remote directory components
-nH, --no-host-directories # Don't create host-based directories
```

### SPA-Specific Flags

#### Navigation & Discovery
```bash
--spa-mode=MODE           # SPA detection mode: auto, react, vue, angular, none
--discover-routes         # Analyze client-side router for routes
--click-links             # Click navigation links to discover routes
--max-click-depth=NUM     # Maximum depth for link clicking
--wait-for-stable         # Wait for network/DOM to stabilize before extracting
--stability-timeout=SEC   # Max time to wait for stability (default: 30s)
```

#### Content Capture
```bash
--capture-api-calls       # Save API responses in HAR format
--capture-websockets      # Capture WebSocket messages
--save-state=FILE         # Save application state (localStorage, etc.)
--screenshots             # Take screenshots of each page
--extract-data=SELECTOR   # Extract and save data from DOM
```

#### JavaScript Handling
```bash
--execute-js=SCRIPT       # Run JavaScript before extraction
--inject-script=FILE      # Inject custom script on each page
--disable-analytics       # Block analytics/tracking scripts
--mock-apis=FILE          # Use mock API responses
```

#### Link Rewriting
```bash
--convert-spa-routes      # Convert SPA routes to static HTML files
--rewrite-api-calls       # Rewrite API calls to local files
--generate-index          # Generate index.html with sitemap
--base-href=PATH          # Set base href for relative URLs
```

## Implementation Strategy

### Phase 1: Link Discovery
1. **Load initial page** with full JavaScript execution
2. **Wait for stability** (network idle, no DOM mutations)
3. **Extract links** from:
   - DOM (a[href], link[href], script[src], img[src])
   - Router configuration (React Router, Vue Router, etc.)
   - Sitemap.xml / robots.txt
   - API responses (if they contain URLs)
4. **Build URL queue** with deduplication

### Phase 2: Intelligent Crawling
```go
type CrawlState struct {
    Visited    map[string]bool      // URLs already processed
    Queue      *PriorityQueue       // URLs to visit
    Assets     map[string]*Asset    // Downloaded assets
    Routes     []*Route             // Discovered SPA routes
    Depth      int                  // Current recursion depth
    Stats      *CrawlStats          // Statistics
}

type Route struct {
    Path       string
    Component  string
    Method     string  // navigate, click, direct
    Parent     string
    Depth      int
    Assets     []string
}
```

### Phase 3: Asset Extraction
For each page/route:
1. Navigate to URL (or trigger SPA route change)
2. Wait for content to load
3. Extract all resources:
   ```go
   - HTML snapshot (after JavaScript execution)
   - CSS files and inline styles
   - JavaScript files
   - Images, fonts, media
   - API responses (from HAR)
   - WebSocket messages
   - localStorage/sessionStorage
   ```

### Phase 4: Link Conversion
```go
type LinkRewriter struct {
    Rules map[string]RewriteRule
}

// Convert absolute URLs to relative
// /api/users -> data/api/users.json
// /app/dashboard -> app/dashboard/index.html
// ws://example.com/live -> data/websockets/live.json
```

### Phase 5: Offline Generation
Generate static mirror:
```
mirror/
├── index.html                    # Entry point
├── app/
│   ├── dashboard/
│   │   ├── index.html           # SPA route snapshot
│   │   └── assets/
│   └── settings/
│       └── index.html
├── static/
│   ├── js/
│   ├── css/
│   └── images/
├── data/
│   ├── api/                     # API responses
│   │   ├── users.json
│   │   └── posts.json
│   └── websockets/              # WebSocket messages
│       └── chat.json
├── _spa_config.json             # Router config, state
└── _mirror_metadata.json        # Mirror info, stats
```

## SPA Framework Detection

### React
```javascript
// Detect React Router
const routes = window.__REACT_ROUTER_ROUTES__ ||
               document.querySelectorAll('[data-reactroot]');

// Extract routes from React Router config
function extractReactRoutes() {
    // Parse route configuration
    // Look for <Route path="..." /> components
}
```

### Vue
```javascript
// Detect Vue Router
const router = window.$router || window.__VUE_ROUTER__;

// Get routes from Vue Router
function extractVueRoutes() {
    return router.options.routes.map(route => ({
        path: route.path,
        component: route.component.name
    }));
}
```

### Angular
```javascript
// Detect Angular Router
const router = window.ng?.probe(document.body)?.injector?.get('Router');

// Extract routes
function extractAngularRoutes() {
    return router.config.map(route => ({
        path: route.path,
        component: route.component?.name
    }));
}
```

## Examples

### Example 1: Mirror a React Documentation Site
```bash
churl --mirror \
      --spa-mode=react \
      --discover-routes \
      --level=3 \
      --page-requisites \
      --convert-links \
      --directory-prefix=./mirror/react-docs \
      https://react.dev
```

### Example 2: Mirror a SPA with API Mocking
```bash
churl --mirror \
      --capture-api-calls \
      --rewrite-api-calls \
      --save-state=app-state.json \
      --inject-script=mock-apis.js \
      --directory-prefix=./mirror/app \
      https://app.example.com
```

### Example 3: Selective Mirroring with Filtering
```bash
churl --mirror \
      --accept=html,js,css,json,svg,png \
      --reject-regex='.*\.(mp4|webm|zip|tar\.gz)' \
      --exclude-dirs=/admin,/legacy \
      --domains=example.com,cdn.example.com \
      --limit-rate=1M \
      https://example.com
```

### Example 4: Mirror with Screenshots
```bash
churl --mirror \
      --screenshots \
      --wait-for-stable \
      --stability-timeout=10 \
      --click-links \
      --max-click-depth=2 \
      --directory-prefix=./visual-mirror \
      https://example.com
```

### Example 5: Development/Testing Mirror
```bash
churl --mirror \
      --capture-api-calls \
      --capture-websockets \
      --save-state=dev-state.json \
      --mock-apis=mocks.json \
      --generate-index \
      --verbose \
      https://localhost:3000
```

## Advanced Features

### Smart Link Discovery
```go
// Discover links beyond simple href extraction
type LinkDiscovery struct {
    Methods []DiscoveryMethod
}

type DiscoveryMethod interface {
    Discover(page *Page) []string
}

// HrefDiscovery - traditional <a href="">
// RouterDiscovery - client-side router configs
// APIDiscovery - URLs from API responses
// SitemapDiscovery - sitemap.xml parsing
// JSVariableDiscovery - JavaScript variables containing URLs
// NetworkDiscovery - URLs from network activity
```

### Incremental Mirroring
```bash
# First mirror
churl --mirror --timestamping --directory-prefix=./mirror https://example.com

# Update mirror (only changed files)
churl --mirror --timestamping --continue --no-clobber --directory-prefix=./mirror https://example.com
```

### Distributed Mirroring
```bash
# Coordinated multi-instance mirroring
churl --mirror \
      --distributed \
      --instance-id=1 \
      --total-instances=4 \
      --shared-state=redis://localhost \
      https://example.com
```

## Output Formats

### Progress Output
```
Mirroring https://example.com
  Discovering routes... [React Router detected]
  Found 47 routes, 234 assets

  Progress: [=====>           ] 35% (12/47 routes)
  Downloaded: 2.3 MB / ~6.5 MB
  Time: 00:02:15 / ~00:06:30
  Rate: 17 KB/s

  Current: /app/dashboard (depth: 2)
  Queue: 35 routes, 189 assets
```

### Summary Output
```
Mirror complete!

  Statistics:
    Total routes:     47 (React Router)
    Pages saved:      47
    Assets saved:     234
    Total size:       6.8 MB
    Duration:         00:06:42
    Avg speed:        17.3 KB/s

  Saved to: ./mirror/example-com/
  Entry point: ./mirror/example-com/index.html

  Browse offline:
    cd mirror/example-com && python -m http.server 8000
```

## Implementation Priority

### P0 - Core Functionality
1. Basic recursive crawling with depth limit
2. Asset extraction (images, CSS, JS)
3. Simple link conversion
4. Directory structure creation

### P1 - SPA Support
5. Wait for stability detection
6. Client-side route discovery
7. SPA framework detection
8. Router config parsing

### P2 - Advanced Features
9. API call capture and rewriting
10. WebSocket capture
11. State preservation
12. Screenshot capture

### P3 - Optimizations
13. Incremental updates
14. Distributed crawling
15. Smart caching
16. Parallel downloads

## Testing Strategy

### Test Cases
1. **Static sites**: Verify wget compatibility
2. **React apps**: CRA, Next.js, Gatsby
3. **Vue apps**: Vue CLI, Nuxt
4. **Angular apps**: Angular CLI
5. **Mixed content**: SPA with static assets
6. **API-driven**: Apps with REST APIs
7. **WebSocket apps**: Real-time applications

### Test Scenarios
```bash
# Test 1: Simple static site
churl --mirror --level=2 https://simple-static-site.com

# Test 2: React SPA
churl --mirror --spa-mode=react --discover-routes https://react-spa.com

# Test 3: API-heavy app
churl --mirror --capture-api-calls --rewrite-api-calls https://api-app.com

# Test 4: WebSocket app
churl --mirror --capture-websockets https://realtime-app.com
```

## Performance Considerations

### Optimization Strategies
1. **Parallel Downloads**: Download assets in parallel
2. **Connection Pooling**: Reuse browser tabs/contexts
3. **Smart Caching**: Cache DOM snapshots, route configs
4. **Deduplication**: Hash-based asset deduplication
5. **Compression**: Compress large JSON/HAR files
6. **Streaming**: Stream large files to disk

### Resource Limits
```bash
--max-concurrent=NUM      # Max parallel operations (default: 10)
--memory-limit=SIZE       # Max memory usage
--disk-cache=SIZE         # Disk cache size
--timeout-multiplier=N    # Adjust timeouts based on complexity
```

## Future Enhancements

1. **Visual Regression**: Compare mirrors over time
2. **Accessibility Check**: Run a11y checks during mirroring
3. **Performance Budget**: Track performance metrics
4. **Security Scan**: Detect security issues
5. **SEO Analysis**: Analyze SEO during mirror
6. **PWA Support**: Handle service workers properly
7. **PDF Generation**: Generate PDF from mirror
8. **Archive Format**: Create distributable archives

---

**Status**: Design Phase
**Target**: Q2 2025
**Owner**: @tmc
