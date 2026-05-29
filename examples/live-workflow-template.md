# live workflow template

Use this when promoting a repeated authenticated browser workflow into a
curated live-only `cdpscript` example.

## Intake

- Site or product:
- Required login state:
- Browser target or profile:
- User-owned input data:
- Expected output artifact or stdout:
- Verification boundary:
- Secret handling and redaction:
- Repeat evidence:
- Known UI fragility:

The user must identify the browser/profile and the specific workflow target
before capture starts. Do not infer an authenticated account, tab, document, or
workspace from nearby browser state.

## Attach flow

Use an already-running browser when the workflow depends on current account
state:

```bash
cdp attach --port 9222
cdpscript --tab <target-id> --port 9222 examples/<workflow>.txtar
```

Use the command printed by `cdp attach`; do not guess tab IDs.

## Capture checklist

1. Observe: take an initial screenshot and record the target URL/title.
2. Act: perform one visible step at a time through `cdpscript` commands.
3. Verify: after each meaningful action, wait for a visible state and capture a
   screenshot or stdout artifact.
4. Redact: do not save cookies, bearer tokens, API keys, document-private text,
   or account-specific identifiers in the txtar.
5. Generalize: replace user-specific URLs or names with environment variables
   when the workflow can be reused safely.
6. Contract: add a static test when CI can verify structure without live login.

## Txtar header

Every promoted live-only script starts with:

```text
# Purpose: ...
# Usage: cdpscript --tab <target-id> --port 9222 examples/<workflow>.txtar
# Inputs: ...
# Verification: live-only; ...
```

## Script shape

Prefer screenshot-first checkpoints around user-visible actions:

```text
-- main.cdp --
goto ${TARGET_URL}
wait '<stable selector>'
screenshot 01-start.png

# Act on visible state. Use @refs or selectors when stable; use coordinates
# for controls hidden behind iframes or closed shadow DOM.
click '<selector-or-coord:x,y>'

wait '<result selector>'
screenshot 02-result.png
assert visible '<result selector>'
```

## Promotion rule

Do not promote a one-off exploratory script. Promote only after repeated use
shows that the workflow, inputs, and verification boundary are stable enough to
document.
