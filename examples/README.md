# cdpscript examples

This directory contains runnable browser scripts and small supporting assets.
Examples are not a second skill catalog; they are executable knowledge that can
be inspected, run, and turned into tests when the workflow becomes stable.
Exploratory drafts live under `scratch/` and are excluded from the reusable
top-level example index.

## Example types

- Local examples use only files or local test servers and should be eligible for
  `go test` or explicit CI fixtures.
- Live-only examples depend on a real site, login state, or user-controlled
  data. They must say so in the txtar header instead of pretending CI can verify
  them.

Every curated live-only txtar should start with:

```text
# Purpose: ...
# Usage: ...
# Inputs: ...
# Verification: live-only; ...
```

The archive body should still be runnable: it must contain `main.cdp`, and any
`jsfile` reference in `main.cdp` should point at a file embedded in the same
txtar.

Use [live-workflow-intake.md](live-workflow-intake.md) to collect the human
boundary before capture. Use [live-workflow-template.md](live-workflow-template.md)
when turning a repeated authenticated workflow into a curated live-only
example.

Use [live-workflows.md](live-workflows.md) to track proposed authenticated
workflows before promotion. The first workflow, Google Docs to Markdown, is
curated and builds on `gdoc-to-markdown.txtar` and keeps document URLs,
account names, screenshots, and generated Markdown out of the repository.

Before capture, collect the site/product, browser target or profile, required
login state, user-owned input data, expected output, verification boundary,
secret redaction needs, repeat evidence, and known UI fragility. If any of
those are unknown, keep the workflow as an exploratory local script rather than
a curated example.

## Running against an existing browser

When the script depends on the user browser's current login state, attach
instead of launching a fresh profile:

```bash
cdp attach --port 9222
cdpscript --tab <target-id> --port 9222 examples/gdoc-to-markdown.txtar
cdp run --tab <target-id> --port 9222 examples/gdoc-to-markdown.txtar
```

Use the command printed by `cdp attach`; do not guess tab IDs.

## Curated live examples

- `extract-gemini-apikey.txtar`: opens Google AI Studio and prints a Gemini API
  key if one is visible to the signed-in profile.
- `gdoc-to-markdown.txtar`: opens the Google Doc named by `GDOC_URL` and prints
  visible content as Markdown.
- `simple.txtar`: demonstrates positional script arguments with `${ARG1}`.

These examples are guarded by `example_contract_test.go`. The test verifies the
header contract, `main.cdp`, and embedded `jsfile` references. It does not claim
that Google UI flows or account state are available in CI.

## Promotion rule

Promote a live-only script only after repeated real use shows that the workflow
is stable enough to document. If the site is unstable or account-specific, keep
the script live-only and make its inputs and verification boundary explicit.
