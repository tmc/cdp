---
name: intercepting-requests
description: Intercepts requests and responses through the cdp MCP server. Use when the task involves blocking URLs, stubbing responses, modifying headers, or verifying network behavior during an automated browser session.
---

# Intercepting Requests

Use this skill when the browser session needs network behavior that differs from the live page.

## Quick start

- Add rules with `intercept_request` or `intercept_response`
- Inspect active rules with `list_intercepts`
- Remove one rule with `remove_intercept` or clear all rules with `all: true`

## Use this skill for

- blocking known URLs during a scenario
- fulfilling requests with synthetic responses
- modifying request or response headers
- checking the effect of rules with the live network log

## Read next

- Tool details and script limits: [references/intercepts.md](references/intercepts.md)
- Script format reference: [../writing-cdp-scripts/references/script-format.md](../writing-cdp-scripts/references/script-format.md)
- HAR and network capture: [../capturing-network-traffic/SKILL.md](../capturing-network-traffic/SKILL.md)
