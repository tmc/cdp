package script

import (
	"bufio"
	"fmt"
	"strings"
	"time"

	"golang.org/x/tools/txtar"
	"gopkg.in/yaml.v3"
)

// Script represents a parsed CDP script
type Script struct {
	Metadata Metadata
	Commands []Command
	Files    map[string][]byte
}

// Metadata holds script configuration
type Metadata struct {
	Author      string        `yaml:"author"`
	Description string        `yaml:"description"`
	Timeout     time.Duration `yaml:"timeout"`
	Headless    bool          `yaml:"headless"`
	Variables   map[string]string `yaml:"variables"`
}

// Command represents a single script command
type Command struct {
	Name string
	Args []string
	Line int
}

// Parse parses a CDP script from a txtar archive
func Parse(data []byte) (*Script, error) {
	archive := txtar.Parse(data)
	script := &Script{
		Files: make(map[string][]byte),
		Metadata: Metadata{
			Timeout: 30 * time.Second, // Default timeout
			Headless: true, // Default headless
		},
	}

	// Process files in the archive
	var mainScript []byte

	for _, f := range archive.Files {
		if f.Name == "meta.yaml" {
			if err := yaml.Unmarshal(f.Data, &script.Metadata); err != nil {
				return nil, fmt.Errorf("failed to parse meta.yaml: %w", err)
			}
		} else if strings.HasSuffix(f.Name, ".cdp") {
			// Assume the first .cdp file or main.cdp is the script
			if mainScript == nil || f.Name == "main.cdp" {
				mainScript = f.Data
			}
		}
		script.Files[f.Name] = f.Data
	}

	if mainScript == nil {
		// If no .cdp file in archive, treat the whole comment section as the script
		// This allows simple scripts without txtar structure
		if len(archive.Files) == 0 {
			mainScript = archive.Comment
		} else {
			return nil, fmt.Errorf("no .cdp file found in archive")
		}
	}

	// Parse commands from the main script
	commands, err := parseCommands(string(mainScript))
	if err != nil {
		return nil, err
	}
	script.Commands = commands

	return script, nil
}

func parseCommands(content string) ([]Command, error) {
	var commands []Command
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0

	var multiLineCmd string
	var multiLineArgs []string
	inMultiLine := false

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle multi-line JS blocks
		if inMultiLine {
			if line == "}" {
				commands = append(commands, Command{
					Name: multiLineCmd,
					Args: []string{strings.Join(multiLineArgs, "\n")},
					Line: lineNum,
				})
				inMultiLine = false
				multiLineCmd = ""
				multiLineArgs = nil
			} else {
				multiLineArgs = append(multiLineArgs, line)
			}
			continue
		}

		if (strings.HasPrefix(line, "js {") || strings.HasPrefix(line, "eval {")) {
			inMultiLine = true
			parts := strings.Fields(line)
			multiLineCmd = parts[0]
			continue
		}

		// Parse single line command
		parts := parseArgs(line)
		if len(parts) > 0 {
			commands = append(commands, Command{
				Name: strings.ToLower(parts[0]),
				Args: parts[1:],
				Line: lineNum,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return commands, nil
}

// parseArgs parses a command line into arguments, handling quoted strings
func parseArgs(line string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for i, r := range line {
		if inQuote {
			if r == quoteChar {
				inQuote = false
				args = append(args, current.String())
				current.Reset()
			} else {
				current.WriteRune(r)
			}
		} else {
			if r == '"' || r == '\'' {
				inQuote = true
				quoteChar = r
			} else if r == ' ' || r == '\t' {
				if current.Len() > 0 {
					args = append(args, current.String())
					current.Reset()
				}
			} else {
				current.WriteRune(r)
			}
		}
		// Handle end of line if not in quote
		if i == len(line)-1 && current.Len() > 0 {
			args = append(args, current.String())
		}
	}

	return args
}
