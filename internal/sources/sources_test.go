package sources

import "testing"

func TestSplitURL(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantOrigin string
		wantRel    string
	}{
		{
			name:       "https with path",
			raw:        "https://example.com/static/app.js",
			wantOrigin: "example.com",
			wantRel:    "static/app.js",
		},
		{
			name:       "https root falls back to index",
			raw:        "https://example.com/",
			wantOrigin: "example.com",
			wantRel:    "index",
		},
		{
			name:       "file URL groups under file scheme (percent-decoded path)",
			raw:        "file:///Applications/Wispr%20Flow.app/Contents/Resources/app.asar/.webpack/renderer/status/index.js",
			wantOrigin: "file",
			wantRel:    "Applications/Wispr Flow.app/Contents/Resources/app.asar/.webpack/renderer/status/index.js",
		},
		{
			name:       "chrome-extension uses host",
			raw:        "chrome-extension://abcdefghijklmnop/background.js",
			wantOrigin: "abcdefghijklmnop",
			wantRel:    "background.js",
		},
		{
			name:       "unparseable returns empty",
			raw:        "://not a url",
			wantOrigin: "",
			wantRel:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOrigin, gotRel := splitURL(tt.raw)
			if gotOrigin != tt.wantOrigin || gotRel != tt.wantRel {
				t.Errorf("splitURL(%q) = (%q, %q), want (%q, %q)",
					tt.raw, gotOrigin, gotRel, tt.wantOrigin, tt.wantRel)
			}
		})
	}
}
