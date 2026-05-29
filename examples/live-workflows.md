# live workflows

This file records authenticated workflows that are candidates for, or have
been promoted to, curated live-only `cdpscript` examples. A workflow can move
from proposed to curated only after it has been run repeatedly against a
user-approved browser/profile and its redaction boundary is clear.

## Google Docs to Markdown

- Site or product: Google Docs.
- Workflow name: `google-docs-to-markdown`.
- Status: curated live-only workflow.
- Existing script: `examples/gdoc-to-markdown.txtar`.
- Browser target or profile: an already-running Chrome-compatible browser with
  remote debugging enabled, or a profile that is signed in to Google and has
  access to the document.
- Required login state: signed in to a Google account that can view the target
  document.
- Starting URL or tab: either `GDOC_URL` or a selected Google Docs tab from
  `cdp attach --port 9222`.
- User-owned input data: a Google Docs document URL supplied through
  `GDOC_URL`; no document ID should be hard-coded into the script.
- Expected output artifact or stdout: Markdown printed to stdout.
- Verification boundary: live-only; verify that the script reaches the Docs
  editor, prints non-empty Markdown, and does not print cookies, bearer tokens,
  account identifiers, or raw browser storage.
- Secret handling and redaction: do not store document URLs, document IDs,
  account names, cookies, tokens, screenshots containing private text, or
  generated Markdown in the repository.
- Repeat evidence: run successfully at least twice against the same
  user-approved document and once against a second disposable document before
  calling the workflow curated.
- Current evidence: ran twice on 2026-05-16 against the same user-supplied
  document with identical stdout hash
  `5a611e843ad4adeb232d578cb509e4742ccbb3570c39664880c082cbe75abe01`,
  `6738` stdout bytes, `20` stdout lines, and empty stderr. It also ran once
  against a second disposable document with stdout hash
  `e5ce4d21e7300ab8106d6c96e1464ae69124eb34371436b5bae6cc920cbdc6a0`,
  `7` stdout bytes, `1` stdout line, empty stderr, and a direct export probe
  confirming HTTP 200 `text/plain` with `6` content bytes.
- Known UI fragility: Google Docs editor class names and canvas-backed document
  rendering can change; keep selectors minimal and prefer attached-tab
  observe-act-verify screenshots during live capture.
- May the workflow change live data: no.
- Must the workflow be read-only: yes.
- Are screenshots allowed: only transient local screenshots for verification;
  do not commit screenshots containing document text.
- Are URLs allowed in artifacts: no, unless they are placeholder examples.
- Are document, workspace, or account names allowed in artifacts: no.

Capture command:

```bash
cdp attach --port 9222
GDOC_URL="https://docs.google.com/document/d/YOUR_DOC_ID/edit" \
  cdpscript --tab <target-id> --port 9222 examples/gdoc-to-markdown.txtar
```

Promotion checks:

- The script keeps `Purpose`, `Usage`, `Inputs`, and `Verification` headers.
- `Verification` stays `live-only;` and names the checked behavior.
- The script uses environment variables for reusable values.
- Any future screenshot checkpoints are output artifacts only, not repository
  fixtures.
- Static tests continue to verify structure without depending on Google login.
