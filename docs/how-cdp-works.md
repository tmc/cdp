---
title: How cdp works
description: Why this repository has several browser commands, how capture and authentication actually work, and which design choices are deliberate.
icon: diagram-project
---

# How cdp works

This page explains the model behind the tools. It is background reading, not a
set of steps — for those, see the [task guides](/docs/index).

## Everything drives a real browser

The Chrome DevTools Protocol is the interface Chrome's own developer tools use.
It is a bidirectional JSON protocol over a WebSocket, organized into domains
such as `Page`, `Network`, `DOM`, and `Runtime`, each with methods and events.

Every command here speaks that protocol to a browser you already have
installed. Nothing reimplements HTTP or HTML. That is the whole reason a page
built entirely by JavaScript behaves the same for these tools as for a person
with a browser window open, and it is also why these tools cannot be as fast or
as light as `curl`.

## Why there are several commands

They differ in who is in control of the session.

- **`chrome-to-har`** owns the whole session: it launches, navigates, waits,
  writes a HAR, exits. Best when the capture is fully described by a URL and
  some waiting rules.
- **`churl`** owns the session too, but the deliverable is the page, not the
  traffic. It is shaped like `curl`, and fetches one URL per run with
  JavaScript executed. It does not implement `wget`-style recursion or
  mirroring.
- **`cdp`** does not assume it owns the session. It can attach to a browser you
  logged into by hand, keep running while you click, and expose the browser to
  an agent over MCP. This is what you reach for when getting to the interesting
  traffic requires a human or an agent doing something first.
- **`cdpscript`** replays a fixed sequence, and **`cdpscripttest`** runs those
  sequences as tests.
- **`chdb`** and **`ndp`** are debuggers rather than capture tools — breakpoints
  and heap snapshots for pages and for Node.js respectively.

When two of them could do the job, prefer the one that owns less of the
session: a `chrome-to-har` invocation that works is easier to rerun than an
interactive `cdp` session that worked once.

## Capture is a recording of protocol events

A HAR is assembled from `Network` domain events as they arrive, not by proxying
traffic. Two consequences follow, and both surprise people:

**Capture stops when the tool stops.** A page that fetches data a second after
load will not appear in the archive unless the tool was still watching. This is
the single most common reason an expected request is missing, and it is why the
waiting flags exist rather than being a nicety. See
[Capture network traffic](/docs/capturing-traffic) for the demonstration.

**Bodies are not free.** Response bodies must be requested per response and
held, so full-body capture is a deliberate mode (`-full-capture`) rather than
the default, and `-max-body-bytes` exists to bound it.

## Authentication is about which profile the browser uses

A browser launched with an empty profile has no cookies, so it has no access to
anything behind a login. There are two ways to fix that and they are not
equivalent:

Copying a profile (`-use-profile`) duplicates the profile directory, cookies
included, into a temporary directory and launches against the copy. Your real
profile is never written to, which is safe, but the copy is a point-in-time
snapshot: a token refreshed since the copy, or a site that pins a session to a
device, may still reject it.

Attaching to a running browser (`-remote-host`) keeps the real, live session.
Nothing is copied. This is what works for sites with strict sign-in, at the
cost of needing a browser started with remote debugging enabled and logged in
by hand first.

## Captures contain credentials

A capture of an authenticated session contains the headers that made it
authenticated. That is what makes it useful for debugging and what makes it
dangerous to attach to a bug report. Redaction of secrets in HAR and source
output is therefore on by default, and turning it off is an explicit
`-no-scrub`.

## Scripts deliberately have no loops

The script language has sequencing, failure expectations (`!`, `?`), and
conditional guards (`[cond]`) — but no loops, no retries, and no variables
beyond arguments and the environment.

This is a deliberate limit, not an unfinished feature. A script that needs real
control flow is better expressed in the shell or in the Go test that calls it,
where there are already debuggers, libraries, and a type checker. Adding a
second control-flow language inside the archive would mean maintaining a
programming language as a side effect of maintaining a browser tool.

## What is stable and what is not

The module has no tagged release. `go install …@latest` resolves to a
pseudo-version of the default branch, so an upgrade can change behavior without
a version number changing to warn you. Pin a specific pseudo-version if you
depend on this in automation.

Within that, the surfaces differ in how settled they are:

- **Settled:** single-URL capture on `chrome-to-har` and `cdp`, the fetch and
  extraction flags on `churl`, and the script command set. These have tests,
  and every flag is covered by a documentation test.
- **Moving:** the MCP tool surface, including the inspection tools behind
  `-enable-inspect`. Tool names and arguments have changed and may change
  again.
- **Known-broken in places:** see [Known issues](/docs/known-issues), which
  lists open bugs with workarounds rather than pretending they are fixed.

Most Go packages are `internal/` precisely so that no stability guarantee is
needed. The exceptions are deliberate: `cdpscript` and `cdpscripttest` are
importable because running and testing scripts from another module is the
point. They are still pre-1.0 — pin a pseudo-version rather than tracking the
branch. See [Adopting cdpscripttest](/docs/cdpscripttest/adopting) for what the
dependency pulls in.

## Documentation lives with the code

Each command's flags are documented in its own `doc.go`, and a test walks the
flags the command actually registers and fails if any is missing from that
file. The guides in this directory therefore do not repeat flag tables; they
explain workflows and link to `go doc`. When a guide and `go doc` disagree,
`go doc` is the one that is checked by a test.

## Next steps

- [Capture network traffic](/docs/capturing-traffic) — the waiting and authentication
  ideas above, applied.
- [cdpscript](/docs/scripting) — the command set and the debugging loop.
- [Command reference](/docs/commands) — where each command's flags are documented.
