# cua

`cua` presents browser and native computer use through seven MCP tools. It starts
an exclusive backend process for each configured backend and client connection.
Backend commands use argument arrays; no shell expands them.

Build the coordinator with `go build ./cmd/cua`, then run `cua -config cua.json`.
For example, a browser configuration can attach to an existing debugging target:

```json
{
  "browser": {
    "command": ["cdp", "-mcp", "-remote-host", "127.0.0.1", "-remote-port", "9222", "-remote-tab", "TARGET_ID"]
  },
  "native": {
    "command": ["computer-use-mcp"],
    "env": {"MACGO_NO_RELAUNCH": "1"}
  }
}
```

Replace the target ID, port and executable paths for your installation. Either
backend may be omitted. The native direct mode avoids app relaunch; the process
still needs macOS permissions. To request app approval, call `surfaces_list`
with `kind: "native"`, `app`, and `request_approval: true` from a client supporting
MCP form elicitation. The prompt identifies the app and explains that acceptance
stores approval for future sessions. Ordinary discovery never prompts. Decline,
cancel, unsupported clients and permission failures yield no new candidates.
App approval does not grant Accessibility or Screen Recording permission. The
request uses the same 30-second bound as discovery; a timeout is never treated as
acceptance.

Use `surfaces_list`, then `surface_select` with a returned `selection_id`.
For native windows, first list apps, then list with `kind: "native"` and `app`.
Selection retains a surface without capturing it. Call `surface_observe` with
its `surface_id`; call `surface_act` with that surface and its `observation_id`.
Browser actions are `click`, `type` and `navigate`. Native actions are `click`,
`type_text`, `set_value`, `scroll`, `secondary_action`, `press_key` and `drag`.
Action-specific fields go in `action_arguments`.

Tokens belong to one connection. Each backend has one current observation; an
action consumes it. Backend routing tokens cannot be supplied as action
arguments. Results report execution, observation and postcondition separately.
A lost reply may mean the action happened: never replay `dispatched_unknown`.
Use a fresh observation to inspect the result instead.

`surface_release` drops a selected surface. Native release frees its retained
handle. Browser release is currently logical; its attachment remains until a
later switch or disconnect. Disconnect closes the owned backend processes and
preserves borrowed browser tabs and native windows. Cross-client handoff and
owned window creation are not implemented. Operations within one connection
are serialized, including operations on different backends.

Use `app_approval_revoke` with an exact `bundle_id` to withdraw native app
approval, even when the app is stopped. This affects persistent and session
grants in cooperating backends sharing the native approval store. It does not
prompt, change macOS permissions, close windows, or undo dispatched actions.
A later explicit approval may grant access again. `revoked: false` with
`error_text` or an MCP error means withdrawal was not confirmed; inspect current
state and do not automatically retry. `timeout_ms` bounds waiting (default
30 seconds, maximum 60 seconds); regular filesystem I/O remains subject to OS
limits. This operation does not require a selected surface.

Native action results preserve `recovery_id` and `pointer_recovery` when a
pointer gesture needs cleanup. Use `native_recover_pointer` with `mode: "status"`
to inspect recovery in this connection's native backend. If `retryable` is true,
an explicit `mode: "retry"` with that exact `recovery_id` retries only the original
confirmed-unsent mouse-up. It cannot create a new gesture or retry an uncertain
post. Poll status for completion; never automatically retry. Status and retry
do not consume a surface observation or require a new surface selection.
