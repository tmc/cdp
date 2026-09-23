---
title: Differential capture
description: Record named captures of the same page and compare them to see which requests appeared, disappeared, or changed between runs.
icon: code-compare
---

# Differential capture

Differential capture records named captures and compares them, so you can see
what changed between two runs of the same page. It is the tool for questions
like "which requests does the new build make that the old one did not?"

Captures live in a working directory, which is what ties a comparison together.
Pass the same `-diff-work-dir` to every command in a session, or they will not
see each other. (The `localhost:8099` fixture below is the demo server from the
[Quickstart](/docs/quickstart).)

## Recording two captures

```bash
chrome-to-har -headless -diff-mode -diff-work-dir ./diffwork \
	-url http://localhost:8099/ -capture-name base
```

```
Completed capture: base (ID: 20260804-185921-3ec56fb6)
To compare with another capture, use: -compare-with <capture-id> -baseline 20260804-185921-3ec56fb6
```

Make whatever change you are measuring, then record the second capture into the
same directory:

```bash
chrome-to-har -headless -diff-mode -diff-work-dir ./diffwork \
	-url http://localhost:8099/ -capture-name after
```

`-capture-name` is a label for your benefit; the ID is what the comparison
flags take. `-list-captures` reports both:

```bash
chrome-to-har -diff-work-dir ./diffwork -list-captures
```

```
Available captures (2):
  ID: 20260804-185924-892cd850
  Name: after
  URL: http://localhost:8099/
  Timestamp: 2026-08-04 18:59:24
  Status: completed
  Entries: 1
  Size: 2.15 KB
  …
```

`Status: completed` with a nonzero `Entries` is what a usable capture looks
like. A capture stuck at `recording` with `Entries: 0` did not record anything,
and comparing against it will fail for want of a HAR file.

Recording obeys the ordinary capture flags, so `-wait-for`, `-wait-stable`,
`-urls`, and `-block` all apply. Use them: a capture that stops before the page
finishes loading produces a diff full of differences that are really just
timing.

## Comparing

```bash
chrome-to-har -diff-mode -diff-work-dir ./diffwork \
	-baseline 20260804-185921-3ec56fb6 \
	-compare-with 20260804-185924-892cd850 \
	-diff-format json -diff-output report.json
```

```
Comparison completed successfully
Report generated: report.json
Summary: 0 added, 0 removed, 1 modified requests
```

`-diff-format` accepts `html` (the default), `json`, `text`, and `csv`. The
JSON report carries `baseline_capture`, `compare_capture`, `summary`,
`network_diffs`, `resource_diffs`, `performance_diff`, and `timestamp`; the
`summary` object holds the added, removed, modified, and unchanged request
counts. Reach for `json` when something downstream reads the result, and `html`
when a person does.

Two captures of an unchanged page still report `1 modified` in the example
above — response timings differ between runs even when the bytes do not. Treat
`added` and `removed` as the signal, and read `modified` with that in mind.

## When a plain diff is enough

If all you want is which URLs appeared or disappeared, two ordinary captures
and `jq` answer it without a working directory:

```bash
chrome-to-har -headless -url http://localhost:8099/ -wait-for '#done' -output before.har
# make the change
chrome-to-har -headless -url http://localhost:8099/ -wait-for '#done' -output after.har

diff <(jq -r '.log.entries[].request.url' before.har | sort) \
     <(jq -r '.log.entries[].request.url' after.har  | sort)
```

The differential mode earns its keep when you want the comparison itself as an
artifact — a report to attach, or timing and resource changes rather than a
list of URLs.

## Next steps

- [Capture network traffic](/docs/capturing-traffic) — the single-capture
  workflow these build on.
- [Command reference](/docs/commands) — `go doc ./cmd/chrome-to-har` for every
  flag.
