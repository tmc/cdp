package parser

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var varPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// SubstituteVariables replaces ${VAR} references with their values.
// Variables are looked up in the following order:
// 1. The provided vars map
// 2. Environment variables
func SubstituteVariables(text string, vars map[string]string) (string, error) {
	result := varPattern.ReplaceAllStringFunc(text, func(match string) string {
		// Extract variable name from ${VAR}
		varName := strings.TrimPrefix(strings.TrimSuffix(match, "}"), "${")

		// Look up in provided vars first
		if val, ok := vars[varName]; ok {
			return val
		}

		// Fall back to environment variables
		if val := os.Getenv(varName); val != "" {
			return val
		}

		// If not found, leave as-is (will be caught by validation later)
		return match
	})

	// Check for unresolved variables
	if varPattern.MatchString(result) {
		unresolved := varPattern.FindAllString(result, -1)
		return result, fmt.Errorf("unresolved variables: %v", unresolved)
	}

	return result, nil
}

// ExtractVariables extracts all ${VAR} references from text.
func ExtractVariables(text string) []string {
	matches := varPattern.FindAllStringSubmatch(text, -1)
	vars := make([]string, 0, len(matches))
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) > 1 {
			varName := match[1]
			if !seen[varName] {
				vars = append(vars, varName)
				seen[varName] = true
			}
		}
	}

	return vars
}

// ValidateVariables checks if all variables in text can be resolved.
func ValidateVariables(text string, vars map[string]string) []string {
	missing := []string{}
	extracted := ExtractVariables(text)

	for _, varName := range extracted {
		if _, ok := vars[varName]; !ok {
			if os.Getenv(varName) == "" {
				missing = append(missing, varName)
			}
		}
	}

	return missing
}
