---
name: capturing-network-traffic
description: Captures network activity with HAR, HARL, and cdp tagging workflows, including streaming capture, annotations, and WebRTC or gRPC-Web related recorder output. Use when the task involves recording, filtering, or explaining network traffic capture in this repository.
---

# Capturing Network Traffic

Use this skill when the task is about HAR output, streamed request logs, or recorder-generated artifacts.

## Quick start

- Use the main capture CLI for whole-session HAR capture
- Use `cdp run` tags when network capture is part of a larger browser script
- Prefer streamed output when the task needs incremental processing

## Use this skill for

- choosing between HAR file output and line-oriented streaming
- tagging phases of a scripted browser session
- embedding notes, screenshots, and DOM captures into recorded output
- explaining the recorder's gRPC-Web and WebRTC-related capture behavior

## Read next

- HAR and HARL details: [references/har.md](references/har.md)
- Script-level artifact capture: [../writing-cdp-scripts/SKILL.md](../writing-cdp-scripts/SKILL.md)
