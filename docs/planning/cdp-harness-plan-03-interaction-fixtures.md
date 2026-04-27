# Plan 03 — Offline interaction fixtures

**Status:** Draft v1
**Date:** 2026-04-21
**Depends on:** plan 01 (commands must be real), plan 04 (argv + exit codes)

## 1. Problem

Skill docs (plan 02) describe mechanics abstractly. Agents benefit from a
canonical, runnable, offline example per mechanic that stays green in CI
forever. No such fixtures exist today.

## 2. Key correction from earlier drafts

Previous drafts claimed fixtures live under `cmd/cdpscripttest/testdata/`
alongside `rtc-*.txt`. Ground truth:

- The package is `cdpscripttest/` at **repo root**, not `cmd/cdpscripttest/`.
- `cdpscripttest/testdata/` currently holds **three HTML files only**:
  `webrtc-loopback.html`, `blur-dashboard.html`, `blur-dynamic.html`.
  No `rtc-*.txt` txtar fixtures exist anywhere in the repo.
- The package is **build-tagged `cdp`** (`cdpscripttest/testdata_test.go:1`)
  and is a Go **test-helper library** used by `webrtc_test.go` etc. There
  is no "runner" to wire fixtures into — it is an API surface consumed by
  Go tests.
- Existing `*.cdp` / `*.txtar` scripts live in `examples/` (e.g.
  `examples/aistudio-fc.cdp`). This is where real scripts already live.

Any fixture plan must respect these constraints.

## 3. Scope

Add offline interaction fixtures in a new subdirectory, plus a Go test
file (under `//go:build cdp`) that iterates them and runs each through the
`cdpscript` engine.

### 3.1 Layout

```
cdpscripttest/
  testdata/
    interaction/
      dialogs.txtar
      iframes.txtar
      uploads.txtar
      intercept.txtar
      storage.txtar
      emulation.txtar
      domdiff.txtar
      pages/
        dialog-trigger.html
        iframe-host.html
        iframe-inner.html
        upload-form.html
        storage-demo.html
        domdiff-before.html
        domdiff-after.html
  interaction_test.go        (new)
```

Why `interaction/` under `testdata/`:

- Keeps static HTML pages (served by `httptest.FileServer`) distinct from
  txtar-packaged fixtures.
- `testdata_test.go:15` already serves the whole `testdata/` directory, so
  pages are fetchable at `${URL}/interaction/pages/<name>.html` from inside
  fixtures.

### 3.2 Each fixture

A fixture is a txtar archive (`.txtar`) with:

- `meta.yaml` — only fields the engine reads (`name`, `description`,
  `timeout`, optionally `env`, optionally `profile`).
- `main.cdp` — uses **only** commands validated by plan 01.
- Any helper `.html` or `.js` lives in `testdata/interaction/pages/` and
  is referenced via the httptest base URL, **not** embedded per-fixture
  (reduces duplication; embedded HTML drifts independently otherwise).

Each fixture:

1. Navigates to a local page.
2. Exercises the mechanic.
3. Asserts an observable outcome with `assert exists|text|visible`.
4. Writes artifacts to `$WORK` or the engine's output dir so test cleanup is
   trivial.

### 3.3 Runner

`cdpscripttest/interaction_test.go`:

```go
//go:build cdp

package cdpscripttest_test

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    "github.com/tmc/cdp/cdpscript"
)

func TestInteractionFixtures(t *testing.T) {
    matches, err := filepath.Glob("testdata/interaction/*.txtar")
    if err != nil { t.Fatal(err) }
    if len(matches) == 0 { t.Skip("no fixtures") }

    base := startTestServer(t) // existing helper in testdata_test.go:15
    t.Setenv("FIXTURE_BASE_URL", base)

    for _, path := range matches {
        path := path
        t.Run(filepath.Base(path), func(t *testing.T) {
            engine := cdpscript.New(cdpscript.WithVerbose(testing.Verbose()))
            if err := engine.ExecuteTxtar(context.Background(), path); err != nil {
                t.Fatalf("%s: %v", path, err)
            }
            _ = os.Stderr
        })
    }
}
```

That is the entire wiring. No new runner, no new package. Each fixture
launches a browser via the engine's existing `initBrowser`; CI already runs
Chrome under the `cdp` build tag (see `cdpscripttest/webrtc_test.go` for
precedent).

## 4. Out of scope

- **Third-party-network fixtures.** Flaky; rejected in every prior round.
  Everything targets `${FIXTURE_BASE_URL}` (the existing httptest server).
- **`--tag external` metadata.** Speculative and unneeded.
- **Embedding `pages/*.html` inside each txtar.** Duplication; use the
  shared httptest root.
- **Screenshots-as-golden.** Tempting but brittle; assertions stay
  DOM/text-based.

## 5. Rollout

Day of work — 7 fixtures + pages + runner + cross-links from the plan 02
skill docs.

Test plan:

- `go test -tags cdp ./cdpscripttest/...` runs all fixtures green.
- CI runs Chrome headless; fixtures must set `headless: true` in
  `meta.yaml`.
- Each plan 02 skill doc adds a "See also: `cdpscripttest/testdata/interaction/<name>.txtar`"
  link.

## 6. Risks

- **Chrome flakiness on CI.** Existing `webrtc_test.go` already lives with
  this; keep per-fixture timeouts aggressive (`timeout: 20s` ceiling).
- **Fixture rot when engine commands change.** That is the point — they
  fail CI when plan 01's invariants break. Feature, not bug.
- **Upload and emulation fixtures require real browser surface.** If
  `mcp_emulation_tools.go` tools are not yet exposed to the engine's
  `cdpscript` command set, those fixtures become MCP-only tests and should
  live elsewhere (cmd/cdp integration tests). Verify engine surface
  before writing the fixture; skip if not reachable from `main.cdp`.
