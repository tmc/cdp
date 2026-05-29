---
name: working-with-shadow-dom
description: Works with open and closed shadow DOM using cdp MCP tools, JavaScript, and viewport coordinate input. Use when an element is visible but selectors, accessibility refs, or page snapshots do not expose the actual control.
---

# Working With Shadow DOM

Use this skill when page state or interaction depends on a web component or
shadow root.

## Quick start

- Use screenshots first to confirm the target is visible.
- For open shadow roots, use `evaluate` to inspect through `element.shadowRoot`.
- For closed shadow roots, use `click coord:x,y` or other viewport-coordinate
  input; JavaScript selectors cannot traverse the closed root.
- After coordinate input, verify with another screenshot or visible page state.

## Use this skill for

- visible controls hidden behind web components
- custom elements whose inner DOM is not in `page_snapshot`
- deciding between JavaScript inspection and compositor-level coordinate input

## Read next

- Tool details and limits: [references/shadow-dom.md](references/shadow-dom.md)
- Iframe and compositor notes: [../working-with-iframes/SKILL.md](../working-with-iframes/SKILL.md)
- General live loop: [../operating-cdp-cli/SKILL.md](../operating-cdp-cli/SKILL.md)
