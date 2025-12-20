package main

import (
	"testing"
)

func TestParseInspectorArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		enabled  bool
		host     string
		port     string
	}{
		{
			name:    "no inspector flag",
			args:    []string{"server.js"},
			enabled: false,
			host:    "127.0.0.1",
			port:    "",
		},
		{
			name:    "default inspector",
			args:    []string{"--inspect", "app.js"},
			enabled: true,
			host:    "127.0.0.1",
			port:    defaultInspectorPort,
		},
		{
			name:    "explicit port with equals",
			args:    []string{"--inspect=9231", "api.js"},
			enabled: true,
			host:    "127.0.0.1",
			port:    "9231",
		},
		{
			name:    "host and port with equals",
			args:    []string{"--inspect=0.0.0.0:9232", "worker.js"},
			enabled: true,
			host:    "0.0.0.0",
			port:    "9232",
		},
		{
			name:    "inspect port separate argument",
			args:    []string{"--inspect", "9235", "main.js"},
			enabled: true,
			host:    "127.0.0.1",
			port:    "9235",
		},
		{
			name:    "inspect-brk with host",
			args:    []string{"--inspect-brk=::1:9240", "service.js"},
			enabled: true,
			host:    "::1",
			port:    "9240",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := parseInspectorArgs(tc.args)
			if cfg.Enabled != tc.enabled {
				t.Fatalf("expected enabled=%v got %v", tc.enabled, cfg.Enabled)
			}
			if cfg.Host != tc.host {
				t.Fatalf("expected host=%s got %s", tc.host, cfg.Host)
			}
			if cfg.Port != tc.port {
				t.Fatalf("expected port=%s got %s", tc.port, cfg.Port)
			}
		})
	}
}

func TestSplitPIDAndCommand(t *testing.T) {
	pid, cmd, ok := splitPIDAndCommand("12345 node server.js")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if pid != "12345" {
		t.Fatalf("expected pid 12345 got %s", pid)
	}
	if cmd != "node server.js" {
		t.Fatalf("expected command 'node server.js' got %s", cmd)
	}
}

func TestDetectNodeScript(t *testing.T) {
	tests := []struct {
		args    []string
		script  string
	}{
		{
			args:   []string{"server.js"},
			script: "server.js",
		},
		{
			args:   []string{"--inspect", "9229", "api.js", "--foo"},
			script: "api.js",
		},
		{
			args:   []string{"-r", "dotenv/config", "worker.mjs"},
			script: "worker.mjs",
		},
		{
			args:   []string{"--loader", "ts-node/esm", "--inspect=9230", "index.ts"},
			script: "index.ts",
		},
		{
			args:   []string{"-e", "console.log('hi')"},
			script: "",
		},
	}

	for _, tc := range tests {
		if script := detectNodeScript(tc.args); script != tc.script {
			t.Fatalf("expected %q, got %q for args %#v", tc.script, script, tc.args)
		}
	}
}
