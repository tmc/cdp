# CDP Console Output & JavaScript Execution

How to execute JavaScript, capture console output, and debug pages using the cdp tool.

## JavaScript Execution

### In Scripts (txtar)

```
# Single-line JavaScript
js document.title
js window.scrollTo(0, 500)
js document.querySelector('#btn').click()

# Execute JS from embedded file
jsfile helper.js

# Evaluate and capture result
extract h1                    # Gets text, sets $EXTRACTED
title                         # Gets title, sets $TITLE
url                           # Gets URL, sets $URL
```

The `js` command executes JavaScript in the page context. The `jsfile` command loads and executes a `.js` file from the txtar archive's embedded files.

### In Interactive Mode

```
cdp> eval document.title
cdp> eval window.location.href
cdp> eval document.querySelectorAll('a').length
cdp> js console.log('hello from cdp')
```

The `eval` command (aliases: `js`, `exec`) evaluates an expression and prints the result.

## Console Message Capture

### Enabling Console Monitoring (Interactive)

In interactive mode, enable the Runtime domain to see console messages:

```
cdp> console
```

This sends `Runtime.enable {}` which starts reporting console API calls.

### Console Commands

```
log Hello World                    # console.log('Hello World')
error Something went wrong         # console.error(...)
warn Deprecated feature            # console.warn(...)
clear_console                      # console.clear()
```

### Capturing Console Output in Scripts

Use `jsfile` for complex logic - return values are printed to stdout:

```
-- main.cdp --
goto https://example.com
jsfile check-page.js

-- check-page.js --
(function() {
  var errors = [];
  document.querySelectorAll('img').forEach(function(img) {
    if (!img.complete || img.naturalWidth === 0) {
      errors.push('Broken image: ' + img.src);
    }
  });
  return errors.length ? errors.join('\n') : 'No issues found';
})();
```

### Collecting Console Messages via JavaScript

Inject a collector script to capture all console output during a session:

```
-- main.cdp --
jsfile console-collector.js
goto https://example.com
wait 2s
jsfile console-dump.js

-- console-collector.js --
(function() {
  window.__console = { log: [], warn: [], error: [] };
  ['log', 'warn', 'error'].forEach(function(level) {
    var orig = console[level];
    console[level] = function() {
      window.__console[level].push({
        time: new Date().toISOString(),
        args: Array.from(arguments).map(function(a) {
          try { return JSON.stringify(a); }
          catch(e) { return String(a); }
        })
      });
      orig.apply(console, arguments);
    };
  });
})();

-- console-dump.js --
(function() {
  var c = window.__console || { log: [], warn: [], error: [] };
  if (c.error.length > 0) {
    return 'ERRORS FOUND:\n' + c.error.map(function(e) { return e.args.join(' '); }).join('\n');
  }
  return 'Console: ' + c.log.length + ' log, ' + c.warn.length + ' warn, ' + c.error.length + ' error';
})();
```

## Debugging Techniques

### Check for Page Errors

```
-- main.cdp --
js window.__errors = []; window.addEventListener('error', function(e) { window.__errors.push(e.message); });
goto https://example.com
wait 2s
js window.__errors.length > 0 ? 'ERRORS: ' + window.__errors.join('; ') : 'No JS errors'
```

### Network Request Inspection via JS

```
-- main.cdp --
js window.__fetches = []; var _fetch = window.fetch; window.fetch = function(url, opts) { window.__fetches.push({url: String(url), method: (opts||{}).method || 'GET', time: Date.now()}); return _fetch.apply(this, arguments); };
goto https://example.com
wait 3s
js JSON.stringify(window.__fetches.map(function(f) { return f.method + ' ' + f.url; }), null, 2)
```

### Performance Metrics

In interactive mode:
```
cdp> metrics                   # performance.timing
cdp> memory                    # performance.memory (Chrome only)
cdp> timing                    # Detailed timing JSON
cdp> paint                     # Paint timing entries
```

In scripts:
```
goto https://example.com
wait 2s
js JSON.stringify(performance.getEntriesByType('navigation')[0], null, 2)
js JSON.stringify(performance.getEntriesByType('resource').map(function(r) { return {name: r.name.split('/').pop(), duration: Math.round(r.duration)}; }), null, 2)
```

### Storage Inspection

```
# Interactive
cdp> localStorage              # Dump all localStorage
cdp> sessionStorage            # Dump all sessionStorage
cdp> getLocal auth_token       # Get specific key

# In scripts
js JSON.stringify(localStorage)
```

## Common Patterns

### Assert No Console Errors

```
-- main.cdp --
jsfile console-collector.js
goto https://example.com
wait 2s
js window.__console.error.length === 0 ? 'PASS: no console errors' : 'FAIL: ' + window.__console.error.length + ' errors'
```

### Extract Structured Data

```
goto https://example.com
js JSON.stringify(Array.from(document.querySelectorAll('h2')).map(function(h) { return h.textContent; }))
```

### Debug with Accessibility Tree

```
goto https://example.com
snapshot -i --compact
# Output shows interactive elements with refs:
#   - button "Submit" [ref=e1]
#   - textbox "Email" [ref=e2]
click @e1
fill @e2 search term
```
