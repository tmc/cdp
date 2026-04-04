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
	// Handle IPv6 addresses like "::1:9240" — the port is the last segment.
	if idx := strings.LastIndex(s, ":"); idx >= 0 {
		host := s[:idx]
		port := s[idx+1:]
		if host == "" {
			host = "127.0.0.1"
		}
		return host, port
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
			// Skip flags that take a following argument.
			switch arg {
			case "--inspect", "--inspect-brk":
				skipNext = true // next arg may be a port number
			case "-r", "--require", "--loader", "-e", "--eval":
				skipNext = true
			}
			continue
		}

		// Skip bare numeric args (port numbers consumed by --inspect/--inspect-brk).
		if isNumeric(arg) {
			continue
		}

		return arg
	}
	return ""
}
