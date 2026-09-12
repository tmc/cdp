package main

import (
	"context"
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
