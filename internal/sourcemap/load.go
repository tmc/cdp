package sourcemap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// StoredMap is a persisted sourcemap loaded from disk.
type StoredMap struct {
	BundleURL string
	MapJSON   []byte
	Structure *Structure
	MapPath   string
}

// LoadMapsFromDisk scans sourcesDir for .js.map files under an
// origin/_compiled directory and loads valid sourcemap v3 files.
// Invalid files are skipped.
func LoadMapsFromDisk(sourcesDir string) []StoredMap {
	if sourcesDir == "" {
		return nil
	}
	var maps []StoredMap
	_ = filepath.WalkDir(sourcesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".js.map") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil || !isMapV3(data) {
			return nil
		}
		bundleURL, ok := bundleURLFromMapPath(sourcesDir, path)
		if !ok {
			return nil
		}

		structure, _ := ReadStructureSidecar(path)
		maps = append(maps, StoredMap{
			BundleURL: bundleURL,
			MapJSON:   data,
			Structure: structure,
			MapPath:   path,
		})
		return nil
	})
	return maps
}

func isMapV3(data []byte) bool {
	var sm struct {
		Version int `json:"version"`
	}
	return json.Unmarshal(data, &sm) == nil && sm.Version == 3
}

// bundleURLFromMapPath recovers the bundle URL from a map path of the form
// [page-host/][_sources/]origin/_compiled/path.js.map under sourcesDir.
func bundleURLFromMapPath(sourcesDir, mapPath string) (string, bool) {
	rel, err := filepath.Rel(sourcesDir, mapPath)
	if err != nil {
		return "", false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i := 1; i < len(parts)-1; i++ {
		if parts[i] != "_compiled" {
			continue
		}
		rest := strings.TrimSuffix(strings.Join(parts[i+1:], "/"), ".map")
		return "https://" + parts[i-1] + "/" + rest, true
	}
	return "", false
}
