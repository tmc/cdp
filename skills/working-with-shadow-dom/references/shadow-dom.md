# Shadow DOM

Shadow DOM needs two different strategies.

## Open roots

Open roots can be inspected with JavaScript:

```text
evaluate document.querySelector('my-widget').shadowRoot.querySelector('button').textContent
evaluate document.querySelector('my-widget').shadowRoot.querySelector('button').click()
```

Use this only when the host selector is stable and the root is open. Verify the
result with a screenshot or page state after the action.

## Closed roots

Closed roots are intentionally not exposed through `element.shadowRoot`.
Selectors and JavaScript cannot traverse them unless the application exposes its
own test hook.

For visible controls in a closed root, prefer viewport coordinates:

```text
click coord:320,240
```

Chrome performs compositor hit testing, so coordinate input can reach visible
controls inside closed shadow DOM. The checked-in fixture
`cdpscripttest/testdata/interaction/coordinate-compositor-surfaces.txtar`
covers this behavior with a closed shadow-root button.

## Decision tree

1. If the control appears in `page_snapshot`, use its `@ref`.
2. If the host has an open shadow root, inspect narrowly with `evaluate`.
3. If the control is visible but hidden from refs/selectors, use
   `click coord:x,y` and verify from visible state.
4. If the control is not visible, first scroll, switch frame, or navigate until
   the browser can hit test it.

## `cdpscript` note

`cdpscript` has no shadow-root traversal command. Use `js` for narrow open-root
work and `click coord:x,y` for visible controls in closed roots. Do not add a
shadow-specific script command until repeated scripts need the same operation.
