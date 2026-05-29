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

// LoadMapsFromDisk scans sourcesDir for .js.map files and loads valid
// sourcemap v3 files. Invalid files are skipped.
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

func bundleURLFromMapPath(sourcesDir, mapPath string) (string, bool) {
	rel, err := filepath.Rel(sourcesDir, mapPath)
	if err != nil {
		return "", false
	}
	parts := strings.SplitN(rel, string(filepath.Separator), 2)
	if len(parts) < 2 {
		return "", false
	}
	origin := parts[0]
	rest := strings.TrimPrefix(parts[1], "_compiled"+string(filepath.Separator))
	rest = strings.TrimSuffix(rest, ".map")
	return "https://" + origin + "/" + filepath.ToSlash(rest), true
}
