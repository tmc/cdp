# Browser Storage

The MCP storage and state tools are:

- `get_storage`
- `set_storage`
- `clear_storage`
- `get_cookies`
- `set_cookie`
- `save_state`
- `load_state`

## Storage types

`get_storage`, `set_storage`, and `clear_storage` accept `type: "local"` or `type: "session"`.

## Cookies

- `get_cookies` optionally filters by `domain`
- `set_cookie` requires `name`, `value`, and `domain`; `path` is optional

## Full state save and restore

- `save_state` writes cookies, `localStorage`, `sessionStorage`, and the current URL to JSON
- `load_state` restores that JSON and navigates to the saved URL first when one is present

## `cdpscript` note

[The script format reference](../../writing-cdp-scripts/references/script-format.md) does not define storage or cookie commands. In txtar scripts, `js` or `jsfile` can read and write `localStorage` or `sessionStorage` in the current page context, but cookie management and `save_state`/`load_state` are MCP-only today.
