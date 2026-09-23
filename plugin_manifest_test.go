package cdp_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/tmc/cdp/internal/docscan"
)

// server is the launch configuration shared by every plugin manifest.
type server struct {
	Command string   `json:"command" toml:"command"`
	Args    []string `json:"args" toml:"args"`
}

type claudePlugin struct {
	Name       string                     `json:"name"`
	Version    string                     `json:"version"`
	MCPServers map[string]server          `json:"mcpServers"`
	UserConfig map[string]json.RawMessage `json:"userConfig"`
}

type claudeMarketplace struct {
	Name    string `json:"name"`
	Plugins []struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	} `json:"plugins"`
}

type mcpbManifest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Server  struct {
		MCPConfig server `json:"mcp_config"`
	} `json:"server"`
	UserConfig map[string]json.RawMessage `json:"user_config"`
}

type codexConfig struct {
	MCPServers map[string]server `toml:"mcp_servers"`
}

// TestPluginManifests checks that the Claude Code, Claude Desktop, and Codex
// packaging agree on name and version, and that every launch flag is one cdp
// defines.
func TestPluginManifests(t *testing.T) {
	var plugin claudePlugin
	decodeJSON(t, ".claude-plugin/plugin.json", &plugin)
	var market claudeMarketplace
	decodeJSON(t, ".claude-plugin/marketplace.json", &market)
	var bundle mcpbManifest
	decodeJSON(t, "plugins/mcpb/manifest.json", &bundle)
	var codex codexConfig
	if _, err := toml.DecodeFile("plugins/codex/config.toml", &codex); err != nil {
		t.Fatal(err)
	}

	const name = "cdp"
	if plugin.Name != name {
		t.Errorf("plugin.json name = %q, want %q", plugin.Name, name)
	}
	if market.Name != name || len(market.Plugins) != 1 || market.Plugins[0].Name != name || market.Plugins[0].Source != "." {
		t.Errorf("marketplace.json = %+v, want one plugin %q with source \".\"", market, name)
	}
	if bundle.Name != name {
		t.Errorf("mcpb manifest name = %q, want %q", bundle.Name, name)
	}
	if plugin.Version == "" || bundle.Version != plugin.Version {
		t.Errorf("versions: plugin.json %q, mcpb manifest %q; want equal and non-empty", plugin.Version, bundle.Version)
	}
	pack := "cdp-" + plugin.Version + ".mcpb"
	for _, doc := range []string{"docs/plugins.md", "plugins/mcpb/README.md"} {
		data, err := os.ReadFile(doc)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), pack) {
			t.Errorf("%s does not name %s", doc, pack)
		}
	}

	// The repository root is the plugin, so a root .mcp.json would also be
	// a project-scoped MCP config where ${user_config.*} is never substituted.
	if _, err := os.Stat(".mcp.json"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("root .mcp.json exists; declare the server in .claude-plugin/plugin.json")
	}

	flags, err := docscan.Flags("cmd/cdp")
	if err != nil {
		t.Fatal(err)
	}
	defined := make(map[string]bool)
	for _, f := range flags {
		defined[f] = true
	}

	tests := []struct {
		name       string
		server     server
		ok         bool
		userConfig map[string]json.RawMessage
	}{
		{"plugin.json", plugin.MCPServers[name], plugin.MCPServers != nil, plugin.UserConfig},
		{"mcpb manifest", bundle.Server.MCPConfig, true, bundle.UserConfig},
		{"codex config.toml", codex.MCPServers[name], codex.MCPServers != nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.ok || tt.server.Command == "" {
				t.Fatalf("no %q server", name)
			}
			if !slices.Contains(tt.server.Args, "-mcp") {
				t.Errorf("args %q do not include -mcp", tt.server.Args)
			}
			for _, s := range append([]string{tt.server.Command}, tt.server.Args...) {
				for _, m := range userConfigRef.FindAllStringSubmatch(s, -1) {
					if _, ok := tt.userConfig[m[1]]; !ok {
						t.Errorf("%q references undeclared user config %q", s, m[1])
					}
				}
			}
			for _, arg := range tt.server.Args {
				if !strings.HasPrefix(arg, "-") {
					continue
				}
				f := strings.TrimLeft(arg, "-")
				f, _, _ = strings.Cut(f, "=")
				if !defined[f] {
					t.Errorf("arg %q: cdp defines no flag -%s", arg, f)
				}
			}
		})
	}
}

var userConfigRef = regexp.MustCompile(`\$\{user_config\.([A-Za-z0-9_]+)\}`)

func decodeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}
