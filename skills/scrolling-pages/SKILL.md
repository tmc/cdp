---
name: scrolling-pages
description: Scrolls pages and elements through the cdp MCP server. Use when the task involves bringing lazy-loaded content into view, centering a target element, or driving directional scroll behavior during exploration.
---

# Scrolling Pages

Use this skill when the page must move before other tools can observe or interact with the right content.

## Quick start

- Use `scroll` with `direction` and optional `distance` for page-level movement
- Use `scroll` with `selector` or `@ref` to bring an element into view
- Re-check the viewport with `page_snapshot`, `get_page_content`, or `screenshot`

## Use this skill for

- loading content that appears only after scrolling
- centering an element before click or hover
- moving horizontally or vertically through large containers

## Read next

- Tool details and script limits: [references/scrolling.md](references/scrolling.md)
- Script format reference: [../writing-cdp-scripts/references/script-format.md](../writing-cdp-scripts/references/script-format.md)
- Visual capture after scrolling: [../capturing-page-artifacts/SKILL.md](../capturing-page-artifacts/SKILL.md)
