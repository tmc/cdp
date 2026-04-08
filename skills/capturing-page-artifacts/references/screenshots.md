# CDP Screenshots & Visual Capture

Screenshot capture, PDF export, and visual testing with the cdp tool.

## Basic Screenshots

### In Scripts (txtar)

```
-- meta.yaml --
name: Screenshot Example
headless: true
timeout: 30s

-- main.cdp --
goto https://example.com
wait h1
screenshot page.png
```

The `screenshot` command captures a full-page screenshot at 90% quality. Files are saved relative to the output directory (set with `cdp run -o <dir>`).

### In Interactive Mode

```
cdp> goto https://example.com
cdp> screenshot                        # Auto-named: screenshot-<timestamp>.png
cdp> screenshot page.png               # Custom filename
cdp> snap page.png                     # Alias: snap, capture
```

## Screenshot with Output Directory

```bash
cdp run -o ./screenshots script.txtar
```

All `screenshot` commands in the script save to the output dir.

## Device Emulation Screenshots

### Mobile Screenshots

```
-- main.cdp --
goto https://example.com
wait h1

# Mobile (iPhone-like: 375x812 @3x)
js window.scrollTo(0, 0)
screenshot mobile.png
```

In interactive mode, change viewport first:
```
cdp> mobile
cdp> goto https://example.com
cdp> screenshot mobile.png
cdp> desktop
cdp> screenshot desktop.png
```

### Custom Viewport

```
cdp> viewport 1024 768
cdp> screenshot tablet-landscape.png
cdp> viewport 768 1024
cdp> screenshot tablet-portrait.png
```

### Dark Mode

```
cdp> darkmode
cdp> screenshot dark.png
cdp> lightmode
cdp> screenshot light.png
```

## Multi-Step Screenshot Sequences

```
-- meta.yaml --
name: Login Flow Screenshots
headless: true
timeout: 60s
env:
  BASE_URL: "https://staging.example.com"

-- main.cdp --
# Step 1: Landing page
goto ${BASE_URL}
wait body
screenshot 01-landing.png
log Captured landing page

# Step 2: Login form
goto ${BASE_URL}/login
wait #login-form
screenshot 02-login-form.png

# Step 3: Filled form
fill #email test@example.com
fill #password secret
screenshot 03-form-filled.png

# Step 4: After submission
click button[type="submit"]
wait #dashboard
screenshot 04-dashboard.png
log All screenshots captured
```

## PDF Export

### In Scripts

```
goto https://example.com
wait body
pdf output.pdf
```

### In Interactive Mode

```
cdp> goto https://example.com
cdp> pdf                               # Auto-named: page-<timestamp>.pdf
cdp> pdf report.pdf                    # Custom filename
```

PDF uses Chrome's `Page.printToPDF` which produces print-layout PDFs.

## Screenshot Capture in HAR

When HAR recording is active (via `tag` command), embed screenshots directly into the HAR file as annotations:

```
-- main.cdp --
tag user-flow
goto https://example.com
wait body
capture screenshot Homepage loaded
click #login-link
wait #login-form
capture screenshot Login form visible
capture dom Login form DOM state
har output.har
```

The `capture screenshot` command takes a screenshot and embeds it (base64-encoded) in the HAR annotations. The `capture dom` command captures a DOM snapshot. Both include timestamps and descriptions.

## Shared Screenshot Helper

The project includes `examples/lib/screenshot.cdp`:

```
# screenshot.cdp - Take a screenshot with the given filename
# $ARG1 = filename
screenshot $ARG1
log Captured: $ARG1
```

Use it as a reusable command:
```
source -as snap examples/lib/screenshot.cdp
goto https://example.com
wait body
snap homepage.png
```

## Full-Page vs Viewport Screenshots

The script engine's `screenshot` command uses `chromedp.FullScreenshot` which captures the entire scrollable page. In interactive mode, `screenshot` uses `chromedp.CaptureScreenshot` which captures only the visible viewport.

To capture just the viewport in a script, use JavaScript:

```
js document.documentElement.style.overflow = 'hidden'
screenshot viewport-only.png
js document.documentElement.style.overflow = ''
```

## Best Practices

1. **Wait before capturing**: Always `wait` for content to load before screenshots.
2. **Scroll to top**: Use `js window.scrollTo(0, 0)` before screenshots for consistency.
3. **Use descriptive names**: Name screenshots by step (`01-login.png`, `02-dashboard.png`).
4. **Use output directory**: Run with `-o` to keep artifacts organized.
5. **Combine with assertions**: Verify page state before capturing.

```
goto https://example.com
wait h1
assert text h1 Example Domain
js window.scrollTo(0, 0)
screenshot verified-page.png
```
