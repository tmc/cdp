# Uploading Files

The MCP upload tool is:

- `upload_file`

`upload_file` takes:

- `selector`: CSS selector or `@ref`
- `files`: one or more absolute paths

## Typical flow

1. Use `page_snapshot`, `find_element`, or direct selectors to locate the file input.
2. Call `upload_file`.
3. Confirm the page reacted as expected with `get_element`, `check_element`, or `get_page_content`.

## Notes

- Multiple paths are supported; the page still needs an input that accepts multiple files.
- Hidden file inputs can still work if the selector resolves to the input element.
- The tool sets files directly on the input element. It does not automate the native picker UI.

## `cdpscript` note

[The script format reference](../../writing-cdp-scripts/references/script-format.md) defines `upload <selector> <file>...`, which sets files on a file input the same way `upload_file` does. Relative paths resolve against the script working directory, then the current directory.
