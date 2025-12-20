package script

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantCmds int
		wantErr  bool
	}{
		{
			name: "basic script",
			input: `
-- main.cdp --
goto https://example.com
wait #content
click button
`,
			wantCmds: 3,
			wantErr:  false,
		},
		{
			name: "script with metadata",
			input: `
-- meta.yaml --
timeout: 10s
headless: false

-- main.cdp --
goto https://example.com
`,
			wantCmds: 1,
			wantErr:  false,
		},
		{
			name: "script with comments and empty lines",
			input: `
-- main.cdp --
# This is a comment

goto https://example.com
# Another comment
wait #content

`,
			wantCmds: 2,
			wantErr:  false,
		},
		{
			name: "script with quoted args",
			input: `
-- main.cdp --
fill #input "some text with spaces"
assert text "some 'quoted' text"
`,
			wantCmds: 2,
			wantErr:  false,
		},
		{
			name: "script with multi-line js",
			input: `
-- main.cdp --
js {
  console.log("hello");
  return 42;
}
`,
			wantCmds: 1,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Parse([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if len(s.Commands) != tt.wantCmds {
					t.Errorf("Parse() got %d commands, want %d", len(s.Commands), tt.wantCmds)
				}
			}
		})
	}
}

func TestParseMetadata(t *testing.T) {
	input := `
-- meta.yaml --
author: "Test User"
timeout: 60s
headless: true
variables:
  BASE_URL: "https://example.com"

-- main.cdp --
goto ${BASE_URL}
`
	s, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if s.Metadata.Author != "Test User" {
		t.Errorf("Metadata.Author = %s, want 'Test User'", s.Metadata.Author)
	}
	if s.Metadata.Timeout != 60*time.Second {
		t.Errorf("Metadata.Timeout = %v, want 60s", s.Metadata.Timeout)
	}
	if !s.Metadata.Headless {
		t.Errorf("Metadata.Headless = %v, want true", s.Metadata.Headless)
	}
	if s.Metadata.Variables["BASE_URL"] != "https://example.com" {
		t.Errorf("Metadata.Variables['BASE_URL'] = %s, want 'https://example.com'", s.Metadata.Variables["BASE_URL"])
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{
			input: `cmd arg1 arg2`,
			want:  []string{"cmd", "arg1", "arg2"},
		},
		{
			input: `cmd "arg with spaces" arg2`,
			want:  []string{"cmd", "arg with spaces", "arg2"},
		},
		{
			input: `cmd 'single quoted' arg2`,
			want:  []string{"cmd", "single quoted", "arg2"},
		},
		{
			input: `cmd "nested 'quotes'"`,
			want:  []string{"cmd", "nested 'quotes'"},
		},
		{
			input: `cmd`,
			want:  []string{"cmd"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseArgs(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("parseArgs() got %d args, want %d", len(got), len(tt.want))
				return
			}
			for i, arg := range got {
				if arg != tt.want[i] {
					t.Errorf("parseArgs() arg[%d] = %s, want %s", i, arg, tt.want[i])
				}
			}
		})
	}
}
