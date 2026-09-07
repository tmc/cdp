# Browser observations and actions

Use `browser_observe` and `browser_act` when an action must refer to the page
state you inspected. Both are MCP tools provided by `cdp`.

Call `browser_observe` with an empty object. It returns a `state_id`, `target_id`,
document identity, URL, and accessibility tree. Interactive elements in the tree
have refs such as `@1`. Copy the returned identifiers into your action:

```json
{
  "state_id": "<returned state_id>",
  "target_id": "<returned target_id>",
  "action": "click",
  "ref": "@1",
  "expect": {"selector": "#count", "text": "1"}
}
```

The ref must come from that observation. For typing, use `"action": "type"`
and supply a nonempty `text`. `timeout_ms` defaults to 30000 and accepts values
from 1 through 60000; zero selects the default.

A successful observation replaces the previous state. An action consumes its
matching state before validation or dispatch, so the same handle cannot be
retried after a failed attempt. Unknown state IDs do not consume the current
state. Invalid timeout values are rejected before state lookup.

The action checks the exact target context, frame, loader and document node.
It also checks that the referenced node is connected to that document. A
same-URL reload, detached node, or changed active target requires another
observation. A detached node is never replaced by another node with the same
name or role.

Before input, the tool activates the target page. Before clicking, it scrolls
the node into view and checks that its click
point reaches that node or one of its descendants. An overlay covering that
point is rejected. Before typing, it checks that focus remains on the observed
node, including focus within a shadow tree.

The result separates three questions:

| Field | Values | Meaning |
| --- | --- | --- |
| `execution` | `not_dispatched`, `dispatched_unknown`, `completed` | Whether action-related commands were attempted and the complete input sequence acknowledged. |
| `observation` | `captured`, `unavailable` | Whether the subsequent page observation succeeded. |
| `postcondition` | `not_requested`, `unknown`, `unmet`, `met` | Whether the optional exact text condition was checked and satisfied. |

Page activation, scrolling and focusing are action-related side effects. A failure after one of
these commands begins is conservatively `dispatched_unknown`. `completed`
describes browser acknowledgement, not completion of the user's broader task.
If capture fails after acknowledged input, execution remains `completed` and
no `fresh_state` is returned. Inspect the target before considering another
action; a missing observation is not permission to click again.

When capture succeeds, use `fresh_state` for the next action. `expect` compares
the exact `textContent` of a single CSS match in the captured document, once,
without waiting for an eventual value. Missing or multiple matches are `unmet`;
an evaluation error is `unknown`. It does not compare input values. `error_text`
explains validation, execution, capture, or postcondition failures.

The pair serializes its own requests and publishes complete observations.
Page scripts, other tools and users can still change the page between validation
and input. This is not a page transaction, and cancellation cannot undo input
already sent. An observation records document identity; it does not freeze the
DOM or guarantee that an unchanged node still has the same business meaning.

The current pair supports clicks and typing in the active target's top document.
A selected same-process subframe is rejected explicitly. Navigation, screenshot
coordinate mapping, and same-process frame actions are not yet part of this pair.
The pair requires the CDP Accessibility domain; it reports an observation error
when that domain is unavailable.
