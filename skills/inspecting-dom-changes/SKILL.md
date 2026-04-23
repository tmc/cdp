---
name: inspecting-dom-changes
description: Captures and compares DOM snapshots through the cdp MCP server. Use when the task involves proving what changed after an interaction, navigation, or script execution.
---

# Inspecting DOM Changes

Use this skill when the important output is a before-and-after diff rather than a single page snapshot.

## Quick start

- Capture named snapshots with `snapshot_dom`
- Compare them with `dom_diff`
- Use `list_dom_snapshots` when you need to inspect what has been saved

## Use this skill for

- verifying that a click or form submission changed the page
- diffing content before and after client-side rendering
- keeping a compact record of structural UI changes

## Read next

- Tool details and limits: [references/dom-changes.md](references/dom-changes.md)
- Script format reference: [../writing-cdp-scripts/references/script-format.md](../writing-cdp-scripts/references/script-format.md)
- General MCP browser control: [../operating-cdp-cli/SKILL.md](../operating-cdp-cli/SKILL.md)
