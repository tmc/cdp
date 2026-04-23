---
name: emulating-device
description: Applies device and network emulation through the cdp MCP server. Use when the task involves viewport changes, mobile presets, throttling, offline mode, geolocation, or request header overrides.
---

# Emulating Device

Use this skill when browser behavior depends on device shape, network conditions, or browser identity.

## Quick start

- Use `set_device` for common mobile, tablet, and desktop presets
- Use `set_viewport`, `set_user_agent`, and `set_extra_headers` for custom overrides
- Use `set_throttling`, `set_offline`, and `set_geolocation` for network and environment changes

## Use this skill for

- reproducing mobile layout bugs
- testing throttled or offline behavior
- changing browser identity or request headers
- pairing emulation with screenshots or PDF capture

## Read next

- Tool details and available presets: [references/device-emulation.md](references/device-emulation.md)
- Script format reference: [../writing-cdp-scripts/references/script-format.md](../writing-cdp-scripts/references/script-format.md)
- Visual capture after emulation: [../capturing-page-artifacts/SKILL.md](../capturing-page-artifacts/SKILL.md)
