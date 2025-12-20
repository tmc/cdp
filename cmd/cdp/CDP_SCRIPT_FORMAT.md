# CDP Script Format - Browser Automation with Txtar

## Overview
A scriptable format for CDP that uses txtar (text archive) format to define browser automation workflows with embedded commands, assertions, and data files.

## Motivation
- **Reproducible**: Share browser automation scripts as single files
- **Testable**: Include assertions and expected outputs
- **Composable**: Embed helper scripts, CSS, data files
- **Executable**: Run directly with `#!/usr/bin/env cdp script`
- **Versionable**: Plain text format works with git

## File Format

### Basic Structure
```
#!/usr/bin/env cdp script
-- metadata.yaml --
name: Example Automation
version: 1.0
browser: chrome
timeout: 30s

-- main.cdp --
# CDP commands and JavaScript
goto https://example.com
wait for #content
screenshot example.png

-- assertions.yaml --
- selector: title
  contains: "Example Domain"
- selector: h1
  text: "Example Domain"
```

## CDP Script Sections

### 1. Metadata Section (`metadata.yaml`)
```yaml
name: Login Flow Test
description: Test login functionality
version: 1.0.0
author: user@example.com

# Browser configuration
browser: chrome  # chrome, brave, chromium
profile: default
headless: false
timeout: 60s

# Environment
env:
  BASE_URL: https://app.example.com
  TEST_USER: test@example.com
  TEST_PASS: ${SECRET_PASSWORD}  # From environment

# Dependencies (other CDP scripts)
imports:
  - ./lib/helpers.cdp
  - ./lib/wait-utils.cdp
```

### 2. Main Script (`main.cdp`)
```bash
# Comments start with #
# Variables use ${VAR} syntax

# Navigation
goto ${BASE_URL}/login
wait for #login-form

# Interaction
fill #email ${TEST_USER}
fill #password ${TEST_PASS}
click button[type=submit]

# Waiting
wait for .dashboard
wait until network idle
wait 2s

# Assertions
assert selector .user-name contains ${TEST_USER}
assert status 200
assert no errors

# Extraction
extract .product-price as price
save $price to price.txt

# Screenshot
screenshot dashboard.png

# JavaScript execution
js {
  console.log('Dashboard loaded');
  return document.querySelector('.user-name').textContent;
} as userName

# Network operations
capture network to requests.har
mock api "/api/users" with users.json

# Debugging
devtools  # Opens DevTools and pauses
breakpoint  # Pauses execution
```

### 3. JavaScript Files
```
-- helpers.js --
// Helper functions available to all js {} blocks
function login(email, password) {
  document.querySelector('#email').value = email;
  document.querySelector('#password').value = password;
  document.querySelector('button[type=submit]').click();
}

function waitForElement(selector, timeout = 5000) {
  return new Promise((resolve, reject) => {
    const startTime = Date.now();
    const interval = setInterval(() => {
      const el = document.querySelector(selector);
      if (el) {
        clearInterval(interval);
        resolve(el);
      } else if (Date.now() - startTime > timeout) {
        clearInterval(interval);
        reject(new Error(`Timeout waiting for ${selector}`));
      }
    }, 100);
  });
}
```

### 4. Test Data
```
-- users.json --
{
  "users": [
    {"id": 1, "name": "Test User", "email": "test@example.com"},
    {"id": 2, "name": "Admin User", "email": "admin@example.com"}
  ]
}

-- expected-response.json --
{
  "status": "success",
  "data": {
    "items": []
  }
}
```

### 5. Assertions
```
-- assertions.yaml --
# DOM assertions
- type: selector
  selector: h1
  text: "Welcome"

- type: selector
  selector: .error
  exists: false

# Network assertions
- type: response
  url: /api/users
  status: 200
  json:
    users:
      length: 2

# Performance assertions
- type: performance
  metric: FCP
  max: 1000ms

- type: performance
  metric: LCP
  max: 2500ms

# Console assertions
- type: console
  level: error
  count: 0
```

### 6. Expected Outputs
```
-- expected/screenshot.png --
<binary data>

-- expected/data.json --
{"result": "success"}
```

## Command Reference

### Navigation
```bash
goto <url>                    # Navigate to URL
back                          # Browser back
forward                       # Browser forward
reload                        # Reload page
```

### Waiting
```bash
wait for <selector>           # Wait for element
wait until <condition>        # network idle, dom stable, load complete
wait <duration>               # Sleep for duration (1s, 500ms)
```

### Interaction
```bash
click <selector>              # Click element
fill <selector> <value>       # Fill input
type <selector> <text>        # Type text (simulates typing)
select <selector> <value>     # Select dropdown option
hover <selector>              # Hover over element
press <key>                   # Press keyboard key
scroll to <selector>          # Scroll element into view
```

### Extraction
```bash
extract <selector> as <var>        # Extract text content
extract <selector> attr src as <var>  # Extract attribute
save <var> to <file>              # Save variable to file
```

### Assertions
```bash
assert selector <sel> exists
assert selector <sel> contains <text>
assert selector <sel> text <text>
assert status <code>
assert no errors
assert url contains <text>
```

### Network
```bash
capture network to <file>     # Record network to HAR
mock api <pattern> with <file>  # Mock API responses
block <pattern>               # Block requests matching pattern
throttle <profile>            # Apply network throttling
```

### Output
```bash
screenshot <file>             # Take screenshot
screenshot <selector> <file>  # Screenshot element
pdf <file>                    # Save as PDF
har <file>                    # Export HAR
```

### JavaScript
```bash
js { <code> }                 # Execute JavaScript
js <file>                     # Execute JavaScript file
js { <code> } as <var>        # Execute and save result
```

### Control Flow
```bash
if <condition> {              # Conditional execution
  <commands>
}

for <var> in <list> {         # Loop
  <commands>
}

include <file>                # Include another script
```

### Debugging
```bash
devtools                      # Open DevTools
breakpoint                    # Pause execution
log <message>                 # Log to console
debug <var>                   # Print variable
```

## Example Scripts

### Example 1: Simple Page Test
```
#!/usr/bin/env cdp script
-- metadata.yaml --
name: Homepage Test
timeout: 10s

-- main.cdp --
goto https://example.com
wait for h1
assert selector h1 text "Example Domain"
screenshot homepage.png
```

### Example 2: Login Flow
```
#!/usr/bin/env cdp script
-- metadata.yaml --
name: Login Flow
env:
  URL: https://app.example.com
  USER: test@example.com

-- main.cdp --
goto ${URL}/login
fill #email ${USER}
fill #password ${PASSWORD}  # From environment
click button[type=submit]
wait until url contains /dashboard
assert selector .user-name exists
screenshot logged-in.png
```

### Example 3: Data Scraping
```
#!/usr/bin/env cdp script
-- metadata.yaml --
name: Product Scraper

-- main.cdp --
goto https://shop.example.com/products

# Wait for products to load
wait for .product-list
wait until network idle

# Extract all product data
js {
  const products = Array.from(document.querySelectorAll('.product')).map(p => ({
    name: p.querySelector('.name').textContent,
    price: p.querySelector('.price').textContent,
    image: p.querySelector('img').src
  }));
  return products;
} as products

save $products to products.json

-- assertions.yaml --
- type: variable
  name: products
  minLength: 10
```

### Example 4: API Testing with Mocking
```
#!/usr/bin/env cdp script
-- metadata.yaml --
name: API Mock Test

-- main.cdp --
# Mock the API response
mock api "/api/users" with users-mock.json

goto https://app.example.com
wait for .user-list

# Verify UI shows mocked data
assert selector .user-list li count 3

-- users-mock.json --
{
  "users": [
    {"id": 1, "name": "User 1"},
    {"id": 2, "name": "User 2"},
    {"id": 3, "name": "User 3"}
  ]
}
```

### Example 5: Performance Testing
```
#!/usr/bin/env cdp script
-- metadata.yaml --
name: Performance Test
timeout: 30s

-- main.cdp --
capture network to performance.har

goto https://app.example.com
wait until load complete

# Get performance metrics
js {
  const perfData = performance.getEntriesByType('navigation')[0];
  const paintData = performance.getEntriesByType('paint');

  return {
    loadTime: perfData.loadEventEnd - perfData.loadEventStart,
    domContentLoaded: perfData.domContentLoadedEventEnd - perfData.domContentLoadedEventStart,
    firstPaint: paintData.find(p => p.name === 'first-paint')?.startTime,
    firstContentfulPaint: paintData.find(p => p.name === 'first-contentful-paint')?.startTime
  };
} as metrics

save $metrics to metrics.json

-- assertions.yaml --
- type: variable
  name: metrics.loadTime
  max: 2000

- type: variable
  name: metrics.firstContentfulPaint
  max: 1000
```

### Example 6: Visual Regression Testing
```
#!/usr/bin/env cdp script
-- metadata.yaml --
name: Visual Regression

-- main.cdp --
goto https://app.example.com

# Take baseline screenshots
screenshot baseline/homepage.png
click #menu-products
wait for .product-list
screenshot baseline/products.png

# Load test with visual diff
screenshot current/homepage.png
compare current/homepage.png with baseline/homepage.png

-- assertions.yaml --
- type: visual
  file: current/homepage.png
  baseline: baseline/homepage.png
  maxDiff: 0.05  # 5% difference allowed
```

## Execution

### Running Scripts
```bash
# Direct execution (if executable)
./test.cdp

# Via cdp command
cdp script test.cdp

# With arguments
cdp script test.cdp --var USER=admin@example.com

# With different browser
cdp script test.cdp --browser brave

# Debug mode
cdp script test.cdp --debug --devtools

# Watch mode (re-run on changes)
cdp script test.cdp --watch
```

### Output
```
Running: Login Flow Test
✓ Navigate to https://app.example.com/login (234ms)
✓ Fill #email test@example.com (12ms)
✓ Fill #password ******** (15ms)
✓ Click button[type=submit] (89ms)
✓ Wait until url contains /dashboard (456ms)
✓ Assert selector .user-name exists (5ms)
✓ Screenshot logged-in.png (178ms)

Results:
  Total: 7 steps
  Passed: 7
  Failed: 0
  Duration: 989ms

Artifacts:
  - logged-in.png
```

## Integration with Other Tools

### Git Hooks
```bash
# .git/hooks/pre-push
#!/bin/bash
cdp script tests/*.cdp || exit 1
```

### CI/CD
```yaml
# .github/workflows/test.yml
- name: Run CDP tests
  run: |
    for script in tests/*.cdp; do
      cdp script "$script" || exit 1
    done
```

### Make
```makefile
test:
	@for f in tests/*.cdp; do \
		cdp script $$f || exit 1; \
	done
```

## Advanced Features

### Parameterized Scripts
```bash
# Run with different parameters
cdp script test.cdp \
  --var URL=https://staging.example.com \
  --var USER=staging@example.com
```

### Matrix Testing
```yaml
-- matrix.yaml --
browsers: [chrome, brave, chromium]
viewports:
  - {width: 1920, height: 1080, name: desktop}
  - {width: 375, height: 667, name: mobile}
environments:
  - {name: staging, url: https://staging.example.com}
  - {name: production, url: https://example.com}
```

```bash
cdp script test.cdp --matrix matrix.yaml
```

### Distributed Execution
```bash
# Run tests in parallel
cdp script tests/*.cdp --parallel --max-workers=4

# Distributed across machines
cdp script tests/*.cdp --distributed --workers=10
```

## Future Enhancements

1. **IDE Support**: VS Code extension for syntax highlighting
2. **Debugging**: Interactive debugging with breakpoints
3. **Recording**: Record browser interactions to generate scripts
4. **Sharing**: CDP script marketplace/registry
5. **Conversion**: Convert from/to Puppeteer, Playwright scripts

---

**Status**: Design Phase
**Target**: Q1 2025
**Owner**: @tmc
