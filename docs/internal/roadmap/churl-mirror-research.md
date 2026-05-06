# SPA Mirroring & Web Archiving Tools Research

## Existing Tools in This Space

### Traditional Mirroring Tools
1. **wget** (1996)
   - Static site mirroring
   - No JavaScript execution
   - Limited to server-rendered content
   - Gold standard for flags/interface

2. **HTTrack** (1998)
   - GUI and CLI versions
   - Better link conversion than wget
   - Still no JavaScript support
   - Windows-focused

3. **curl** (1997)
   - Single URL fetching (not mirroring)
   - Scriptable, composable
   - No JavaScript execution

### Modern Web Archiving

4. **ArchiveBox** (2017)
   - Full-featured web archiving
   - Uses Chromium for JavaScript
   - Multiple format outputs (HTML, PDF, screenshot, WARC)
   - Self-hosted archiving platform
   - **Key insight**: Browser integration is central

5. **SingleFile** (Browser Extension)
   - Saves complete page as single HTML file
   - Inlines all assets
   - Works with SPAs
   - Browser-based architecture
   - **Key insight**: Running in browser context is powerful

6. **Webrecorder** / **ArchiveWeb.page**
   - Browser-based recording
   - WARC format
   - Replay capability
   - Records user interactions
   - **Key insight**: Interactive recording matters

7. **Monolith** (Rust CLI)
   - Save page as single HTML file
   - Bundles all assets inline
   - No JavaScript execution
   - Fast and minimal

### SPA-Specific Solutions

8. **Puppeteer/Playwright + custom scripts**
   - Most common approach for SPAs
   - Requires custom scripting
   - No standard tool
   - **Gap**: No standardized SPA mirroring tool

9. **Prerender.io** / **Rendertron**
   - Headless Chrome rendering
   - Server-side solution
   - For SEO, not archiving
   - **Key insight**: Chrome as a service model

10. **Browsertrix Crawler**
    - High-fidelity web archiving
    - Built on Puppeteer
    - WARC format
    - Distributed crawling
    - **Key insight**: Professional-grade needs distributed architecture

## Browser Access Opportunities

### What Browser Access Enables

#### 1. **Runtime Inspection**
```javascript
// Access to live application state
window.__REACT_ROUTER__
window.$nuxt
window.ng.probe()
localStorage
sessionStorage
IndexedDB
```

#### 2. **Event Simulation**
```javascript
// Trigger application behaviors
click(), scroll(), hover()
keyboard events
touch events
Form submissions
```

#### 3. **Network Interception**
```javascript
// Chrome DevTools Protocol
Network.requestWillBeSent
Network.responseReceived
Fetch interception
Service Worker interception
```

#### 4. **DOM Manipulation**
```javascript
// Inject helpers into page
window.__MIRROR_HELPER__ = {
    collectLinks: function() { ... },
    extractData: function() { ... },
    serializeState: function() { ... }
}
```

#### 5. **Performance Data**
```javascript
performance.getEntries()
PerformanceObserver
Core Web Vitals
Resource timing
```

### Browser Integration Patterns

#### Pattern 1: Injected Helper Script
```javascript
// Inject a helper that understands SPAs
const helper = `
  (function() {
    window.__CHURL_MIRROR__ = {
      version: '1.0',

      // Discover all routes
      discoverRoutes: async function() {
        const routes = new Set();

        // React Router
        if (window.__reactRouter) {
          routes.add(...extractReactRoutes());
        }

        // Vue Router
        if (window.$nuxt?.$router) {
          routes.add(...extractVueRoutes());
        }

        // Angular Router
        if (window.ng) {
          routes.add(...extractAngularRoutes());
        }

        // Generic: scan for data-route, href, etc.
        routes.add(...extractGenericRoutes());

        return Array.from(routes);
      },

      // Serialize application state
      captureState: function() {
        return {
          localStorage: {...localStorage},
          sessionStorage: {...sessionStorage},
          indexedDB: await exportIndexedDB(),
          cookies: document.cookie,
          redux: window.__REDUX_DEVTOOLS_EXTENSION__?.store?.getState(),
          vuex: window.$nuxt?.$store?.state,
        };
      },

      // Extract structured data
      extractData: function(selectors) {
        return selectors.map(sel => ({
          selector: sel,
          elements: Array.from(document.querySelectorAll(sel)).map(el => ({
            text: el.textContent,
            html: el.innerHTML,
            attributes: Object.fromEntries(
              Array.from(el.attributes).map(a => [a.name, a.value])
            )
          }))
        }));
      },

      // Wait for SPA to be ready
      waitForReady: function() {
        return new Promise(resolve => {
          // Framework-specific ready checks
          if (window.__reactReady) resolve();
          if (window.$nuxt?.isReady) resolve();
          // etc.

          // Fallback: network idle + no mutations
          setTimeout(resolve, 2000);
        });
      }
    };
  })();
`;

// Inject before navigating
await page.evaluateOnNewDocument(helper);
```

#### Pattern 2: CDP Network Interception
```go
// Intercept and mock APIs during playback
type NetworkInterceptor struct {
    capturedAPIs map[string]*APIResponse
}

func (ni *NetworkInterceptor) Intercept(ctx context.Context) {
    chromedp.Run(ctx,
        network.Enable(),
        network.SetRequestInterception(network.RequestStageRequest),
    )

    chromedp.ListenTarget(ctx, func(ev interface{}) {
        switch ev := ev.(type) {
        case *network.EventRequestPaused:
            url := ev.Request.URL

            // Check if we have captured this API call
            if resp, ok := ni.capturedAPIs[url]; ok {
                // Continue with mocked response
                network.ContinueInterceptedRequest(ev.RequestID).
                    WithBody(resp.Body).
                    WithStatus(resp.StatusCode).
                    Do(ctx)
            } else {
                // Let it through and capture
                network.ContinueInterceptedRequest(ev.RequestID).Do(ctx)
            }
        }
    })
}
```

#### Pattern 3: Browser Extension Integration
```javascript
// Chrome extension that helps with mirroring
chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
    if (msg.action === 'mirror') {
        // Extension can access more than page scripts
        chrome.tabs.query({}, tabs => {
            // Get all tabs
            // Access to extension storage
            // Can intercept all requests globally
        });
    }
});
```

### Powerful Browser-Based Features

#### 1. Visual Diff & Testing
```bash
# Mirror, then compare visually
churl --mirror --screenshots --visual-baseline=./baseline https://app.com

# Later, check for visual changes
churl --mirror --screenshots --visual-diff=./baseline https://app.com
```

#### 2. Interactive Mirroring
```bash
# Let user interact, capture everything
churl --mirror --interactive --record-actions https://app.com

# Replay captured interactions
churl --replay=actions.json --mirror https://app.com
```

#### 3. Browser DevTools Integration
```bash
# Mirror with DevTools open for inspection
churl --mirror --devtools --pause-on-route https://app.com

# This opens DevTools, pauses on each route
# User can inspect, modify, then continue
```

#### 4. Service Worker Aware
```go
// Detect and handle Service Workers
type ServiceWorkerHandler struct {
    scripts map[string]string
}

func (sw *ServiceWorkerHandler) Capture(ctx context.Context) {
    // Get service worker script
    // Understand caching strategy
    // Export cache contents
    // Generate offline-capable mirror
}
```

#### 5. Progressive Enhancement
```bash
# Mirror with different capability levels
churl --mirror --capabilities=baseline https://app.com   # No JS
churl --mirror --capabilities=enhanced https://app.com   # JS enabled
churl --mirror --capabilities=modern https://app.com     # Full modern features

# Compare outputs
diff -r mirror-baseline/ mirror-modern/
```

## Unique Value Propositions

### What Makes churl --mirror Different?

1. **SPA-Native**: Built for modern SPAs from the ground up
2. **Developer-Friendly**: Uses familiar wget flags
3. **Programmable**: Inject custom logic via scripts
4. **Observable**: Full network, performance, state capture
5. **Testable**: Built-in visual/functional testing capabilities
6. **Composable**: Works with other CLI tools (jq, grep, diff)

### Killer Features

#### Feature 1: Smart State Preservation
```bash
# Capture entire app state, replay perfectly
churl --mirror \
      --capture-state \
      --save-state=app.state \
      --replay-offline \
      https://app.com

# Generated mirror includes:
# - All routes with pre-rendered HTML
# - All API data as static JSON
# - Application state (Redux, Vuex, etc.)
# - Offline-first service worker
```

#### Feature 2: Development Mirror
```bash
# Create a mirror optimized for development
churl --mirror \
      --dev-mode \
      --hot-reload \
      --mock-apis \
      --source-maps \
      https://localhost:3000

# Result: Fully functional dev environment snapshot
```

#### Feature 3: Testing Fixtures
```bash
# Generate test fixtures from live site
churl --mirror \
      --generate-fixtures \
      --fixture-format=jest \
      --extract-schema \
      https://api.example.com

# Outputs:
# - Mock API responses
# - TypeScript interfaces
# - Test data generators
```

#### Feature 4: Documentation Generation
```bash
# Mirror + auto-generate docs
churl --mirror \
      --generate-docs \
      --doc-format=markdown \
      --include-screenshots \
      --include-flows \
      https://app.example.com

# Generates:
# - Sitemap documentation
# - User flow diagrams
# - Screenshot gallery
# - API endpoint catalog
```

## Architecture for Browser Integration

### Three-Tier Architecture

```
┌─────────────────────────────────────────┐
│  CLI Layer (churl binary)               │
│  - Argument parsing                     │
│  - Progress reporting                   │
│  - File I/O                             │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│  Orchestration Layer                    │
│  - Browser lifecycle management         │
│  - Queue management                     │
│  - Link discovery                       │
│  - Asset tracking                       │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│  Browser Layer (Chrome via CDP)         │
│  - Page rendering                       │
│  - Network interception                 │
│  - State extraction                     │
│  - Script injection                     │
└─────────────────────────────────────────┘
```

### Plugin Architecture

```go
type MirrorPlugin interface {
    Name() string
    OnPageLoad(ctx context.Context, page *Page) error
    OnBeforeNavigate(ctx context.Context, url string) error
    OnAssetDiscovered(ctx context.Context, asset *Asset) error
    OnComplete(ctx context.Context, stats *Stats) error
}

// Example plugins:
// - SPAFrameworkPlugin (React/Vue/Angular detection)
// - APICapture Plugin (HAR generation)
// - ScreenshotPlugin (Visual capture)
// - ValidationPlugin (Link checker)
// - PerformancePlugin (Metrics collection)
```

## Competitive Positioning

| Tool | Static | SPA | API Capture | State | Interactive | Offline Replay |
|------|--------|-----|-------------|-------|-------------|----------------|
| wget | ✅ | ❌ | ❌ | ❌ | ❌ | ⚠️ |
| HTTrack | ✅ | ❌ | ❌ | ❌ | ❌ | ✅ |
| ArchiveBox | ✅ | ✅ | ⚠️ | ❌ | ❌ | ✅ |
| SingleFile | ✅ | ✅ | ❌ | ❌ | ❌ | ✅ |
| Browsertrix | ✅ | ✅ | ✅ | ⚠️ | ⚠️ | ✅ |
| **churl --mirror** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

## Next Steps

1. **Prototype**: Build minimal viable mirror command
2. **Test**: Run against popular SPA frameworks
3. **Iterate**: Add SPA-specific features based on learnings
4. **Document**: Write comprehensive guides
5. **Community**: Get feedback from users

---

**Key Insight**: Browser access is not just an implementation detail—it's the core differentiator that enables true SPA mirroring, state preservation, and offline replay capabilities.
