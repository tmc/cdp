---
title: Adopting cdpscripttest
description: Import path, what the dependency costs your module graph, version pinning, and the build-tag arrangement that keeps browsers out of a default go test.
icon: download
---

# Adopting cdpscripttest

`cdpscripttest` is an exported package of the `github.com/tmc/cdp` module, not a
module of its own:

```go
import "github.com/tmc/cdp/cdpscripttest"
```

```bash
go get github.com/tmc/cdp
```

There is no separate `go get` for the test package. Requiring it requires the
whole module.

## What the dependency costs

Because `cdpscripttest` is part of the main module, importing it brings that
module's whole dependency set into your graph — chromedp and the libraries the
`cdp` and `churl` commands use for extraction and rendering. Expect minimum
versions of anything you already share to be raised to whatever `cdp` requires,
and expect new indirect entries.

That is minimal version selection working correctly, not churn. But if your
project pins any of those deliberately, look before you adopt:

```bash
go mod graph | grep github.com/tmc/cdp   # what it brings
go get github.com/tmc/cdp && go mod tidy # then read the go.mod diff
```

Read the diff rather than assuming. `go mod tidy` drops a stale *require* but
never removes a `replace`, so an unused replace directive can survive the
cleanup and keep pointing at a path that no longer resolves.

**The build tag does not make it free.** Guarding your test file with
`//go:build cdp` keeps the browser out of a default `go test ./...`, but the
import still puts `github.com/tmc/cdp` in your main module graph and in
`go.sum`. The tag controls *execution*, not *dependency*.

## Pinning

Behavior here can change without a version number changing to warn you, so if
you depend on this in CI, pin an exact version rather than tracking whatever
`@latest` resolves to:

```bash
go get github.com/tmc/cdp@<commit-or-version>
```

Check what you actually got — `go list -m github.com/tmc/cdp` — rather than
assuming `@latest` picked up the branch tip.

For local development against a checkout, a `replace` is normal:

```
replace github.com/tmc/cdp => ../cdp
```

If you use a `replace` pointing at a sibling directory, make sure that
directory really is a module root with its own `go.mod`. A `replace` at a path
that has no `go.mod` fails at dependency resolution, before any compilation, with
`no such file or directory` — an error that reads like a missing file rather
than a misconfigured module.

## The build-tag arrangement

Put browser-backed tests behind a tag so contributors without a browser, and CI
jobs that only lint, are unaffected:

```go
//go:build cdp

package myapp_test
```

```bash
go test -tags cdp -p 1 ./...
```

Two failure modes this creates, both of which look like success:

- **Forgetting the tag.** The file is excluded, the package still passes, and
  nothing browser-related ran.
- **A glob that matches nothing.** `Test` calls `t.Fatal` on an empty glob,
  which protects you there — but a `-run` pattern that matches no subtest exits
  0 with nothing executed.

Check `-v` output for named subtests before believing a pass. When a run is
long, wait for the final `ok`/`FAIL <package> <time>` line: Go emits `=== RUN`
lines as subtests start but does not stream their verdicts, so a log read
mid-run undercounts failures.

## Verifying an adoption

```bash
go build ./...
go vet -tags cdp ./path/to/pkg
go test -tags cdp -p 1 -parallel 1 -run TestCDPScript ./path/to/pkg -v
```

The third command is the only one that proves anything, and only if its output
names subtests that actually ran.

## Next steps

- [Writing fixtures](/docs/cdpscripttest/fixtures) — the two dialects.
- [Extending the engine](/docs/cdpscripttest/extending) — app-specific commands.
- [Testing](/docs/testing) — this repository's own suite.
