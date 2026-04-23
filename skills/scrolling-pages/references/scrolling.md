# Scrolling

The MCP scroll tool is:

- `scroll`

It supports two modes:

- page scrolling with `direction: up|down|left|right` and optional `distance`
- element scrolling with `selector` or `@ref`

If `distance` is omitted, the default is `500` pixels.

## Typical flow

1. Use directional scrolling until the relevant content appears.
2. Switch to selector-based scrolling when a specific element should be centered.
3. Verify the new viewport with `page_snapshot`, `get_page_content`, or `screenshot`.

## `cdpscript` note

[The script format reference](../../writing-cdp-scripts/references/script-format.md) does not define a `scroll` command. In txtar scripts, use `js window.scrollTo(...)` or `js document.querySelector(...).scrollIntoView(...)` when that is sufficient.
