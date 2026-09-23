---
title: Visual testing
description: Screenshot comparison against baselines, masking dynamic content with blur, where baselines live, and screen recording.
icon: image
---

# Visual testing

`screenshot-compare` captures an element and compares it against a stored
baseline, failing when the difference exceeds a threshold. It is the cheapest
way to catch a layout regression that no text assertion would notice.

```text
screenshot-compare --threshold 3 --blur '.timestamp' '#main' main.png
```

Options: `--threshold N` is the maximum allowed diff percent (default 5), and
`--update` overwrites the baseline with the current capture.

Related commands: `screenshot [file]` for a full-page PNG, `screenshot-sel
[--padding N] <sel> [file]` for one element, and `screenrecord start|stop` to
write an animated GIF, PNG, numbered PNG frames, or WebM of a tab. WebM uses
`ffmpeg` from `PATH`; the other formats have no external dependency. If a
script fails while recording, cdpscripttest stops the recording during cleanup
and leaves the artifact path in the command log.

## The first run always passes

On first run the capture *becomes* the baseline. A brand-new
`screenshot-compare` therefore cannot fail, and a fixture added this way is not
yet testing anything. Look at the generated image before you trust it, and
commit it deliberately.

## Blur, do not raise the threshold

Every dynamic element — a timestamp, a generated id, a live counter, a relative
"3 minutes ago" — differs between runs and eats your diff budget. The
temptation is to raise `--threshold` until the test goes green. That also
raises it past the regressions you were trying to catch.

`--blur <selector>` replaces the element's text with a fixed placeholder and
applies a mild CSS blur before capture, so the content produces identical
pixels across runs while still showing that it was present. The flag repeats:

```text
screenshot-compare --blur '.badge' --blur '[class*="time"]' --threshold 5 main main.png
```

A fixture masking a table full of live state looks like:

```text
screenshot-compare --blur '.badge, td, [class*="status"], [class*="time"], [class*="id"]' \
	--threshold 12 'main, [role="main"], body' table.png
```

A threshold that high is a signal the mask is incomplete. When a comparison
fails, the runner writes the current capture next to the baseline as
`<name>.fail.png` — open both before changing any number.

Two flags exist for exactly this diagnosis:

```bash
go test -tags cdp -cdp-emit-unblurred ./...   # save an unmasked copy alongside each capture
go test -tags cdp -cdp-skip-blur ./...        # disable masking entirely, to see raw content
```

`-cdp-emit-unblurred` is the one to reach for first: it shows what the mask hid,
which is how you find the element you forgot to blur.

## Where baselines live

Baselines live in the artifact directory, which is resolved in this order:

1. `CDPSCRIPTTEST_ARTIFACTS` environment variable
2. `-cdp-artifacts=<dir>` flag
3. `-emit-artifacts`, which derives the path from the script's own location —
   `testdata/interaction/viewport.txtar` → `testdata/interaction/artifacts/viewport/`
4. `t.ArtifactDir()`

Environment variables take precedence over their flag counterparts.

```bash
CDPSCRIPTTEST_ARTIFACTS=./out go test -tags cdp -p 1 -parallel 1 ./...
# ./out/fleet-view/fleet-full-page.png
```

That flat layout is the readable one when you want to inspect images.

**Baselines you intend to keep must live somewhere durable.** If the artifact
root is a temporary directory that the run clears, every comparison is a first
run and the suite silently stops catching regressions. Commit baselines under
`testdata/` and point the artifact directory at them.

## Updating baselines

```bash
go test -update-golden -tags cdp -run TestCDPScript ./path/to/pkg
```

`UPDATE_GOLDEN` is the environment equivalent. Update when you changed the UI on
purpose, and look at every changed image before committing. Updating to make a
red test green converts a caught regression into a committed one — it is the
one move that turns this whole mechanism off.

## Stabilizing the render

Pixel comparison is only as reproducible as the browser you run it in. Fix the
window size so layout does not depend on the terminal or display:

```go
opts = append(opts, chromedp.WindowSize(1280, 800))
```

Expect headed and headless to differ. Font availability differs across machines
too, which is the usual reason a baseline captured locally fails in CI. Prefer
comparing a single component (`screenshot-sel`) over a full page when the page
contains anything you do not control.

## Next steps

- [Writing fixtures](/docs/cdpscripttest/fixtures) — the commands around these.
- [Reports and artifacts](/docs/cdpscripttest/reports) — turning a run into
  something reviewable.
