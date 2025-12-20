package parser

import (
	"fmt"

	"github.com/tmc/misc/chrome-to-har/internal/cdpscript/types"
	"gopkg.in/yaml.v3"
)

// ParseAssertions parses the assertions.yaml section of a CDP script.
func ParseAssertions(data []byte) ([]types.Assertion, error) {
	var assertions []types.Assertion

	if err := yaml.Unmarshal(data, &assertions); err != nil {
		return nil, fmt.Errorf("failed to parse assertions: %w", err)
	}

	// Validate assertions
	for i, a := range assertions {
		if a.Type == "" {
			return nil, fmt.Errorf("assertion %d: missing type", i)
		}
	}

	return assertions, nil
}
