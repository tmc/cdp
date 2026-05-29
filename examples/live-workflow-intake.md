# live workflow intake

Fill this out before promoting an authenticated browser workflow into a
curated live-only `cdpscript` example. If any field is unknown, keep the script
exploratory.

## Required

- Site or product:
- Workflow name:
- Browser target or profile:
- Required login state:
- Starting URL or tab:
- User-owned input data:
- Expected output artifact or stdout:
- Verification boundary:
- Secret handling and redaction:
- Repeat evidence:
- Known UI fragility:

## Constraints

- May the workflow change live data:
- Must the workflow be read-only:
- Are screenshots allowed:
- Are URLs allowed in artifacts:
- Are document, workspace, or account names allowed in artifacts:

## Capture Command

```bash
cdp attach --port 9222
cdpscript --tab <target-id> --port 9222 examples/<workflow>.txtar
```

Use the tab command printed by `cdp attach`. Do not infer an authenticated account,
tab, document, or workspace from nearby browser state.

## Promotion Check

- The script starts with `Purpose`, `Usage`, `Inputs`, and `Verification`
  headers.
- `Verification` says `live-only;` and names what was checked.
- The script captures screenshot-first observe-act-verify checkpoints.
- Secrets, cookies, bearer tokens, API keys, private text, and account-specific
  identifiers are not stored in the txtar.
- Reusable values are environment variables, not hard-coded user state.
- Repeated use showed the workflow is stable enough to document.
