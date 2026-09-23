---
title: cdp
description: The live browser surface — attach to a running browser or launch one, then navigate, evaluate, extract, render, screenshot, or capture.
icon: terminal
---

# cdp

`cdp` is the surface you reach for when the work is exploratory: you want to
look at a page, poke it, and see what comes back. It drives a browser it
launches, a browser you are already logged into, or a browser on another
machine, and it exposes the same actions three ways: as flags for one-shot use,
as an interactive shell, and as MCP tools an agent can call.

```bash
cdp -headless -url https://example.com -render body
```

```
# Example Domain

This domain is for use in documentation examples without needing permission. Avoid use in operations.

[Learn more](https://iana.org/domains/example)
```

## The three ways to drive it

**One-shot flags.** Give a `-url` and an action; cdp does it and exits.

```bash
cdp -headless -url https://example.com -js 'document.title'
cdp -headless -url https://example.com -extract h1
cdp -headless -url https://example.com -screenshot 'full shot.png'
```

**A shell.** With no `-url` and no `-js`, cdp starts interactive. This is the
mode for working out what a page needs before writing it down:

```bash
cdp -shell
```

**MCP.** `cdp --mcp` exposes the same operations as tools for an agent. See
[Use cdp as an MCP server](/docs/plugins).

## Where the browser comes from

By default cdp *discovers* a browser: if one is already listening on the debug
port it attaches to that, otherwise it launches its own. Which path you get
therefore depends on what happens to be running, so pin it when that matters.

```bash
cdp -remote-host localhost -remote-port 9222 --list-tabs   # what is out there
cdp -remote-host localhost -remote-port 9222 -tab <id> -js 'document.title'
cdp -use-profile Default -url https://app.example.com -shell  # copy of a logged-in profile
```

`-use-profile` launches against a *copy* of a profile; `-remote-host` attaches
to the live one. The two are not equivalent — see
[How cdp works](/docs/how-cdp-works).

`-auto-discover=false` forces a launch: cdp ignores any browser already
listening, resolves an installed executable, and starts a fresh one. That is
what you want for a reproducible capture. If no browser can be found it exits 3
with `No browser executable found` rather than proceeding. Pass `-chrome-path`
to name the executable yourself.

## Output shapes

`-render` converts a selector's HTML to Markdown, which is usually what you
want for reading. `-extract` returns structured data and is the one to script
against. Note that it prints a JSON-encoded string, so a consumer unwraps it
once:

```bash
cdp -headless -url https://example.com -extract h1
```

```
"{\"text\":\"Example Domain\"}"
```

`-format json` wraps `-js` results in an array:

```bash
cdp -headless -url https://example.com -format json -js 'document.title'
```

```json
[
  "Example Domain"
]
```

`-screenshot` takes a *selector*, optionally followed by a filename — `'full
shot.png'`, not `shot.png`. This is the easy typo to make, and it fails badly:
a bare path is read as a selector, so cdp waits the full timeout for an element
named `shot.png` before giving up.

```
Error: [general_error] Screenshot failed: screenshot selector "shot.png": context deadline exceeded
```

That took 63 seconds against this tree and exits 1. `-timeout` bounds the wait:
`-timeout 10` fails in 13 seconds instead.

## When to use something else

- The work should run the same way twice → [`cdpscript`](/docs/scripting).
- The work should fail a build when it breaks → [`cdpscripttest`](/docs/cdpscripttest).
- You only want the bytes a page fetched → [`chrome-to-har`](/docs/capturing-traffic).
- You only want the rendered page → [`churl`](/docs/churl).

## Next steps

- [Quickstart](/docs/quickstart) — install and run the first command.
- [Capture network traffic](/docs/capturing-traffic) — recording what a page fetches.
- [Use cdp as an MCP server](/docs/plugins) — the agent-facing surface.
- [Command reference](/docs/commands) — `go doc ./cmd/cdp` for every flag.
- [Known issues](/docs/known-issues) — what is broken today.
