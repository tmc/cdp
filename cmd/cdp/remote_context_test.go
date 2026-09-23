package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewRemoteBrowserContextReportsFailedConnection(t *testing.T) {
	ctx, cancelCtx := context.WithTimeout(t.Context(), time.Second)
	defer cancelCtx()
	browserCtx, cancel, err := newRemoteBrowserContext(ctx, "ws://127.0.0.1:1")
	if err == nil {
		cancel()
		t.Fatal("newRemoteBrowserContext succeeded with an unreachable browser")
	}
	if browserCtx != nil || cancel != nil {
		t.Fatal("newRemoteBrowserContext returned a context after a failed connection")
	}
}

func TestRemoteTargetID(t *testing.T) {
	tests := []struct {
		name      string
		tabID     string
		remoteTab string
		tabs      []ChromeTab
		want      string
		wantErr   bool
	}{
		{name: "none"},
		{name: "remote tab ID", remoteTab: "remote", tabs: []ChromeTab{{ID: "remote"}}, want: "remote"},
		{name: "remote tab URL", remoteTab: "https://example.test", tabs: []ChromeTab{{ID: "remote", URL: "https://example.test"}}, want: "remote"},
		{name: "missing remote tab", remoteTab: "missing", wantErr: true},
		{name: "tab", tabID: "tab", want: "tab"},
		{name: "tab takes precedence", tabID: "tab", remoteTab: "remote", want: "tab"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := remoteTargetID(test.tabID, test.remoteTab, test.tabs)
			if (err != nil) != test.wantErr {
				t.Fatalf("remoteTargetID(%q, %q, %v) error = %v, want error %v", test.tabID, test.remoteTab, test.tabs, err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("remoteTargetID(%q, %q, %v) = %q, want %q", test.tabID, test.remoteTab, test.tabs, got, test.want)
			}
		})
	}
}

func TestMCPTargetID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/list" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode([]ChromeTab{{ID: "remote", URL: "https://example.test/", Type: "page"}})
	}))
	defer srv.Close()
	addr := srv.Listener.Addr().(*net.TCPAddr)

	tests := []struct {
		name      string
		tabID     string
		remoteTab string
		want      string
		wantErr   bool
	}{
		{name: "none"},
		{name: "tab", tabID: "tab", want: "tab"},
		{name: "tab takes precedence", tabID: "tab", remoteTab: "https://example.test/", want: "tab"},
		{name: "remote tab ID", remoteTab: "remote", want: "remote"},
		{name: "remote tab URL", remoteTab: "https://example.test/", want: "remote"},
		{name: "missing remote tab", remoteTab: "https://missing.test/", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := mcpConfig{RemoteHost: "127.0.0.1", RemotePort: addr.Port, TabID: test.tabID, RemoteTab: test.remoteTab}
			got, err := mcpTargetID(cfg)
			if (err != nil) != test.wantErr {
				t.Fatalf("mcpTargetID(%+v) error = %v, want error %v", cfg, err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("mcpTargetID(%+v) = %q, want %q", cfg, got, test.want)
			}
		})
	}
}
