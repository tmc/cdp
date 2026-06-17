package cdpscripttest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tmc/cdp/cdpscript"
	"rsc.io/script/scripttest"
)

func TestDefaultCmdsHyphenatedNamesAndAliases(t *testing.T) {
	cmds := DefaultCmds()
	for _, name := range []string{
		"wait-visible",
		"wait-not-visible",
		"send-keys",
		"set-base-url",
		"wait",
		"type",
		"fill",
		"waitVisible",
		"waitNotVisible",
		"sendKeys",
		"setBaseURL",
	} {
		if _, ok := cmds[name]; !ok {
			t.Errorf("DefaultCmds()[%q] missing", name)
		}
	}
}

func TestPackageDocMentionsDefaultCDPCommands(t *testing.T) {
	data, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	base := scripttest.DefaultCmds()

	for name := range DefaultCmds() {
		if _, ok := base[name]; ok {
			continue
		}
		if !docMentionsCommand(doc, name) {
			t.Errorf("doc.go does not mention command %q", name)
		}
	}
}

func TestPackageDocMentionsRunCDPScriptCommands(t *testing.T) {
	data, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)

	for _, name := range cdpscript.CommandNames() {
		if !docMentionsCommand(doc, name) {
			t.Errorf("doc.go does not mention RunCDPScript command %q", name)
		}
	}
}

func TestPackageDocReferencedFixtureDirsExist(t *testing.T) {
	data, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{
		"testdata/interaction",
		"testdata/cdpscript",
	} {
		if !strings.Contains(string(data), dir) {
			t.Fatalf("doc.go does not mention %s", dir)
		}
		if info, err := os.Stat(filepath.Clean(dir)); err != nil || !info.IsDir() {
			t.Fatalf("doc-referenced dir %s missing: %v", dir, err)
		}
	}
	if strings.Contains(string(data), "testdata/cdp/") {
		t.Fatal("doc.go still references removed testdata/cdp/ layout")
	}
}

func docMentionsCommand(doc, name string) bool {
	for _, line := range strings.Split(doc, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "//" {
			fields = fields[1:]
		}
		if len(fields) > 0 && strings.TrimSpace(fields[0]) == name {
			return true
		}
		if strings.Contains(line, name) {
			return true
		}
	}
	return false
}
