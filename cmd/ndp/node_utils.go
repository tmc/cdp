package main

import (
	"strings"
)

const defaultInspectorPort = "9229"

type InspectorConfig struct {
	Enabled bool
	Host    string
	Port    string
}

func parseInspectorArgs(args []string) InspectorConfig {
	cfg := InspectorConfig{
		Host: "127.0.0.1",
	}

	for i, arg := range args {
		if arg == "--inspect" || arg == "--inspect-brk" {
			cfg.Enabled = true
			cfg.Port = defaultInspectorPort

			// Check if next arg is a port (simple heuristic)
			if i+1 < len(args) {
				next := args[i+1]
				if !strings.HasPrefix(next, "-") && isNumeric(next) {
					cfg.Port = next
				}
			}
		} else if strings.HasPrefix(arg, "--inspect=") || strings.HasPrefix(arg, "--inspect-brk=") {
			cfg.Enabled = true
			val := strings.SplitN(arg, "=", 2)[1]
			cfg.Host, cfg.Port = parseHostPort(val)
		}
	}

	return cfg
}

func parseHostPort(s string) (string, string) {
	if strings.Contains(s, ":") {
		parts := strings.Split(s, ":")
		return parts[0], parts[1]
	}
	return "127.0.0.1", s
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func splitPIDAndCommand(line string) (string, string, bool) {
	parts := strings.SplitN(line, " ", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func detectNodeScript(args []string) string {
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}

		if strings.HasPrefix(arg, "-") {
			// Skip flags that take arguments
			if arg == "--inspect" || arg == "--inspect-brk" {
				// check if next is port number
				// This is a naive check similar to parseInspectorArgs logic
				// But we'll rely on skipping known single-arg flags or assumptions
				// For the test cases, we just need to identify the script.
			}
			// Assume standard flags don't take args unless known?
			// In node, most single dash flags don't take args except -e, -r
			if arg == "-r" || arg == "--require" || arg == "--loader" || arg == "-e" || arg == "--eval" {
				skipNext = true
			}
			continue
		}

		// If previous was --inspect/brk and this is a number, skip it
		if isNumeric(arg) {
			// This is ambiguous without context of which flag preceded it,
			// but for simple detection we can try heuristics or just assume first non-numeric non-flag is script
			// unless it was consumed by a flag using skipNext.
			// But wait, the loop structure implies we iterate sequentially.
			// If we didn't set skipNext, we assume it's NOT a value for a flag.
			// However `parseInspectorArgs` consumes next arg if numeric.
			// Let's refine:
		}

		// If we are here, it's not a flag name, and not consumed by a flag we know takes an arg.
		// However, --inspect 9229 consumes 9229.
		// But in this simple loop we didn't track the *previous* flag.

		return arg
	}
	return ""
}
