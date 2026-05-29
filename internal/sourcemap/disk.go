package sourcemap

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// DiskPath returns the on-disk .map path for bundleURL under sourcesDir.
//
// The layout matches saved sources:
//
//	sourcesDir/origin/_compiled/path.js.map
func DiskPath(sourcesDir, bundleURL string) string {
	u, err := url.Parse(bundleURL)
	if err != nil || u.Host == "" {
		return ""
	}
	relPath := strings.TrimPrefix(u.Path, "/")
	if relPath == "" {
		relPath = "index.js"
	}
	return filepath.Join(sourcesDir, u.Host, "_compiled", relPath+".map")
}

// WriteMap writes mapJSON to the bundleURL .map path under sourcesDir.
// It returns an empty path when sourcesDir or bundleURL cannot produce a path.
func WriteMap(sourcesDir, bundleURL string, mapJSON []byte) (string, error) {
	if sourcesDir == "" {
		return "", nil
	}
	path := DiskPath(sourcesDir, bundleURL)
	if path == "" {
		return "", nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	if err := os.WriteFile(path, mapJSON, 0o644); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	return path, nil
}

// StructureSidecarPath returns the JSON sidecar path for a .map file.
func StructureSidecarPath(mapPath string) string {
	if mapPath == "" {
		return ""
	}
	return strings.TrimSuffix(mapPath, ".map") + ".structure.json"
}

// WriteStructureSidecar writes structure as a JSON sidecar next to mapPath.
func WriteStructureSidecar(mapPath string, structure *Structure) error {
	if mapPath == "" || structure == nil {
		return nil
	}
	data, err := json.MarshalIndent(structure, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal structure: %w", err)
	}
	if err := os.WriteFile(StructureSidecarPath(mapPath), data, 0o644); err != nil {
		return fmt.Errorf("write structure: %w", err)
	}
	return nil
}

// ReadStructureSidecar reads the JSON sidecar next to mapPath.
func ReadStructureSidecar(mapPath string) (*Structure, error) {
	data, err := os.ReadFile(StructureSidecarPath(mapPath))
	if err != nil {
		return nil, err
	}
	var structure Structure
	if err := json.Unmarshal(data, &structure); err != nil {
		return nil, fmt.Errorf("unmarshal structure: %w", err)
	}
	if len(structure.Files) == 0 {
		return nil, fmt.Errorf("empty structure")
	}
	return &structure, nil
}
