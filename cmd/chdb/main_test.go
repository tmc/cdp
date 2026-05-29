package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRootHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs([]string{"--help"})
	t.Cleanup(func() {
		rootCmd.SetOut(os.Stdout)
		rootCmd.SetErr(os.Stderr)
		rootCmd.SetArgs(nil)
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr:\n%s", stderr.String())
	}
	for _, want := range []string{
		"CHDB provides advanced debugging capabilities",
		"Available Commands",
		"attach",
		"screenshot",
		"network",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRootCommandRegistration(t *testing.T) {
	for _, name := range []string{
		"attach",
		"list",
		"exec",
		"navigate",
		"screenshot",
		"break",
		"debug",
		"monitor",
		"profile",
		"devtools",
		"dom",
		"css",
		"network",
		"storage",
		"sw",
		"device",
		"render",
		"animation",
		"inspect",
		"console",
		"sources",
		"overrides",
		"cookies",
		"emulate",
		"trace",
		"heap",
		"audit",
		"new",
		"bridge",
		"unminify",
	} {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := rootCmd.Find([]string{name})
			if err != nil {
				t.Fatal(err)
			}
			if cmd == nil || cmd.Name() != name {
				t.Fatalf("Find(%q) = %v, want command named %q", name, cmd, name)
			}
		})
	}
}
