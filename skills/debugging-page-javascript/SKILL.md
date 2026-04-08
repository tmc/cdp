---
name: debugging-page-javascript
description: Executes JavaScript in browser pages, captures console output, and debugs page-side behavior with cdp. Use when the task involves console diagnostics, injected collectors, page evaluation, or browser-side debugging during automation.
---

# Debugging Page JavaScript

Use this skill when the task is about browser-side JavaScript behavior rather than navigation alone.

## Quick start

- Use `eval` or `js` for short page-side expressions
- Use `jsfile` for longer snippets or reusable helpers
- Use collector scripts when the task needs structured console output over time

## Use this skill for

- evaluating expressions in the page context
- enabling and interpreting console output
- capturing page errors or ad hoc telemetry
- debugging page-side state during scripts or interactive sessions

## Read next

- Console and JavaScript execution patterns: [references/console.md](references/console.md)
- CLI-level command surface: [../operating-cdp-cli/SKILL.md](../operating-cdp-cli/SKILL.md)
