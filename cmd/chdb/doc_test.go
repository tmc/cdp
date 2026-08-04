package main

import (
	"os"
	"strings"
	"testing"
)

// TestDocCoversCommands checks that every top-level chdb command is described
// in doc.go. A command that exists but is not documented is invisible to users
// and to the agents that drive this command.
func TestDocCoversCommands(t *testing.T) {
	doc, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(doc)

	var missing []string
	for _, cmd := range rootCmd.Commands() {
		if cmd.Hidden || cmd.Name() == "help" || cmd.Name() == "completion" {
			continue
		}
		if !strings.Contains(text, cmd.Name()) {
			missing = append(missing, cmd.Name())
		}
	}
	if len(missing) > 0 {
		t.Errorf("doc.go does not document %d commands:\n\t%s",
			len(missing), strings.Join(missing, "\n\t"))
	}
}
