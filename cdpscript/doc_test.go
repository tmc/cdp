package cdpscript

import (
	"os"
	"sort"
	"strings"
	"testing"
)

// commandReference renders the godoc command reference from the engine's own
// command table. doc.go must contain the result verbatim.
func commandReference() string {
	cmds := New().commands()
	names := make([]string, 0, len(cmds))
	for name := range cmds {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		usage := cmds[name].Usage()
		sig := name
		if usage.Args != "" {
			sig += " " + usage.Args
		}
		pad := 46 - len(sig)
		if pad < 1 {
			pad = 1
		}
		b.WriteString("//\t" + sig + strings.Repeat(" ", pad) + usage.Summary + "\n")
	}
	return b.String()
}

func TestDocCommandReferenceMatchesEngine(t *testing.T) {
	doc, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	want := commandReference()
	if !strings.Contains(string(doc), want) {
		t.Errorf("doc.go command reference is out of date; replace it with:\n\n%s", want)
	}
}

func TestDocDocumentsConditions(t *testing.T) {
	doc, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	for name := range New().conditions() {
		if !strings.Contains(string(doc), name) {
			t.Errorf("doc.go does not document the %q condition", name)
		}
	}
}
