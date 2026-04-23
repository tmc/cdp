# Handling JavaScript Dialogs

Use these MCP tools when a page opens a native JavaScript dialog:

- `get_dialogs`
- `handle_dialog`

`get_dialogs` returns the current pending dialog, if any, plus history. `handle_dialog` accepts or dismisses the pending dialog and can send `prompt_text` for prompts.

## Typical flow

1. Trigger the action that opens the dialog.
2. Call `get_dialogs` to inspect `type`, `message`, `url`, and `default_prompt`.
3. Call `handle_dialog` with `accept: true` or `false`.
4. Verify the page after the dialog closes.

## Notes

- `handle_dialog` covers `alert`, `confirm`, `prompt`, and `beforeunload`.
- If nothing is pending, `handle_dialog` returns `no pending dialog`.
- Dialog capture is enabled for MCP sessions, so `get_dialogs` is the first check when a flow appears blocked.

## `cdpscript` note

[The script format reference](../../writing-cdp-scripts/references/script-format.md) does not define `get_dialogs` or `handle_dialog`. Use the MCP tools for real dialog handling; page-side `js` cannot reliably dismiss a native dialog after it opens.
