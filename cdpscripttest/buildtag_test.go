package cdpscripttest_test

import (
	"path/filepath"
	"testing"
)

func TestBrowserFixtureBuildTagGuidance(t *testing.T) {
	t.Log("browser-backed fixtures are behind the cdp build tag; run: go test -tags cdp -p 1 ./cdpscripttest")

	for _, pattern := range []string{
		"testdata/interaction/*.txtar",
		"testdata/cdpscript/*.txtar",
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) == 0 {
			t.Fatalf("no fixtures matched %s", pattern)
		}
	}
}
