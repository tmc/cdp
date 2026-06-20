# cdp as an MCP plugin

`cdp` ships an MCP server (`cdp --mcp`) that exposes its Chrome DevTools
Protocol toolkit — navigate, click/type, screenshot, PDF, extract text/HTML,
capture HAR/HARL, intercept and block requests, manage cookies and storage, and
emulate devices. Captured network and source output is secret-redacted by
default.

The same server backs three packaging targets:

| Target | Format | Files |
|---|---|---|
| Claude Code | repo-as-plugin / marketplace | [`.claude-plugin/plugin.json`](../.claude-plugin/plugin.json), [`.mcp.json`](../.mcp.json), [`.claude-plugin/marketplace.json`](../.claude-plugin/marketplace.json) |
| Claude Desktop | MCP bundle (`.mcpb`) | [`plugins/mcpb/`](../plugins/mcpb/) |
| Codex | `config.toml` | [`plugins/codex/`](../plugins/codex/) |

## Prerequisite

All three launch the `cdp` binary from `PATH`; none embed it. Install it
(Go 1.26+) and make sure your Go bin directory is on `PATH`:

```bash
go install github.com/tmc/cdp/cmd/cdp@latest
```

## Claude Code

The repository is itself a Claude Code plugin and a single-plugin marketplace.

```bash
/plugin marketplace add tmc/cdp
/plugin install cdp@cdp
```

This wires up the `cdp` MCP server (`cdp --mcp --headless`, see
[`.mcp.json`](../.mcp.json)) and the browser-automation skills under
[`skills/`](../skills). To develop locally without a marketplace:

```bash
claude --plugin-dir .
```

## Claude Desktop

Build and install the MCP bundle:

```bash
cd plugins/mcpb
mcpb pack . cdp.mcpb        # requires the mcpb CLI
```

Open `cdp.mcpb` with Claude Desktop and enable it in Settings → Extensions.
See [`plugins/mcpb/README.md`](../plugins/mcpb/README.md) for customizing the
launch (visible browser, capture directory, extra tools).

## Codex

```bash
codex mcp add cdp -- cdp --mcp --headless
```

Or append the block in [`plugins/codex/config.toml`](../plugins/codex/config.toml)
to `~/.codex/config.toml`. See [`plugins/codex/README.md`](../plugins/codex/README.md).

## Launch options

`cdp --mcp` takes the same flags as the CLI. Common adjustments:

- drop `--headless` to watch Chrome while the agent drives it;
- `--output-dir <dir>` to persist HAR/HARL captures and saved page sources;
- `--enable-inspect` to expose the reversing/inspection tools;
- `--tools-dir <dir>` to register custom `.cdp` tool definitions;
- `--no-scrub` to disable secret redaction (on by default).

Run `cdp --help` for the full list.
