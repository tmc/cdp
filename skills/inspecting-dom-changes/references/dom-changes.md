# DOM Snapshot And Diff

The MCP DOM-change tools are:

- `snapshot_dom`
- `dom_diff`
- `list_dom_snapshots`

These snapshots are simplified accessibility-tree views of the page, not raw HTML dumps.

## Typical flow

1. Call `snapshot_dom` with a name such as `before`.
2. Trigger the interaction or navigation.
3. Call `snapshot_dom` again with a second name such as `after`.
4. Call `dom_diff` with the two names.

## Notes

- `list_dom_snapshots` reports the currently stored names.
- The diff is best for structural and text changes that show up in the accessibility tree.
- Pair with `page_snapshot`, `get_element`, or `check_element` when you need selector-level detail.

## `cdpscript` note

[The script format reference](../../writing-cdp-scripts/references/script-format.md) documents `snapshot` output, but it does not define `snapshot_dom`, `dom_diff`, or named DOM snapshot storage. Use the MCP tools when you need reusable before-and-after diffs.
