package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/tmc/cdp/internal/docscan"
)

// TestDocCoversFlags checks that every flag cdpscript registers is described
// in doc.go. The flags are defined by the cdpscript package, which owns the
// command line, so that is where they are scanned from.
func TestDocCoversFlags(t *testing.T) {
	missing, err := docscan.Undocumented(filepath.Join("..", "..", "cdpscript"), "doc.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Errorf("doc.go does not document %d flags:\n\t-%s",
			len(missing), strings.Join(missing, "\n\t-"))
	}
}
