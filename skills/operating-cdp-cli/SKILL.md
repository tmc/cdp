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

## Read next

- Core command surface and examples: [references/basics.md](references/basics.md)
- Script format and `cdp run`: [../writing-cdp-scripts/SKILL.md](../writing-cdp-scripts/SKILL.md)
- HAR capture workflows: [../capturing-network-traffic/SKILL.md](../capturing-network-traffic/SKILL.md)
