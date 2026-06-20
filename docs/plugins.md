# cdp as an MCP plugin

`cdp` ships an MCP server (`cdp -mcp`) that exposes its Chrome DevTools
Protocol toolkit — navigate, click/type, screenshot, PDF, extract text/HTML,
capture HAR/HARL, intercept and block requests, manage cookies and storage, and
emulate devices. Captured HAR and source-capture output is secret-redacted by
default.

The same server backs three packaging targets:

| Target | Format | Files |
|---|---|---|
| Claude Code | repo-as-plugin / marketplace | [`.claude-plugin/plugin.json`](../.claude-plugin/plugin.json), [`.mcp.json`](../.mcp.json), [`.claude-plugin/marketplace.json`](../.claude-plugin/marketplace.json) |
| Claude Desktop | MCP bundle (`.mcpb`) | [`plugins/mcpb/`](../plugins/mcpb/) |
| Codex | `config.toml` | [`plugins/codex/`](../plugins/codex/) |

## Prerequisite

All three launch the `cdp` binary; none embed it. Install it (Go 1.26+):

```bash
go install github.com/tmc/cdp/cmd/cdp@latest
```

This lands at `$(go env GOPATH)/bin/cdp` (typically `~/go/bin/cdp`). The Claude
Code and Codex integrations resolve `cdp` from your shell `PATH`, so add that
directory to `PATH`. The Claude Desktop bundle instead launches the binary by
absolute path (a GUI app does not inherit your shell `PATH`), defaulting to
`~/go/bin/cdp`.

## Claude Code

The repository is itself a Claude Code plugin and a single-plugin marketplace.
Once the repository is published to its default branch on GitHub:

```bash
/plugin marketplace add tmc/cdp
/plugin install cdp@cdp
```

`/plugin marketplace add` reads `marketplace.json` from the repository's default
branch. For local development, load the plugin directly from a checkout:

```bash
claude --plugin-dir .
```

Either way you get the `cdp` MCP server (see [`.mcp.json`](../.mcp.json)) and
the browser-automation skills under [`skills/`](../skills). The plugin prompts
for a "Run Chrome headless" toggle at enable time (default on); turn it off to
watch the browser.

## Claude Desktop

Build the MCP bundle:

```bash
cd plugins/mcpb
npx -y @anthropic-ai/mcpb pack .   # writes cdp-0.1.0.mcpb
```

Open the `.mcpb` with Claude Desktop and enable it in Settings → Extensions.
Set "Path to the cdp binary" if your install isn't at the `~/go/bin/cdp`
default. See [`plugins/mcpb/README.md`](../plugins/mcpb/README.md) for
customizing the launch (visible browser, capture directory, extra tools).

## Codex

```bash
codex mcp add cdp -- cdp -mcp -headless
```

Or append the block in [`plugins/codex/config.toml`](../plugins/codex/config.toml)
to `~/.codex/config.toml`. See [`plugins/codex/README.md`](../plugins/codex/README.md).

## Launch options

`cdp -mcp` takes the same flags as the CLI (`cdp` accepts both `-flag` and
`--flag`). Common adjustments:

- drop `-headless` to watch Chrome while the agent drives it;
- `-output-dir <dir>` to persist HAR/HARL captures and saved page sources;
- `-enable-inspect` to expose the reversing/inspection tools;
- `-tools-dir <dir>` to register custom `.cdp` tool definitions;
- `-no-scrub` to disable secret redaction (on by default).

Run `cdp -help` for the full list.
