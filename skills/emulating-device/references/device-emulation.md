# Device And Environment Emulation

The MCP emulation tools are:

- `set_device`
- `set_viewport`
- `set_user_agent`
- `set_offline`
- `set_geolocation`
- `set_throttling`
- `set_extra_headers`

## Preset devices

`set_device` supports these preset names:

- `iphone-14`
- `iphone-14-pro`
- `iphone-15-pro`
- `iphone-15-pro-max`
- `iphone-se`
- `ipad`
- `ipad-pro-11`
- `ipad-pro-12.9`
- `pixel-7`
- `pixel-7-pro`
- `galaxy-s23`
- `galaxy-s23-ultra`
- `galaxy-tab-s8`
- `desktop-1080p`
- `desktop-1440p`

## Network presets

`set_throttling` supports:

- `slow-3g`
- `fast-3g`
- `slow-4g`
- `fast-4g`
- `offline`
- `none`

It also accepts custom download, upload, latency, and CPU slowdown values.

## `cdpscript` note

[The script format reference](../../writing-cdp-scripts/references/script-format.md) defines `viewport <width> <height>` for script-level responsive layout checks. Use MCP tools for device presets, touch, user-agent, geolocation, offline mode, and throttling; page-side `js` is not an equivalent substitute for those browser-level overrides.
