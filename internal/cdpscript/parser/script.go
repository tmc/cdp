package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tmc/misc/chrome-to-har/internal/cdpscript/ast"
	"github.com/tmc/misc/chrome-to-har/internal/cdpscript/lexer"
	"github.com/tmc/misc/chrome-to-har/internal/cdpscript/types"
)

// ParseScript parses a complete CDP script from txtar format.
func ParseScript(data []byte) (*types.Script, error) {
	// Parse txtar archive
	archive, err := ParseTxtar(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse txtar: %w", err)
	}

	script := &types.Script{
		Helpers:  make(map[string]string),
		TestData: make(map[string][]byte),
		Expected: make(map[string][]byte),
	}

	// Parse metadata if present
	if metaData, ok := archive.GetFile("metadata.yaml"); ok {
		metadata, err := ParseMetadata(metaData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse metadata: %w", err)
		}
		script.Metadata = *metadata
	} else {
		// Use defaults
		script.Metadata = types.Metadata{
			Browser: "chrome",
			Timeout: 30,
			Env:     make(map[string]string),
		}
	}

	// Parse main.cdp commands
	if mainData, ok := archive.GetFile("main.cdp"); ok {
		// Substitute variables in main script
		mainText := string(mainData)
		mainText, err = SubstituteVariables(mainText, script.Metadata.Env)
		if err != nil {
			// Non-fatal: just log that some variables couldn't be resolved
			// They might be resolved at runtime
		}

		l := lexer.New(mainText)
		p := NewParser(l)
		commands := p.ParseCommands()

		if len(p.Errors()) > 0 {
			return nil, fmt.Errorf("parse errors: %v", strings.Join(p.Errors(), "; "))
		}

		script.Main = &types.CommandList{Commands: commands}
	}

	// Parse assertions if present
	if assertData, ok := archive.GetFile("assertions.yaml"); ok {
		assertions, err := ParseAssertions(assertData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse assertions: %w", err)
		}
		script.Assertions = assertions
	}

	// Collect helper JavaScript files
	for _, file := range archive.Files {
		if strings.HasSuffix(file.Name, ".js") {
			script.Helpers[file.Name] = string(file.Data)
		}
	}

	// Collect test data files (JSON, YAML, etc.)
	for _, file := range archive.Files {
		if strings.HasSuffix(file.Name, ".json") || strings.HasSuffix(file.Name, ".yaml") {
			if !strings.HasPrefix(file.Name, "expected/") {
				script.TestData[file.Name] = file.Data
			}
		}
	}

	// Collect expected output files
	for _, file := range archive.Files {
		if strings.HasPrefix(file.Name, "expected/") {
			script.Expected[file.Name] = file.Data
		}
	}

	return script, nil
}

// ParseScriptFile parses a CDP script from a file.
func ParseScriptFile(filename string) (*types.Script, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read script file: %w", err)
	}

	// Remove shebang if present
	if len(data) > 2 && data[0] == '#' && data[1] == '!' {
		// Find end of first line
		for i, b := range data {
			if b == '\n' {
				data = data[i+1:]
				break
			}
		}
	}

	script, err := ParseScript(data)
	if err != nil {
		return nil, err
	}

	// Resolve imports relative to script file directory
	if len(script.Metadata.Imports) > 0 {
		scriptDir := filepath.Dir(filename)
		for _, importPath := range script.Metadata.Imports {
			resolvedPath := importPath
			if !filepath.IsAbs(importPath) {
				resolvedPath = filepath.Join(scriptDir, importPath)
			}

			// Parse imported script
			importedScript, err := ParseScriptFile(resolvedPath)
			if err != nil {
				return nil, fmt.Errorf("failed to parse import %s: %w", importPath, err)
			}

			// Merge helpers and test data from imported script
			for name, helper := range importedScript.Helpers {
				if _, exists := script.Helpers[name]; !exists {
					script.Helpers[name] = helper
				}
			}
			for name, data := range importedScript.TestData {
				if _, exists := script.TestData[name]; !exists {
					script.TestData[name] = data
				}
			}
		}
	}

	return script, nil
}

// CommandList adapter to satisfy types.CommandList interface
type commandListAdapter struct {
	commands []ast.Command
}

func (c *commandListAdapter) String() string {
	var sb strings.Builder
	for _, cmd := range c.commands {
		sb.WriteString(cmd.String())
		sb.WriteString("\n")
	}
	return sb.String()
}
