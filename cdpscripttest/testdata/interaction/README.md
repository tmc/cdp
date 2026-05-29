# interaction fixtures

These txtars are local browser-mechanic fixtures for `cdpscript` and
`cdpscripttest`. They are the executable index for mechanics that should work
without third-party network access.

Run them with:

```bash
go test -count=1 -tags cdp ./cdpscripttest -run TestInteractionFixtures
```

## Mechanics

| Mechanic | Fixture | Page/data | Skill or reference |
|---|---|---|---|
| blocking scripts and network requests | [block-script.txtar](block-script.txtar) | [pages/block-script.html](pages/block-script.html) | [capturing network traffic](../../../skills/capturing-network-traffic/references/har.md) |
| coordinate clicks | [coordinate-click.txtar](coordinate-click.txtar) | [pages/coordinate-click.html](pages/coordinate-click.html) | [script format](../../../skills/writing-cdp-scripts/references/script-format.md) |
| iframes and closed shadow DOM coordinate hit testing | [coordinate-compositor-surfaces.txtar](coordinate-compositor-surfaces.txtar) | [pages/coordinate-compositor-surfaces.html](pages/coordinate-compositor-surfaces.html), [pages/coordinate-frame.html](pages/coordinate-frame.html) | [iframes](../../../skills/working-with-iframes/references/iframes.md), [shadow DOM](../../../skills/working-with-shadow-dom/references/shadow-dom.md) |
| dialogs | [dialogs.txtar](dialogs.txtar) | [pages/dialogs.html](pages/dialogs.html) | [dialogs](../../../skills/handling-dialogs/references/dialogs.md) |
| downloads | [download.txtar](download.txtar) | [pages/download.html](pages/download.html), [pages/download-payload.txt](pages/download-payload.txt) | [script format](../../../skills/writing-cdp-scripts/references/script-format.md) |
| drag and drop | [drag-drop.txtar](drag-drop.txtar) | [pages/drag-drop.html](pages/drag-drop.html) | [script format](../../../skills/writing-cdp-scripts/references/script-format.md) |
| dropdowns | [dropdown.txtar](dropdown.txtar) | [pages/dropdown.html](pages/dropdown.html) | [script format](../../../skills/writing-cdp-scripts/references/script-format.md) |
| forms | [form-submit.txtar](form-submit.txtar) | [pages/form-submit.html](pages/form-submit.html) | [script format](../../../skills/writing-cdp-scripts/references/script-format.md) |
| history navigation | [history-navigation.txtar](history-navigation.txtar) | [pages/history-start.html](pages/history-start.html) | [script format](../../../skills/writing-cdp-scripts/references/script-format.md) |
| keyboard input | [keyboard-enter.txtar](keyboard-enter.txtar) | [pages/keyboard-enter.html](pages/keyboard-enter.html) | [script format](../../../skills/writing-cdp-scripts/references/script-format.md) |
| screenshots | [../cdpscript/cdpscript-screenshot-output.txtar](../cdpscript/cdpscript-screenshot-output.txtar) | output PNG artifact | [screenshots](../../../skills/capturing-page-artifacts/references/screenshots.md) |
| scrolling | [scroll.txtar](scroll.txtar) | [pages/scroll.html](pages/scroll.html) | [scrolling](../../../skills/scrolling-pages/references/scrolling.md) |
| accessibility refs and snapshots | [snapshot-refs.txtar](snapshot-refs.txtar) | [pages/snapshot-refs.html](pages/snapshot-refs.html) | [script format](../../../skills/writing-cdp-scripts/references/script-format.md) |
| sourced helper scripts | [source-command.txtar](source-command.txtar) | [source-helper.cdp](source-helper.cdp) | [script format](../../../skills/writing-cdp-scripts/references/script-format.md) |
| storage and cookies | [storage-cookie.txtar](storage-cookie.txtar) | [pages/storage-cookie.html](pages/storage-cookie.html) | [storage](../../../skills/managing-storage/references/storage.md) |
| uploads | [upload-file.txtar](upload-file.txtar) | [pages/upload-form.html](pages/upload-form.html) | [uploads](../../../skills/uploading-files/references/uploads.md) |
| viewport | [viewport.txtar](viewport.txtar) | [pages/viewport.html](pages/viewport.html) | [device emulation](../../../skills/emulating-device/references/device-emulation.md) |

## Artifact fixtures

Artifact-output fixtures live in `../cdpscript` because they exercise the
runtime command contract rather than a single interaction mechanic:

- [../cdpscript/cdpscript-pdf-output.txtar](../cdpscript/cdpscript-pdf-output.txtar)
- [../cdpscript/cdpscript-screenshot-output.txtar](../cdpscript/cdpscript-screenshot-output.txtar)
- [../cdpscript/cdpscript-har-output.txtar](../cdpscript/cdpscript-har-output.txtar)
- [../cdpscript/cdpscript-observe-act-verify.txtar](../cdpscript/cdpscript-observe-act-verify.txtar)
- [../cdpscript/cdpscript-attach-observe-act-verify.txtar](../cdpscript/cdpscript-attach-observe-act-verify.txtar)

Keep this index synchronized when adding a fixture or declaring a mechanic
live-only in a skill. The untagged `fixture_index_test.go` test validates the
links and core mechanic names; the `cdp`-tagged browser tests validate behavior.
