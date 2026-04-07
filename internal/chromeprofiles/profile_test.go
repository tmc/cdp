package chromeprofiles

import (
	"strings"
	"testing"
)

func TestBuildCookieFilterQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		domains []string
		want    []string
		wantErr bool
	}{
		{
			name:    "single domain",
			domains: []string{"example.com"},
			want:    []string{"host_key LIKE '%example.com%'"},
		},
		{
			name:    "leading dot is normalized",
			domains: []string{".example.com"},
			want:    []string{"host_key LIKE '%example.com%'"},
		},
		{
			name:    "multiple domains",
			domains: []string{"example.com", "api.example.com"},
			want:    []string{"host_key LIKE '%example.com%'", "host_key LIKE '%api.example.com%'"},
		},
		{
			name:    "invalid domain",
			domains: []string{"exa mple.com"},
			wantErr: true,
		},
		{
			name:    "empty list",
			domains: nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := buildCookieFilterQuery(tt.domains)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("buildCookieFilterQuery(%v) succeeded, want error", tt.domains)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildCookieFilterQuery(%v): %v", tt.domains, err)
			}
			if !strings.HasPrefix(query, "PRAGMA busy_timeout=5000; DELETE FROM cookies WHERE NOT (") {
				t.Fatalf("query %q missing expected prefix", query)
			}
			for _, want := range tt.want {
				if !strings.Contains(query, want) {
					t.Fatalf("query %q missing clause %q", query, want)
				}
			}
		})
	}
}
