package cdpscripttest_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestInteractionFixtureIndexReferencesExistingFiles(t *testing.T) {
	const index = "testdata/interaction/README.md"
	data, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}

	linkRE := regexp.MustCompile(`\[[^]]+\]\(([^)]+)\)`)
	for _, match := range linkRE.FindAllStringSubmatch(string(data), -1) {
		target := match[1]
		if strings.Contains(target, "://") || strings.HasPrefix(target, "#") {
			continue
		}
		target = strings.Split(target, "#")[0]
		path := filepath.Clean(filepath.Join(filepath.Dir(index), target))
		if _, err := os.Stat(path); err != nil {
			t.Errorf("link %q points to missing file %s: %v", match[1], path, err)
		}
	}
}

func TestInteractionFixtureIndexCoversCoreMechanics(t *testing.T) {
	data, err := os.ReadFile("testdata/interaction/README.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	for _, want := range []string{
		"dialogs",
		"downloads",
		"drag and drop",
		"dropdowns",
		"iframes",
		"network",
		"screenshots",
		"scrolling",
		"shadow DOM",
		"storage and cookies",
		"uploads",
		"viewport",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("fixture index missing %q", want)
		}
	}
}
