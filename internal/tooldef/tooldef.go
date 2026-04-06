// Package tooldef parses .cdp tool definition files for the MCP server.
//
// A .cdp file has Go-directive-style header comments followed by a cdpscript body:
//
//	# name: extract_price
//	# description: Extract product price from page
//	# input: selector string "CSS selector for price element"
//	# readonly: true
//
//	extract $selector
//	stdout '\d+\.\d+'
package tooldef

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ToolDef represents a parsed tool definition from a .cdp file.
type ToolDef struct {
	Name        string
	Description string
	Inputs      []InputDef
	ReadOnly    bool
	Script      string // the cdpscript body (everything after headers)
	SourcePath  string // path to the .cdp file (for error messages)
}

// InputDef represents a single input parameter.
type InputDef struct {
	Name        string
	Type        string // "string", "int", "bool"
	Description string
	Optional    bool
}

// Parse reads a .cdp file from data and returns a ToolDef.
// Lines starting with "# " are header directives until the first non-comment,
// non-blank line. Everything after is the script body.
func Parse(data []byte, sourcePath string) (*ToolDef, error) {
	def := &ToolDef{SourcePath: sourcePath}
	lines := strings.Split(string(data), "\n")

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			i++
			continue
		}
		if !strings.HasPrefix(trimmed, "# ") {
			break
		}
		directive := strings.TrimPrefix(trimmed, "# ")
		if err := parseDirective(def, directive); err != nil {
			return nil, fmt.Errorf("parse %s line %d: %w", sourcePath, i+1, err)
		}
		i++
	}

	// Skip blank lines between headers and body.
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}

	if i < len(lines) {
		def.Script = strings.Join(lines[i:], "\n")
		def.Script = strings.TrimRight(def.Script, "\n")
	}

	if def.Name == "" {
		return nil, fmt.Errorf("parse %s: missing required name directive", sourcePath)
	}
	return def, nil
}

func parseDirective(def *ToolDef, directive string) error {
	key, value, ok := strings.Cut(directive, ":")
	if !ok {
		return fmt.Errorf("invalid directive: %q", directive)
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)

	switch key {
	case "name":
		def.Name = value
	case "description":
		def.Description = value
	case "readonly":
		def.ReadOnly = value == "true"
	case "input":
		inp, err := parseInput(value)
		if err != nil {
			return fmt.Errorf("input directive: %w", err)
		}
		def.Inputs = append(def.Inputs, inp)
	default:
		// Ignore unknown directives for forward compatibility.
	}
	return nil
}

func parseInput(s string) (InputDef, error) {
	var inp InputDef

	// Extract quoted description if present.
	var desc string
	if idx := strings.IndexByte(s, '"'); idx >= 0 {
		end := strings.IndexByte(s[idx+1:], '"')
		if end < 0 {
			return inp, fmt.Errorf("unterminated quoted description in %q", s)
		}
		desc = s[idx+1 : idx+1+end]
		// Remove the quoted part and rejoin.
		s = s[:idx] + s[idx+1+end+1:]
	}

	tokens := strings.Fields(s)
	if len(tokens) < 2 {
		return inp, fmt.Errorf("need at least name and type, got %q", s)
	}
	inp.Name = tokens[0]
	inp.Type = tokens[1]
	inp.Description = desc

	for _, tok := range tokens[2:] {
		if tok == "optional" {
			inp.Optional = true
		}
	}
	return inp, nil
}

// ParseFile reads a .cdp file from disk and parses it.
func ParseFile(path string) (*ToolDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tool definition: %w", err)
	}
	return Parse(data, path)
}

// LoadDir scans dir for *.cdp files and parses each one.
// Files that fail to parse are logged to stderr but do not stop the load.
func LoadDir(dir string) ([]*ToolDef, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.cdp"))
	if err != nil {
		return nil, fmt.Errorf("scan tool directory: %w", err)
	}
	var defs []*ToolDef
	for _, path := range matches {
		def, err := ParseFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tooldef: warning: %v\n", err)
			continue
		}
		defs = append(defs, def)
	}
	return defs, nil
}

// InputSchema returns a JSON Schema object suitable for MCP tool registration.
func (t *ToolDef) InputSchema() map[string]any {
	properties := make(map[string]any, len(t.Inputs))
	var required []string

	for _, inp := range t.Inputs {
		prop := map[string]any{
			"type": goTypeToJSONSchema(inp.Type),
		}
		if inp.Description != "" {
			prop["description"] = inp.Description
		}
		properties[inp.Name] = prop
		if !inp.Optional {
			required = append(required, inp.Name)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func goTypeToJSONSchema(t string) string {
	switch t {
	case "int":
		return "integer"
	case "bool":
		return "boolean"
	default:
		return "string"
	}
}

// Generate produces .cdp file content from a ToolDef (inverse of Parse).
func Generate(def *ToolDef) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# name: %s\n", def.Name)
	if def.Description != "" {
		fmt.Fprintf(&b, "# description: %s\n", def.Description)
	}
	for _, inp := range def.Inputs {
		b.WriteString("# input: ")
		b.WriteString(inp.Name)
		b.WriteString(" ")
		b.WriteString(inp.Type)
		if inp.Description != "" {
			fmt.Fprintf(&b, " %q", inp.Description)
		}
		if inp.Optional {
			b.WriteString(" optional")
		}
		b.WriteString("\n")
	}
	if def.ReadOnly {
		b.WriteString("# readonly: true\n")
	}
	if def.Script != "" {
		b.WriteString("\n")
		b.WriteString(def.Script)
		b.WriteString("\n")
	}
	return []byte(b.String())
}
