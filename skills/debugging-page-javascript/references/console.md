# CDP Console Output & JavaScript Execution

How to execute JavaScript, capture console output, and debug pages using the cdp tool.

## JavaScript Execution

### In Scripts (txtar)

```
# Single-line JavaScript
js document.title
js window.scrollTo(0, 500)
js 'document.querySelector("#btn").click()'

# Execute JS from embedded file
jsfile helper.js

# Evaluate and capture result
extract h1                    # Gets text, sets $EXTRACTED
title                         # Gets title, sets $TITLE
url                           # Gets URL, sets $URL
```

The `js` command executes JavaScript in the page context and discards the result. The `jsfile` command runs a `.js` file from the txtar archive and prints its result when it is not null. Both run in the current document only; a later `goto` discards anything they installed.

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

### Console Commands (Interactive)

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

Inject a collector after navigation to record console output from that page
(a later `goto` discards it):

```
-- main.cdp --
goto https://example.com
jsfile console-collector.js
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

### Page Errors and Requests

Listeners installed with `js` or `jsfile` start after the page has loaded and
are discarded by the next `goto`, so they miss load-time errors and requests.
Use `cdp --console` or the MCP `get_console` and `get_errors` tools for errors,
and `tag`/`har` (see capturing-network-traffic) or MCP `get_network_log` for
requests.

### Performance Metrics

In interactive mode:
```
cdp> metrics                   # performance.timing
cdp> memory                    # performance.memory (Chrome only)
cdp> timing                    # Detailed timing JSON
cdp> paint                     # Paint timing entries
```

In scripts, return the value from a `jsfile` section; `js` discards results:
```
-- main.cdp --
goto https://example.com
wait 2s
jsfile nav-timing.js

-- nav-timing.js --
JSON.stringify(performance.getEntriesByType('navigation')[0], null, 2)
```

### Storage Inspection

```
# Interactive
cdp> localStorage              # Dump all localStorage
cdp> sessionStorage            # Dump all sessionStorage
cdp> getLocal auth_token       # Get specific key

# In scripts: a jsfile section whose body is JSON.stringify(localStorage)
jsfile dump-storage.js
```

## Common Patterns

### Fail on Console Errors

A `jsfile` that throws stops the script with exit status 1. With the
`console-collector.js` section from above:

```
-- main.cdp --
goto https://example.com
jsfile console-collector.js
wait 2s
jsfile no-console-errors.js

-- no-console-errors.js --
if (window.__console.error.length > 0) {
  throw new Error(window.__console.error.length + ' console errors');
}
```

### Extract Structured Data

```
-- main.cdp --
goto https://example.com
jsfile headings.js

-- headings.js --
JSON.stringify(Array.from(document.querySelectorAll('h2')).map(h => h.textContent))
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
