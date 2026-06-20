# cdp for Claude Desktop (MCP Bundle)

[`manifest.json`](manifest.json) packages the `cdp` Chrome DevTools Protocol
toolkit as an [MCP bundle](https://github.com/anthropics/mcpb) for Claude
Desktop.

## Prerequisite

Install the binary (Go 1.26+):

```bash
go install github.com/tmc/cdp/cmd/cdp@latest
```

The bundle does not embed the binary. It launches your installed copy by
**absolute path** — Claude Desktop is a GUI app and does not inherit your shell
`PATH`, so the bundle cannot rely on a bare `cdp` command. The manifest exposes
a "Path to the cdp binary" setting that defaults to `~/go/bin/cdp` (the standard
`go install` location); change it if your `GOBIN`/`GOPATH` differs.

## Build the bundle

With the [`mcpb` CLI](https://github.com/anthropics/mcpb):

```bash
cd plugins/mcpb
npx -y @anthropic-ai/mcpb pack .
```

`mcpb pack` validates `manifest.json` and writes `cdp-0.1.0.mcpb`.

## Install

Open the `.mcpb` with Claude Desktop (or drag it onto the app), then enable it
in Settings → Extensions. Set "Path to the cdp binary" if the default isn't
right. The server runs as `cdp -mcp -headless`.

## Customizing the launch

The bundle defaults to headless Chrome. To watch the browser, persist captures,
or enable extra tools, edit the server entry Claude Desktop generated for this
extension and adjust the args, for example:

```json
{ "command": "/Users/you/go/bin/cdp", "args": ["-mcp", "-output-dir", "/path/to/captures"] }
```

`cdp` accepts both `-flag` and `--flag`. Run `cdp -help` for the full list.
