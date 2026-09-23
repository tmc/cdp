---
title: Troubleshooting
description: Diagnose the common failures — missing requests, browser not found, script errors, and stderr noise that is not an error.
icon: circle-question
---

# Troubleshooting

Every symptom below was reproduced against this repository's tools. For bugs
with no workaround yet, see [Known issues](/docs/known-issues).

## A request I expected is missing from the capture

**Cause:** the tool stopped watching before the request happened. Anything a
page fetches after load — an XHR, a lazily loaded image, an analytics beacon —
needs the capture to still be running.

**Fix:** wait for something that only exists once the page is ready, with
`-wait-for '<selector>'`, or `-wait-stable` when you do not know a selector.

[Capture network traffic](/docs/capturing-traffic) shows the same page captured
with and without waiting, and what each produces.

## Cannot navigate to invalid URL

A script's arguments split on whitespace, so a URL containing a literal space
is cut at the space:

```
cdpscript: greet.txtar:1: goto /index.html: chrome navigation error:
navigate to URL: Cannot navigate to invalid URL (-32000)
```

Percent-encode the URL — `%20` for spaces, `%3C` and `%3E` for angle brackets:

```
goto data:text/html,%3Ch1%3EHello,%20cdp%3C/h1%3E
```

The same truncation silently loads a *different* page when the remainder still
parses as a URL, which shows up as an assertion failing against text you can
see in the browser. If an assertion fails on text that looks correct, check the
URL for spaces first.

## A script asserts against text that looks right but fails

```
assertion failed: text "Hello," does not contain "Hello, cdp"
```

The quoted actual value is what the page really contained. `"Hello,"` ending at
the comma is the truncation above. If the actual value is genuinely different,
the page changed; run without `-headless` to watch it.

## Exit status is 3, not 1

That is deliberate. `cdpscript` exits 2 for a usage error, 3 for a failed
assertion, and 1 for anything else, so a shell caller can tell "the page was
wrong" from "the script was wrong":

```bash
cdpscript -headless greet.txtar; echo $?
```

## The browser prints errors even though the run succeeded

Chromium writes to stderr on startup regardless of outcome:

```
DevTools listening on ws://127.0.0.1:59062/devtools/browser/f676ffa2-…
[…:ERROR:chrome/browser/profiles/profile_attributes_storage.cc:1036] Failed to PNG encode the image.
```

Neither line indicates failure. Check the exit status, and redirect stderr with
`2>/dev/null` if it is in the way.

## No browser found, or the wrong one is used

```bash
cdp -list-browsers
```

lists what was discovered and which is running. To pin one, pass
`-chrome-path`, or set `CHROME_PATH`. By default `cdp` prefers a browser that
is already running; `-auto-discover=false` stops that and launches a fresh one,
which is what you want for a reproducible capture.

## cdp attach finds nothing

```
No attachable CDP targets found on localhost.
Probe diagnostics:
  localhost:9222: devtools endpoint not reachable
  …
Start a browser with remote debugging enabled, then rerun attach:
  '/Applications/Brave Browser.app/…/Brave Browser' --remote-debugging-port=9222 --user-data-dir="$(mktemp -d)"
  cdp attach --port 9222
```

The command prints the exact invocation for each browser it found, including
the `--user-data-dir` — a browser already running without remote debugging
cannot be attached to retroactively.

## The -js flag prints null instead of my value

`-js` prints the evaluation result, and `console.log(...)` evaluates to
`undefined`, rendered as `null`:

```bash
cdp -headless -url '…' -js 'console.log(document.title)'
```

```
Executed 1 JavaScript script(s) in new Chrome instance
null
```

To get a value out, use `-extract` for element content or `-render` for the
page as Markdown. To see the page's console output, use `-console`.

## The -har flag requires -url

`-har` writes one archive for one bounded navigation, so it needs to know what
to navigate to. For a session you drive yourself, stream instead:

```bash
cdp -harl -harl-file session.har.jsonl -shell
```

## An authenticated page behaves as though logged out

A browser `cdp` launches has an empty profile. `-use-profile` copies a
profile's cookies but is a point-in-time snapshot; sites that refresh tokens or
pin sessions to a device may still reject it. Attach to a browser you logged
into by hand instead:

```bash
cdp -remote-host localhost -remote-port 9222 -shell
```

See [How cdp works](/docs/how-cdp-works) for why the two differ.

## Browser tests fail only in a full run

For package-level browser contention, run with `-p 1`. For `cdpscripttest`
fixtures, add `-parallel 1` to serialize its subtests:

```bash
go test -p 1 ./...
go test -p 1 -parallel 1 -tags cdp ./cdpscripttest
```

Go tests different packages in parallel by default, and several packages each
launching a browser contend for profile directories and ports. Failures that
appear only at full parallelism are usually this, not a regression. See
[Testing](/docs/testing).

## Next steps

- [Known issues](/docs/known-issues) — open bugs and their workarounds.
- [How cdp works](/docs/how-cdp-works) — the model behind most of these symptoms.
