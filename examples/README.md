# cdpscript examples

This directory contains runnable browser scripts and small supporting assets.
Examples can be inspected, run, and turned into tests when the workflow
becomes stable.
Exploratory drafts go under `scratch/`, which is git-ignored.

## Example types

- Local examples use only files or local test servers. `go test` checks their
  syntax but does not run them.
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

- `gdoc-to-markdown.txtar`: opens the Google Doc named by `GDOC_URL` and prints
  visible content as Markdown.
- `simple.txtar`: demonstrates positional script arguments with `${ARG1}`.

These examples are guarded by `example_contract_test.go`. The test parses every
top-level script with `cdpscript.ValidateReader` and verifies the header
contract, `main.cdp`, and embedded `jsfile` references. It does not start a
browser, and it does not claim that Google UI flows or account state are
available in CI.

## Promotion rule

Promote a live-only script only after repeated real use shows that the workflow
is stable enough to document. If the site is unstable or account-specific, keep
the script live-only and make its inputs and verification boundary explicit.
