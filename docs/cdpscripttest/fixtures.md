---
title: Writing fixtures
description: The two script dialects side by side, txtar structure, commands, conditions, and the environment variables a fixture can read.
icon: file-code
---

# Writing fixtures

A fixture is a txtar archive either way. What differs is which runtime reads it,
and that choice decides the whole vocabulary.

Two dialects, and four Go entry points. The dialect decides the vocabulary; the
entry point decides the orchestration:

| Entry point | Dialect | Use for |
|---|---|---|
| `Test` | native | a glob of independent fixtures, run as parallel subtests |
| `RunFiles` | native | files run **sequentially**, with per-file results and reports |
| `Run` | native | one script against a `State` you already built |
| `RunCDPScript` | cdpscript | a `main.cdp` archive, through the real cdpscript runtime |

## Choosing a dialect

|  | Native fixture | cdpscript archive |
|---|---|---|
| Runner | `Test`, `RunFiles`, or `Run` | `cdpscripttest.RunCDPScript(ctx, path, opts)` |
| Body lives in | the txtar comment section | a `-- main.cdp --` section |
| Navigate | `navigate /path` (BASE_URL-relative) | `goto <url>` |
| Wait | `wait-visible <sel>` (alias `wait`) | `wait <sel>` or `wait 2s` |
| Evaluate | `eval <js>` (alias `js`), `evalfile` | `js <code>`, `jsfile <path>` |
| Assert | `stdout '<regexp>'` after a command | `assert exists\|text\|visible\|status\|response\|header` |
| Extras | `screenshot-compare`, `rtc-*`, `network-emulate`, `distill`, `markdown` | the cdpscript command set |

Pick `RunCDPScript` when the archive should also run as a command-line tool —
that path executes the same runtime as `cdpscript` and `cdp run`, so the test
cannot drift from the tool. Pick a native fixture when you want the richer
assertion and comparison surface.

The tell is a `main.cdp` section. Handed such an archive, the native runner
finds an empty comment section, runs nothing, and reports success.

## A native fixture

```text
# Verify the result element renders after setup.
navigate /app
evalfile setup.js
text '#result'
stdout 'ok'

# Only meaningful with a visible window.
[!headless] screenshot-compare --threshold 3 '#main' main.png

-- setup.js --
document.getElementById('result').textContent = 'ok';
```

Every line is a comment (`#`), a condition guard, or a command. `!` before a
command expects failure. `stdout`/`stderr` match the previous command's output
as a regexp. `cmp` compares files, which pairs with txtar sections.

Named sections are extracted into the test's working directory before the
script runs, so a fixture carries its own HTML, JS, and JSON without external
files.

## A cdpscript archive

```text
#!/usr/bin/env cdpscript
# Check the account page renders a session list.
#
# Usage:
#   FIXTURE_BASE_URL=http://127.0.0.1:8090 cdpscript account.txtar

-- main.cdp --
goto ${FIXTURE_BASE_URL}/account
wait '#sessions'
assert exists '#sessions'
assert text '#sessions' Session
```

See [cdpscript](/docs/scripting) for the full command set and the Unix tool
contract these get when run from the command line.

## Conditions

Boolean: `headless`, `rtc`, `short`, `verbose`.

Prefix conditions take a colon- or space-separated argument:

```text
[exec:html2md] markdown '#main'
[element:#status] text '#status'
[title:*Dashboard*] screenshot dash.png
[stdout:ok] echo matched
[rtc-state connected] rtc-ice
```

All negate with `!`:

```text
[!headless] skip 'requires headed mode'
[!rtc] skip 'WebRTC not injected'
```

`skip` ends the script without failing; `stop` ends it early without failing or
skipping. Use `skip` for "this environment cannot run this," and `stop` for
"nothing further to check."

## Waiting is the whole game

The most common fixture failure is asserting on text before the page has
produced it. A run that captures `Loading...` where it expected content is not
a flaky test — it is a missing wait.

```text
navigate /account
wait-visible '#sessions'      # not optional
text '#sessions'
stdout '[Ss]ession'
```

`timeout <duration>` sets the default wait timeout for the script (default
10s); `--timeout` on an individual `wait-visible` overrides it for that line.

## Environment

| Variable | Meaning |
|---|---|
| `WORK` | the test's working directory |
| `TMPDIR` | temporary directory, cleaned up after the test |
| `BASE_URL` | the base URL passed to `Test`; read-only, change with `set-base-url` |
| `SCREENSHOT_DIR` | override the screenshot output directory |

For a `RunCDPScript` archive, `CDPScriptRunOptions.Env` is how a fixture server's
address reaches the script, and `Args` becomes `ARG1..ARGN` with `ARGC`.

## Next steps

- [Visual testing](/docs/cdpscripttest/visual) — screenshots and baselines.
- [Extending the engine](/docs/cdpscripttest/extending) — commands of your own.
- `go doc ./cdpscripttest` — every command, condition, and flag.
