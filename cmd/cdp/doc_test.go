package main

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/tmc/cdp/internal/docscan"
)

// TestDocCoversFlags checks that every flag cdp registers is described in
// doc.go. A flag that works but is not documented is invisible to users and to
// the agents that drive this command.
func TestDocCoversFlags(t *testing.T) {
	flags, err := docscan.Flags(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(flags) == 0 {
		t.Fatal("no flags found; the scanner is broken")
	}

	doc, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(doc)

	var missing []string
	for _, name := range flags {
		if !documents(text, name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Errorf("doc.go does not document %d of %d flags:\n\t-%s",
			len(missing), len(flags), strings.Join(missing, "\n\t-"))
	}
}

// TestDocCoversShellCommands checks that every command in the interactive
// shell registry is described in doc.go, along with its aliases.
func TestDocCoversShellCommands(t *testing.T) {
	doc, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(doc)

	r := NewCommandRegistry()
	var missing []string
	for _, cat := range r.ListCategories() {
		for _, cmd := range cat.Commands {
			for _, name := range append([]string{cmd.Name}, cmd.Aliases...) {
				if !strings.Contains(text, name) {
					missing = append(missing, cat.Name+"."+name)
				}
			}
		}
	}
	if len(missing) > 0 {
		t.Errorf("doc.go does not document %d shell commands/aliases:\n\t%s",
			len(missing), strings.Join(missing, "\n\t"))
	}
}

// TestDumpRegistry prints the shell command registry. It is a documentation aid,
// not an assertion: run it with -v when updating doc.go.
func TestDumpRegistry(t *testing.T) {
	if testing.Short() {
		t.Skip("documentation aid")
	}
	for _, cat := range NewCommandRegistry().ListCategories() {
		t.Logf("== %s", cat.Name)
		for _, cmd := range cat.Commands {
			t.Logf("   %-16s %-28s aliases=%v", cmd.Name, cmd.Usage, cmd.Aliases)
		}
	}
}

// documents reports whether text mentions the flag name as a whole word.
// A plain substring search is not enough: -har would be satisfied by a
// mention of -har-mode, hiding the undocumented flag.
func documents(text, name string) bool {
	re := regexp.MustCompile(regexp.QuoteMeta("-"+name) + `([^\w-]|$)`)
	return re.MatchString(text)
}
