package sourcemap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiskPath(t *testing.T) {
	got := filepath.ToSlash(DiskPath("/tmp/sources", "https://example.com/static/app.js"))
	want := "/tmp/sources/example.com/_compiled/static/app.js.map"
	if got != want {
		t.Fatalf("DiskPath = %q, want %q", got, want)
	}
	if got := DiskPath("/tmp/sources", "not a url"); got != "" {
		t.Fatalf("DiskPath invalid URL = %q, want empty", got)
	}
}

func TestWriteMapAndStructureSidecar(t *testing.T) {
	dir := t.TempDir()
	mapPath, err := WriteMap(dir, "https://example.com/app.js", []byte(`{"version":3}`))
	if err != nil {
		t.Fatalf("WriteMap: %v", err)
	}
	if mapPath == "" {
		t.Fatal("WriteMap path is empty")
	}
	data, err := os.ReadFile(mapPath)
	if err != nil {
		t.Fatalf("read map: %v", err)
	}
	if string(data) != `{"version":3}` {
		t.Fatalf("map data = %q", data)
	}

	structure := &Structure{
		Summary: "test",
		Files:   []File{{Path: "src/app.js"}},
	}
	if err := WriteStructureSidecar(mapPath, structure); err != nil {
		t.Fatalf("WriteStructureSidecar: %v", err)
	}
	if _, err := os.Stat(StructureSidecarPath(mapPath)); err != nil {
		t.Fatalf("stat sidecar: %v", err)
	}

	got, err := ReadStructureSidecar(mapPath)
	if err != nil {
		t.Fatalf("ReadStructureSidecar: %v", err)
	}
	if got.Summary != "test" || len(got.Files) != 1 || got.Files[0].Path != "src/app.js" {
		t.Fatalf("structure = %#v", got)
	}
}

func TestWriteMapNoSourcesDir(t *testing.T) {
	path, err := WriteMap("", "https://example.com/app.js", []byte("{}"))
	if err != nil {
		t.Fatalf("WriteMap: %v", err)
	}
	if path != "" {
		t.Fatalf("path = %q, want empty", path)
	}
}
