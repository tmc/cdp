package parser

import (
	"fmt"
	"time"

	"github.com/tmc/misc/chrome-to-har/internal/cdpscript/types"
	"gopkg.in/yaml.v3"
)

// ParseMetadata parses the metadata.yaml section of a CDP script.
func ParseMetadata(data []byte) (*types.Metadata, error) {
	var meta types.Metadata

	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	// Set defaults
	if meta.Browser == "" {
		meta.Browser = "chrome"
	}
	if meta.Timeout == 0 {
		meta.Timeout = 30 * time.Second
	}
	if meta.Env == nil {
		meta.Env = make(map[string]string)
	}

	return &meta, nil
}
