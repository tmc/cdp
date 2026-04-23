---
name: uploading-files
description: Uploads local files into browser file inputs through the cdp MCP server. Use when the task involves selecting files for forms, drag-less upload widgets, or validating multi-file upload flows.
---

# Uploading Files

Use this skill when a page expects local files through `<input type="file">`.

## Quick start

- Locate the file input with a CSS selector or `@ref`
- Call `upload_file` with absolute file paths
- Verify the result with `get_element`, `check_element`, or page text

## Use this skill for

- single-file and multi-file input uploads
- upload widgets backed by hidden file inputs
- validating file chooser state without a manual picker

## Read next

- Tool details and limits: [references/uploads.md](references/uploads.md)
- Script format reference: [../writing-cdp-scripts/references/script-format.md](../writing-cdp-scripts/references/script-format.md)
- General MCP browser control: [../operating-cdp-cli/SKILL.md](../operating-cdp-cli/SKILL.md)
