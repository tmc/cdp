---
title: Command reference
description: Where every command's flags and subcommands are documented, and how that documentation is kept in sync with the code.
icon: terminal
---

# Command reference

Flag-level reference lives in each command's `doc.go` rather than here, so it
cannot drift from the code it describes. Read it with `go doc`, which needs no
network and works against the version you have checked out.

| Command | Reference | Covers |
| --- | --- | --- |
| `cdp` | `go doc ./cmd/cdp` | Automation, capture, extraction, MCP, the interactive shell |
| `chrome-to-har` | `go doc ./cmd/chrome-to-har` | HAR capture, filtering, blocking, waiting, differential capture |
| `churl` | `go doc ./cmd/churl` | Fetching, output formats, WebSockets (recursion and mirroring are not implemented) |
| `chdb` | `go doc ./cmd/chdb` | Page debugging: breakpoints, DOM, network, heap, tracing |
| `ndp` | `go doc ./cmd/ndp` | Node.js and V8 debugging |
| `cdpscript` | `go doc ./cmd/cdpscript` | Running one script |
| `cdpscripttest` | `go doc ./cmd/cdpscripttest` | Running scripts as tests |
| `native-host` | `go doc ./cmd/native-host` | The Chrome native messaging host |

Library packages:

| Package | Reference | Covers |
| --- | --- | --- |
| `cdpscript` | `go doc ./cdpscript` | The script command set and syntax |
| `cdpscripttest` | `go doc ./cdpscripttest` | Running scripts from Go tests |

The two cobra-based commands, `chdb` and `ndp`, also document themselves at the
command line:

```bash
chdb help
chdb help <command>
ndp help
```

## How this stays accurate

Each command has a test that enumerates the flags the command actually
registers — by walking the source for flag registrations, or the command tree
for the cobra commands — and fails if any is absent from its `doc.go`:

```bash
go test ./cmd/... -run TestDocCovers
```

Adding a flag without documenting it fails `go test ./cmd/...`. It does not
break `go build`, and the repository has no CI configuration that runs it for
you. This is why the guides
in this directory link to `go doc` instead of reproducing flag tables: a table
here would have no such gate.

## Script format

The script command set is documented in the `cdpscript` package. The format
reference, including the txtar layout and the full command list, is in
[skills/writing-cdp-scripts/references/script-format.md](https://github.com/tmc/cdp/blob/main/skills/writing-cdp-scripts/references/script-format.md).

## Next steps

- [cdpscript](/docs/scripting) — using the script commands in practice.
- [Troubleshooting](/docs/troubleshooting) — when a documented flag does not behave
  as expected.
