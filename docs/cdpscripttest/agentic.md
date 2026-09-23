---
title: Agentic workflows
description: Why the fixture format suits agent-driven product development, and the loop that turns live browser exploration into a committed regression suite.
icon: robot
---

# Agentic workflows

An agent building a web product has a verification problem. It can change code
confidently, but "does the page still work?" is a question about pixels, focus,
overlays, and network timing that no unit test answers. The usual outcome is an
agent that reports success because the code compiled.

The three tools in this repository close that loop, and `cdpscripttest` is the
step that makes the closure durable.

## The loop

1. **Explore live.** The agent drives a real browser through `cdp --mcp`,
   screenshot-first: observe, act, verify, re-observe. See
   [cdp](/docs/cdp). Screenshots are the source of truth here — most browser
   failures are layout, focus, overlay, iframe, or dialog problems that a text
   snapshot cannot show.
2. **Crystallize.** The sequence that worked becomes a txtar script with
   assertions and an exit code — see [cdpscript](/docs/scripting). This is the
   step that converts a session into an artifact.
3. **Commit.** The same archive runs under `go test` through `RunCDPScript`, or
   as a native fixture. Now it fails the build when the behavior regresses,
   rather than when someone remembers to check.

Each step narrows what the next one has to trust. The agent is not asked to
remember that the account page needs a wait before its text assertion; the
fixture encodes it.

## Why the format holds up

**Fixtures are text an agent can write and diff.** A txtar archive is a
plain-text file with its assertions inline. An agent can generate one, read a
failure that cites `file:line`, and edit it — none of which is true of a
screenshot-comparison tool driven by a GUI.

**The failure message is the diagnosis.** `assert text h1 NotThePage: assertion
failed: text "Example Domain" does not contain "NotThePage"` says what was
expected, what was found, and where. Compare with a bare non-zero exit, which
sends the agent back to re-run the whole flow to learn anything.

**Exit codes distinguish kinds of failure.** `3` is an assertion that failed —
the product is wrong. `1` is a runtime error — the script is wrong. `2` is
usage. An agent can branch on that instead of guessing.

**Reports are readable evidence.** A run emits Markdown pairing each script's
source with its command log and images
([Reports and artifacts](/docs/cdpscripttest/reports)), so an agent that did not
run the test can still read what happened. This matters when one agent
implements and another reviews.

**Custom commands compress the vocabulary.** A single `sign-in` command instead
of six lines of navigation means the agent writes fixtures about the feature
rather than about the login flow
([Extending the engine](/docs/cdpscripttest/extending)).

## What this looks like at size

A suite of this shape scales to dozens of fixtures over an application's own
dev server, started by the suite itself on a free port, with visual baselines
committed for the pages whose layout matters. It needs nothing on the machine
but a browser.

The shape that makes that work: fixtures own their server, share one browser
with a fresh tab each, reset state between scripts, and mask dynamic content
rather than loosening thresholds.

## Where it is weak

Worth knowing before you lean on it:

- **A fixture with no assertion always passes.** So does a `-run` pattern that
  matches nothing, and a test file whose build tag was omitted. Check `-v`
  output for named subtests before believing a green run.
- **The first `screenshot-compare` cannot fail** — the capture becomes the
  baseline. A visual test added today proves nothing until it has a committed
  baseline someone looked at.
- **Timing is the dominant failure mode.** Most flakes are a missing wait, not a
  broken product. An agent that responds to a flake by raising a timeout or a
  diff threshold is disabling the test; the correct response is a wait on the
  thing that was actually missing.
- **The script language has no loops or retries, deliberately.** Control flow
  belongs in the Go test or the shell. An agent that wants a retry loop is
  usually papering over a missing wait.
- **Pixel comparison is machine-sensitive.** Fonts and headed-versus-headless
  rendering differ, so baselines captured locally can fail in CI. Prefer
  component captures over full pages.

## Next steps

- [cdp](/docs/cdp) — the live loop and the MCP tool surface.
- [cdpscript](/docs/scripting) — crystallizing a session into an archive.
- [Extending the engine](/docs/cdpscripttest/extending) — commands for your app.
- [Adopting cdpscripttest](/docs/cdpscripttest/adopting) — using it from your module.
