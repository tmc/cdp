---
name: operating-cdp-cli
description: Operates the cdp CLI for browser navigation, element interaction, JavaScript evaluation, browser attachment, and interactive REPL workflows. Use when the task involves driving Chrome or Chromium with the repo's cdp command, exploring available commands, or choosing between interactive and scripted execution.
---

# Operating CDP CLI

Use this skill when the task is primarily about running or explaining the `cdp` command.

## Quick start

- Build with `go build -o cdp ./cmd/cdp`
- Use interactive mode for exploration and ad hoc debugging
- Use `cdp run` when the task should be reproducible as a script

## Use this skill for

- connecting to a local or remote browser
- navigating pages and interacting with DOM elements
- evaluating JavaScript or extracting page state
- explaining common `cdp` commands and aliases

## Live agent loop

For live browser work, prefer `cdp --mcp` and use a screenshot-first loop:

1. Observe with `screenshot`, usually with annotation enabled, and use `page_snapshot` when you need accessible element refs.
2. Act with `click` or `type_text` using `@ref` values first, CSS selectors when stable, and `coord:x,y` viewport clicks when the target is visible but not represented well in the accessibility tree.
3. Verify with another `screenshot` after every meaningful action; use `get_page_content`, `get_console`, and `get_errors` for text and failure context.
4. Refresh refs with `page_snapshot` after navigation or large DOM changes; stale refs should not be debugged manually first.
5. Use specialized tools before raw protocol calls: `list_frames`/`switch_frame`, `handle_dialog`, `upload_file`, storage/cookie tools, network log tools, screenshots, and PDFs.
6. Use `raw_cdp` only as an escape hatch for a missing tool or protocol-specific diagnosis.

Treat screenshots as the source of truth for visible state. Text snapshots are complements, not replacements, because many browser failures are layout, focus, overlay, iframe, or dialog problems.

## Read next

- Core command surface and examples: [references/basics.md](references/basics.md)
- Script format and `cdp run`: [../writing-cdp-scripts/SKILL.md](../writing-cdp-scripts/SKILL.md)
- HAR capture workflows: [../capturing-network-traffic/SKILL.md](../capturing-network-traffic/SKILL.md)
