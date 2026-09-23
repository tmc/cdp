package main

import (
	"sync"

	"github.com/tmc/cdp/internal/sourcemap"
)

// syntheticMap holds a generated sourcemap for a bundle URL.
type syntheticMap struct {
	BundleURL   string               `json:"bundle_url"`
	MapJSON     []byte               `json:"-"`
	Sources     *sourcemap.Structure `json:"sources,omitempty"`
	Serving     bool                 `json:"serving"`
	InterceptID string               `json:"intercept_id,omitempty"`
	MapPath     string               `json:"map_path,omitempty"` // on-disk path to .map file
	LogEntries  int                  `json:"log_entries,omitempty"`
}

// sourcemapManager manages synthetic maps keyed by bundle URL.
type sourcemapManager struct {
	mu   sync.Mutex
	maps map[string]*syntheticMap
}

func newSourcemapManager() *sourcemapManager {
	return &sourcemapManager{maps: make(map[string]*syntheticMap)}
}

func (s *mcpSession) ensureSourcemaps() *sourcemapManager {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.syntheticMaps == nil {
		s.syntheticMaps = newSourcemapManager()
	}
	return s.syntheticMaps
}

// sourcemaps returns the session's sourcemap manager, or nil if no bundle
// has been analyzed yet.
func (s *mcpSession) sourcemaps() *sourcemapManager {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.syntheticMaps
}

func (im *InteractiveMode) ensureSourcemaps() *sourcemapManager {
	if im.syntheticMaps == nil {
		im.syntheticMaps = newSourcemapManager()
	}
	return im.syntheticMaps
}

func (m *sourcemapManager) get(url string) *syntheticMap {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.maps[url]
}

func (m *sourcemapManager) update(url string, fn func(*syntheticMap)) *syntheticMap {
	m.mu.Lock()
	defer m.mu.Unlock()
	sm := m.maps[url]
	if sm == nil {
		sm = &syntheticMap{BundleURL: url}
		m.maps[url] = sm
	}
	fn(sm)
	return sm
}

func (m *sourcemapManager) set(url string, sm *syntheticMap) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maps[url] = sm
}

func (m *sourcemapManager) list() []*syntheticMap {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*syntheticMap
	for _, sm := range m.maps {
		result = append(result, sm)
	}
	return result
}

func (m *sourcemapManager) loadFromDisk(sourcesDir string) int {
	if m == nil || sourcesDir == "" {
		return 0
	}
	maps := sourcemap.LoadMapsFromDisk(sourcesDir)
	for _, sm := range maps {
		m.set(sm.BundleURL, &syntheticMap{
			BundleURL: sm.BundleURL,
			MapJSON:   sm.MapJSON,
			Sources:   sm.Structure,
			MapPath:   sm.MapPath,
		})
	}
	return len(maps)
}
