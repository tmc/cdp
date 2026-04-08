---
name: writing-cdp-scripts
description: Writes and explains txtar-based cdp automation scripts, including meta.yaml, main.cdp, helper JavaScript files, sourced helper scripts, assertions, and artifact output. Use when the task involves authoring, debugging, or reviewing cdp run scripts.
---

# Writing CDP Scripts

Use this skill when the task needs a reusable `cdp run` script rather than ad hoc CLI commands.

## Quick start

- Scripts are txtar archives
- `main.cdp` is required
- `meta.yaml`, helper `.js`, and helper `.cdp` files are optional

## Use this skill for

- designing `main.cdp` workflows
- choosing between inline commands and sourced helpers
- wiring environment variables and output artifacts
- explaining assertions, waits, snapshots, and extraction commands

## Read next

- Full txtar format and command reference: [references/script-format.md](references/script-format.md)
- Screenshot and PDF artifacts: [../capturing-page-artifacts/SKILL.md](../capturing-page-artifacts/SKILL.md)
- HAR tagging and capture commands: [../capturing-network-traffic/SKILL.md](../capturing-network-traffic/SKILL.md)
