---
name: handling-dialogs
description: Handles JavaScript dialogs through the cdp MCP server. Use when the task involves alert, confirm, prompt, or beforeunload flows that block page automation until the dialog is answered.
---

# Handling Dialogs

Use this skill when a page action opens a JavaScript dialog and the browser is waiting for accept or dismiss.

## Quick start

- Use `get_dialogs` to inspect the pending dialog and recent history
- Use `handle_dialog` with `accept` and optional `prompt_text`
- Re-check page state with `page_snapshot`, `check_element`, or `get_page_content`

## Use this skill for

- accepting or dismissing `alert`, `confirm`, `prompt`, and `beforeunload`
- verifying dialog text before responding
- unblocking flows that stall after a click or navigation

## Read next

- Tool details and limits: [references/dialogs.md](references/dialogs.md)
- Script format reference: [../writing-cdp-scripts/references/script-format.md](../writing-cdp-scripts/references/script-format.md)
- General MCP browser control: [../operating-cdp-cli/SKILL.md](../operating-cdp-cli/SKILL.md)
