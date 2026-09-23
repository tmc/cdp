---
title: cdp documentation
description: Drive Chrome from the command line to capture network traffic, extract rendered pages, and write browser automation that runs as tests.
icon: book-open
---

# cdp documentation

This repository is built on the Chrome DevTools Protocol. Everything here
drives a real browser, so JavaScript runs, single-page applications render, and
authenticated sessions work.

There are three entry points, and which one you want depends on how durable the
work needs to be.

## `cdp` — generalized browser scripting

The interactive and agent-driven surface. Attach to a browser you are already
logged into, drive it from a prompt or over MCP, capture traffic while you
click, extract or render what you find. Reach for it when the work is
exploratory, or when getting to the interesting page needs a human or an agent
first.

```bash
cdp -headless -url https://example.com -render body
```

→ [The `cdp` guide](/docs/cdp) · [Quickstart](/docs/quickstart) ·
[Capture traffic](/docs/capturing-traffic) · [MCP server](/docs/plugins)

## `cdpscript` — Unix-like tools for browser operations

The durable command-line surface. A script is a txtar archive that runs the
same way every time, takes arguments, writes artifacts, and exits with a status
a shell can branch on. Give it a shebang line and `chmod +x` and it *is* a Unix
tool — one you can put on `$PATH`, in a pipeline, or in a Makefile.

```
#!/usr/bin/env cdpscript
# greet - print the title of a page

-- main.cdp --
goto $ARG1
title
```

```bash
./greet.cdpscript https://example.com    # or: cdpscript -o ./artifacts greet.cdpscript
```

→ [cdpscript](/docs/scripting) ·
[Tutorial](/docs/tutorial-first-script)

## `cdpscripttest` — Go-native browser testing

The testing surface. The same archives run under `go test`, so a script that
reproduces a bug becomes the regression test for it without being rewritten.
Fixtures travel inside the archive, and browser tests sit behind a build tag so
an ordinary `go test ./...` needs no browser.

```go
cdpscripttest.RunCDPScript(t.Context(), "greet.txtar", opts)
```

→ [The `cdpscripttest` guide](/docs/cdpscripttest) ·
[Use it from your module](/docs/cdpscripttest/adopting) ·
[Agentic workflows](/docs/cdpscripttest/agentic) ·
[Testing this repository](/docs/testing)

## I want to…

| Goal | Start here |
| --- | --- |
| Install the tools and see one work | [Quickstart](/docs/quickstart) |
| Learn the idioms by building something small | [Tutorial: your first script](/docs/tutorial-first-script) |
| Understand how these tools fit together | [How cdp works](/docs/how-cdp-works) |
| Record network traffic, including behind a login | [Capture network traffic](/docs/capturing-traffic) |
| Find what changed between two page loads | [Differential capture](/docs/differential-capture) |
| Automate a browser task repeatably | [cdpscript](/docs/scripting) |
| Let an AI agent drive the browser | [Use cdp as an MCP server](/docs/plugins) |
| Debug an Electron application | [Electron debugging](/docs/electron-mcp) |
| Fetch through a proxy | [churl behind a proxy](/docs/churl-proxy) |
| Fix something that is going wrong | [Troubleshooting](/docs/troubleshooting) |
| Work on this repository | [Testing](/docs/testing) |
| Look up a flag or command | [Command reference](/docs/commands) |
| Read about one tool end to end | [cdp](/docs/cdp) · [cdpscript](/docs/scripting) · [cdpscripttest](/docs/cdpscripttest) · [churl](/docs/churl) |

## The other commands

Focused tools alongside the three entry points above. (`native-host` also
ships, but Chrome starts it — see [Command reference](/docs/commands).)

| Command | Use it for |
| --- | --- |
| `chrome-to-har` | One-shot HAR capture, plus differential capture |
| [`churl`](/docs/churl) | Fetching a URL with JavaScript executed |
| `chdb` | Debugging a page: breakpoints, DOM, heap, tracing |
| `ndp` | Debugging Node.js and V8 |

`chdb` and `ndp` have no guide here: they are documented by their own `--help`
and `go doc`, and no page on this site has verified their workflows. Start with
`chdb help` or `ndp help`.

Every command documents its own flags, checked by a test that fails when a flag
is added without being documented:

```bash
go doc ./cmd/cdp
```

See [Command reference](/docs/commands) for the full list.

## Next steps

- [Quickstart](/docs/quickstart) — install and see a tool work.
- [Tutorial: your first script](/docs/tutorial-first-script) — the guided
  build, once the quickstart runs.
- [How cdp works](/docs/how-cdp-works) — the model behind the tools, and what
  is stable.
- [Troubleshooting](/docs/troubleshooting) — when a command misbehaves.

Already installed and looking for a specific task? Pick a row from the table
above.
