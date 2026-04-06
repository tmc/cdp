package tooldef

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ToolDef
		wantErr bool
	}{
		{
			name: "all directives",
			input: `# name: extract_price
# description: Extract product price from page
# input: selector string "CSS selector for price element"
# input: currency string "Currency code" optional
# readonly: true

extract $selector
stdout '\d+\.\d+'`,
			want: ToolDef{
				Name:        "extract_price",
				Description: "Extract product price from page",
				Inputs: []InputDef{
					{Name: "selector", Type: "string", Description: "CSS selector for price element"},
					{Name: "currency", Type: "string", Description: "Currency code", Optional: true},
				},
				ReadOnly: true,
				Script:   "extract $selector\nstdout '\\d+\\.\\d+'",
			},
		},
		{
			name:    "missing name",
			input:   "# description: no name here\n\nscript body",
			wantErr: true,
		},
		{
			name: "name only",
			input: `# name: simple

navigate https://example.com`,
			want: ToolDef{
				Name:   "simple",
				Script: "navigate https://example.com",
			},
		},
		{
			name: "input without description",
			input: `# name: basic
# input: url string
# input: count int optional`,
			want: ToolDef{
				Name: "basic",
				Inputs: []InputDef{
					{Name: "url", Type: "string"},
					{Name: "count", Type: "int", Optional: true},
				},
			},
		},
		{
			name: "bool input",
			input: `# name: toggle
# input: verbose bool "Enable verbose output"`,
			want: ToolDef{
				Name: "toggle",
				Inputs: []InputDef{
					{Name: "verbose", Type: "bool", Description: "Enable verbose output"},
				},
			},
		},
		{
			name:  "no script body",
			input: "# name: empty\n",
			want: ToolDef{
				Name: "empty",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.input), "test.cdp")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.want.Name)
			}
			if got.Description != tt.want.Description {
				t.Errorf("Description = %q, want %q", got.Description, tt.want.Description)
			}
			if got.ReadOnly != tt.want.ReadOnly {
				t.Errorf("ReadOnly = %v, want %v", got.ReadOnly, tt.want.ReadOnly)
			}
			if got.Script != tt.want.Script {
				t.Errorf("Script = %q, want %q", got.Script, tt.want.Script)
			}
			if len(got.Inputs) != len(tt.want.Inputs) {
				t.Fatalf("len(Inputs) = %d, want %d", len(got.Inputs), len(tt.want.Inputs))
			}
			for i, inp := range got.Inputs {
				want := tt.want.Inputs[i]
				if inp.Name != want.Name {
					t.Errorf("Inputs[%d].Name = %q, want %q", i, inp.Name, want.Name)
				}
				if inp.Type != want.Type {
					t.Errorf("Inputs[%d].Type = %q, want %q", i, inp.Type, want.Type)
				}
				if inp.Description != want.Description {
					t.Errorf("Inputs[%d].Description = %q, want %q", i, inp.Description, want.Description)
				}
				if inp.Optional != want.Optional {
					t.Errorf("Inputs[%d].Optional = %v, want %v", i, inp.Optional, want.Optional)
				}
			}
		})
	}
}

func TestInputSchema(t *testing.T) {
	def := &ToolDef{
		Name: "test",
		Inputs: []InputDef{
			{Name: "url", Type: "string", Description: "Target URL"},
			{Name: "count", Type: "int"},
			{Name: "verbose", Type: "bool", Optional: true},
		},
	}
	schema := def.InputSchema()

	if schema["type"] != "object" {
		t.Errorf("schema type = %v, want object", schema["type"])
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties not a map")
	}
	if len(props) != 3 {
		t.Errorf("len(properties) = %d, want 3", len(props))
	}

	urlProp := props["url"].(map[string]any)
	if urlProp["type"] != "string" {
		t.Errorf("url type = %v, want string", urlProp["type"])
	}
	if urlProp["description"] != "Target URL" {
		t.Errorf("url description = %v, want Target URL", urlProp["description"])
	}

	countProp := props["count"].(map[string]any)
	if countProp["type"] != "integer" {
		t.Errorf("count type = %v, want integer", countProp["type"])
	}

	verboseProp := props["verbose"].(map[string]any)
	if verboseProp["type"] != "boolean" {
		t.Errorf("verbose type = %v, want boolean", verboseProp["type"])
	}

	required := schema["required"].([]string)
	if len(required) != 2 {
		t.Fatalf("len(required) = %d, want 2", len(required))
	}
	if required[0] != "url" || required[1] != "count" {
		t.Errorf("required = %v, want [url count]", required)
	}
}

func TestGenerate(t *testing.T) {
	def := &ToolDef{
		Name:        "extract_price",
		Description: "Extract product price",
		Inputs: []InputDef{
			{Name: "selector", Type: "string", Description: "CSS selector"},
			{Name: "currency", Type: "string", Optional: true},
		},
		ReadOnly: true,
		Script:   "extract $selector\nstdout '\\d+'",
	}

	data := Generate(def)
	got, err := Parse(data, "generated.cdp")
	if err != nil {
		t.Fatalf("round-trip parse: %v", err)
	}
	if got.Name != def.Name {
		t.Errorf("Name = %q, want %q", got.Name, def.Name)
	}
	if got.Description != def.Description {
		t.Errorf("Description = %q, want %q", got.Description, def.Description)
	}
	if got.ReadOnly != def.ReadOnly {
		t.Errorf("ReadOnly = %v, want %v", got.ReadOnly, def.ReadOnly)
	}
	if got.Script != def.Script {
		t.Errorf("Script = %q, want %q", got.Script, def.Script)
	}
	if len(got.Inputs) != len(def.Inputs) {
		t.Fatalf("len(Inputs) = %d, want %d", len(got.Inputs), len(def.Inputs))
	}
	for i, inp := range got.Inputs {
		want := def.Inputs[i]
		if inp.Name != want.Name || inp.Type != want.Type || inp.Description != want.Description || inp.Optional != want.Optional {
			t.Errorf("Inputs[%d] = %+v, want %+v", i, inp, want)
		}
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()

	// Write a valid file.
	valid := `# name: tool_a
# description: Tool A

navigate https://example.com`
	os.WriteFile(filepath.Join(dir, "tool_a.cdp"), []byte(valid), 0644)

	// Write another valid file.
	valid2 := `# name: tool_b

click #button`
	os.WriteFile(filepath.Join(dir, "tool_b.cdp"), []byte(valid2), 0644)

	// Write an invalid file (no name).
	invalid := `# description: missing name`
	os.WriteFile(filepath.Join(dir, "bad.cdp"), []byte(invalid), 0644)

	// Write a non-.cdp file (should be ignored).
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a tool"), 0644)

	defs, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("len(defs) = %d, want 2", len(defs))
	}
}
