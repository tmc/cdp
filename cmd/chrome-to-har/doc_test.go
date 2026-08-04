package main

import (
	"strings"
	"testing"

	"github.com/tmc/cdp/internal/docscan"
)

// TestDocCoversFlags checks that every flag chrome-to-har registers is
// described in doc.go. A flag that works but is not documented is invisible to
// users and to the agents that drive this command.
func TestDocCoversFlags(t *testing.T) {
	missing, err := docscan.Undocumented(".", "doc.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Errorf("doc.go does not document %d flags:\n\t-%s",
			len(missing), strings.Join(missing, "\n\t-"))
	}
}
