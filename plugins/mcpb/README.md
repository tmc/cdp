# cdp for Claude Desktop (MCP Bundle)

[`manifest.json`](manifest.json) packages the `cdp` Chrome DevTools Protocol
toolkit as an [MCP bundle](https://github.com/anthropics/mcpb) for Claude
Desktop.

## Prerequisite

Install the binary (Go 1.26+) and make sure your Go bin directory is on `PATH`:

```bash
go install github.com/tmc/cdp/cmd/cdp@latest
```

The bundle launches `cdp` from `PATH`; it does not embed the binary.

## Build the bundle

With the [`mcpb` CLI](https://github.com/anthropics/mcpb):

```bash
cd plugins/mcpb
mcpb pack . cdp.mcpb
```

`mcpb pack` validates `manifest.json` and writes `cdp.mcpb`.

## Install

Open `cdp.mcpb` with Claude Desktop (or drag it onto the app), then enable it
in Settings → Extensions. The server runs as `cdp --mcp --headless`.

## Customizing the launch

The bundle defaults to headless Chrome. To watch the browser, persist captures,
or enable extra tools, edit the server entry Claude Desktop generated for this
extension and adjust the args, for example:

```json
{ "command": "cdp", "args": ["--mcp", "--output-dir", "/path/to/captures"] }
```

Run `cdp --help` for the full flag list.
