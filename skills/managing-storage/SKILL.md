---
name: managing-storage
description: Reads and mutates browser storage through the cdp MCP server. Use when the task involves localStorage, sessionStorage, cookies, or saving and restoring browser state across runs.
---

# Managing Storage

Use this skill when page behavior depends on browser-side state rather than visible DOM alone.

## Quick start

- Use `get_storage` and `get_cookies` to inspect state
- Use `set_storage`, `clear_storage`, or `set_cookie` to seed state
- Use `save_state` and `load_state` when the whole browser state should round-trip through disk

## Use this skill for

- checking login or feature-flag state
- seeding storage before navigation or assertions
- restoring a browser session for repeated workflows
- clearing state between runs

## Read next

- Tool details and script limits: [references/storage.md](references/storage.md)
- Script format reference: [../writing-cdp-scripts/references/script-format.md](../writing-cdp-scripts/references/script-format.md)
- Page-side JavaScript patterns: [../debugging-page-javascript/SKILL.md](../debugging-page-javascript/SKILL.md)
