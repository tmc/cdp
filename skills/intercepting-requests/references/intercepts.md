# Request And Response Interception

The MCP interception tools are:

- `intercept_request`
- `intercept_response`
- `list_intercepts`
- `remove_intercept`

Useful companion tools:

- `start_network_log`
- `get_network_log`

## Request-stage rules

`intercept_request` matches outgoing requests by `url_pattern`. The documented actions are:

- `block`
- `fulfill`
- `modify`

Patterns support `*` and `?`.

## Response-stage rules

`intercept_response` matches paused responses by `url_pattern`. The documented actions are:

- `modify`
- `fulfill`

## Typical flow

1. Add one or more request or response rules.
2. Exercise the page.
3. Inspect `list_intercepts` to confirm the active rule set.
4. Use `get_network_log` if you need request-level evidence.
5. Remove rules when the scenario ends.

## `cdpscript` note

[The script format reference](../../writing-cdp-scripts/references/script-format.md) only documents `block <pattern>`. There is no txtar command for response interception, request fulfillment, header rewriting, `list_intercepts`, or `remove_intercept`.
