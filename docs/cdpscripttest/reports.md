---
title: Reports and artifacts
description: Per-script and combined Markdown reports pairing each script with its log and images, rendering them to HTML, and redacting the environment first.
icon: file-lines
---

# Reports and artifacts

A browser test that only reports pass or fail throws away most of what it saw.
`cdpscripttest` can emit a Markdown report pairing each script's source with its
command log and the artifacts it produced — which turns a failing run into
something a person, or an agent, can read without rerunning it.

```go
cdpscripttest.GenerateReport(reportPath, name, archive.Comment, log)
```

The command log is what `Engine.Execute` writes to its `io.Writer`, so capture
it in a `strings.Builder` rather than discarding it.

## Combined reports

For a suite, one merged report beats a directory of files:

```go
w, err := cdpscripttest.NewCombinedReportWriter(combinedPath, names, sources)
...
w.Update(cdpscripttest.ScriptReport{
	Name:        name,
	Source:      archive.Comment,
	Log:         log,
	ArtifactDir: scriptArtifacts,
	Failed:      failed,
})
```

`Update` is called per script as it finishes, so a run that crashes halfway
still leaves a readable report covering what completed. `GenerateCombinedReport`
writes them all at once when you already have the full set.

For Go tests, the compatibility flags `-emit-cdp-report` and
`-emit-cdp-report-combined` write Markdown reports into the normal artifact
tree. For a self-contained report tree with native HTML, use:

```bash
go test -p 1 -parallel 1 -tags cdp ./cdpscripttest \
  -cdp-report-dir=./report \
  -emit-cdp-report-html \
  -emit-cdp-report-combined
```

This writes `index.md` and `index.html` at `./report`, with each fixture's
`report.md`, `report.html`, and existing artifacts in `./report/<fixture>/`.
`-cdp-report-dir` takes precedence over the usual artifact-root selection.
`-emit-cdp-report-html` writes HTML only when `-cdp-report-dir` is set.

## Rendering to HTML

`cdpscripttest/report` renders HTML directly with `html/template`. HTML pages
use relative links to the existing artifact tree, so no converter or asset-copy
step is required.

## What ends up in a report, and what to redact

`GenerateReport` takes the script source and the execution log. It does not
serialize the environment, so a credential in `os.Environ()` does not land in a
report merely by being there.

The leak path is the **log**. Anything a script prints is captured, so a fixture
that runs `env`, echoes a variable, or evaluates JavaScript that returns a token
puts that value into the report — which then gets committed, rendered to HTML,
and attached to pull requests.

Two defenses, and you want both. Do not print secrets from a fixture. And when a
run's report will be shared, filter the environment you hand the runner so a
careless `env` cannot expose anything:

```go
func sanitize(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if sensitive(k) {
			out = append(out, k+"=[redacted]")
			continue
		}
		out = append(out, k+"="+v)
	}
	return out
}
```

Treat a key as sensitive when it contains `TOKEN`, `SECRET`, `PASSWORD`,
`COOKIE`, or `PRIVATE_KEY`, or ends in `_KEY` or `_API`. That list is a floor,
not a guarantee — a key named `github_pat` passes all of those checks. Prefer an
allowlist when the report is published.

The same caution applies to captures: a HAR of an authenticated session contains
the headers that made it authenticated. See
[Capture network traffic](/docs/capturing-traffic).

## Coverage

Native fixture execution starts CDP coverage collection by default and writes
`coverage.json` into each script's artifact directory. That is why the file
appears next to your screenshots without anyone asking for it.

```bash
CDPSCRIPTTEST_COVERAGE=0 go test -tags cdp ./...   # also 'false', 'off'
```

Coverage only records JavaScript that executed, so a server-rendered page with
little script produces a near-empty file — expected, not a failure.

## Artifact layout

Screenshots, baselines, GIFs, and reports share one artifact directory per
script. Resolution order and the flat `CDPSCRIPTTEST_ARTIFACTS` layout are
covered in [Visual testing](/docs/cdpscripttest/visual).

Keep the artifact root out of the committed tree unless it holds baselines you
mean to keep. A run that clears its artifact root each time cannot accumulate
baselines — every comparison becomes a first run, and first runs always pass.

## Next steps

- [Visual testing](/docs/cdpscripttest/visual) — where baselines live.
- [Agentic workflows](/docs/cdpscripttest/agentic) — reports as agent-readable evidence.
