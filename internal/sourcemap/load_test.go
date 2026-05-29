package sourcemap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMapsFromDisk(t *testing.T) {
	dir := t.TempDir()
	mapPath, err := WriteMap(dir, "https://example.com/static/app.js", []byte(`{"version":3,"sources":[],"mappings":""}`))
	if err != nil {
		t.Fatalf("WriteMap: %v", err)
	}
	if err := WriteStructureSidecar(mapPath, &Structure{
		Summary: "loaded",
		Files:   []File{{Path: "src/app.js"}},
	}); err != nil {
		t.Fatalf("WriteStructureSidecar: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignore.js.map"), []byte(`{"version":2}`), 0o644); err != nil {
		t.Fatalf("write invalid map: %v", err)
	}

	maps := LoadMapsFromDisk(dir)
	if len(maps) != 1 {
		t.Fatalf("len(maps) = %d, want 1", len(maps))
	}
	m := maps[0]
	if m.BundleURL != "https://example.com/static/app.js" {
		t.Fatalf("BundleURL = %q", m.BundleURL)
	}
	if string(m.MapJSON) != `{"version":3,"sources":[],"mappings":""}` {
		t.Fatalf("MapJSON = %q", m.MapJSON)
	}
	if m.MapPath != mapPath {
		t.Fatalf("MapPath = %q, want %q", m.MapPath, mapPath)
	}
	if m.Structure == nil || len(m.Structure.Files) != 1 || m.Structure.Files[0].Path != "src/app.js" {
		t.Fatalf("Structure = %#v", m.Structure)
	}
}

func TestLoadMapsFromDiskEmpty(t *testing.T) {
	if maps := LoadMapsFromDisk(""); maps != nil {
		t.Fatalf("LoadMapsFromDisk(empty) = %#v, want nil", maps)
	}
	if maps := LoadMapsFromDisk(filepath.Join(t.TempDir(), "missing")); maps != nil {
		t.Fatalf("LoadMapsFromDisk(missing) = %#v, want nil", maps)
	}
}
