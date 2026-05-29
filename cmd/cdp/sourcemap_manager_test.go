package main

import (
	"testing"

	"github.com/tmc/cdp/internal/sourcemap"
)

func TestSourcemapManagerUpdateAndList(t *testing.T) {
	m := newSourcemapManager()
	sm := m.update("https://example.com/app.js", func(sm *syntheticMap) {
		sm.MapJSON = []byte(`{"version":3}`)
		sm.Sources = &sourcemap.Structure{
			Files: []sourcemap.File{{Path: "src/app.js"}},
		}
	})

	if sm.BundleURL != "https://example.com/app.js" {
		t.Fatalf("BundleURL = %q", sm.BundleURL)
	}
	again := m.update("https://example.com/app.js", func(sm *syntheticMap) {
		sm.Serving = true
		sm.InterceptID = "intercept-1"
	})
	if again != sm {
		t.Fatal("update replaced existing map")
	}
	if got := m.get("https://example.com/app.js"); got == nil || string(got.MapJSON) != `{"version":3}` {
		t.Fatalf("get returned %#v", got)
	}
	if got := m.get("https://example.com/app.js"); !got.Serving || got.InterceptID != "intercept-1" {
		t.Fatalf("updated state = %#v", got)
	}
	if got := m.get("https://example.com/missing.js"); got != nil {
		t.Fatalf("missing get = %#v, want nil", got)
	}

	listed := m.list()
	if len(listed) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(listed))
	}
	if listed[0].Sources == nil || listed[0].Sources.Files[0].Path != "src/app.js" {
		t.Fatalf("listed source = %#v", listed[0].Sources)
	}
}

func TestEnsureSourcemaps(t *testing.T) {
	s := &mcpSession{}
	if s.syntheticMaps != nil {
		t.Fatal("new session has synthetic maps")
	}
	sm := s.ensureSourcemaps()
	if sm == nil || s.syntheticMaps != sm {
		t.Fatal("ensureSourcemaps did not initialize session manager")
	}
	if again := s.ensureSourcemaps(); again != sm {
		t.Fatal("ensureSourcemaps replaced existing session manager")
	}

	im := &InteractiveMode{}
	imMaps := im.ensureSourcemaps()
	if imMaps == nil || im.syntheticMaps != imMaps {
		t.Fatal("ensureSourcemaps did not initialize interactive manager")
	}
	if again := im.ensureSourcemaps(); again != imMaps {
		t.Fatal("ensureSourcemaps replaced existing interactive manager")
	}
}

func TestNilSourcemapManager(t *testing.T) {
	var m *sourcemapManager
	if got := m.get("x"); got != nil {
		t.Fatalf("nil get = %#v, want nil", got)
	}
	if got := m.list(); got != nil {
		t.Fatalf("nil list = %#v, want nil", got)
	}
	if got := m.loadFromDisk(""); got != 0 {
		t.Fatalf("nil loadFromDisk = %d, want 0", got)
	}
}
