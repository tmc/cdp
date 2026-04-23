---
name: working-with-iframes
description: Works with frames and iframes through the cdp MCP server. Use when the task involves listing frames, switching execution context, or validating content that lives inside embedded documents.
---

# Working With Iframes

Use this skill when page state or interactions depend on an iframe rather than the top document.

## Quick start

- Use `list_frames` to see frame IDs, names, URLs, and parents
- Use `switch_frame` with `main`, a frame name, an index, or an iframe selector
- After switching, use normal page tools such as `page_snapshot`, `click`, `type_text`, and `get_page_content`

## Use this skill for

- finding the right iframe before interacting with it
- switching back and forth between the main document and child frames
- checking whether embedded content is same-page UI or a separate frame target

## Read next

- Tool details and current limits: [references/iframes.md](references/iframes.md)
- Script format reference: [../writing-cdp-scripts/references/script-format.md](../writing-cdp-scripts/references/script-format.md)
- General MCP browser control: [../operating-cdp-cli/SKILL.md](../operating-cdp-cli/SKILL.md)
