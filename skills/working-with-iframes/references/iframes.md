# Frames And Iframes

The MCP frame tools are:

- `list_frames`
- `switch_frame`

`switch_frame` accepts:

- `main` or `top` to return to the top frame
- a frame name from `list_frames`
- a numeric child-frame index from `list_frames`
- a CSS selector that points at an iframe element

## Typical flow

1. Call `list_frames`.
2. Pick the target frame by name, index, URL, or iframe selector.
3. Call `switch_frame`.
4. Use normal tools in the selected context: `page_snapshot`, `click`, `type_text`, `wait_for`, `get_page_content`, `evaluate`.
5. Call `switch_frame` with `main` when done.

## Notes

- The tool works best when the iframe has its own target, such as an out-of-process iframe.
- For same-origin frames without a separate target, the current implementation records the frame ID but other tools may still behave like the main frame. Verify with `get_page_content` or `page_snapshot` after switching.

## `cdpscript` note

[The script format reference](../../writing-cdp-scripts/references/script-format.md) does not define frame-listing or frame-switching commands. If script automation must stay in txtar form, use `js` only for narrow same-origin iframe work; there is no general `switch_frame` script command today.
