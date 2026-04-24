//go:build cdp

package cdpscripttest_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/tmc/cdp/cdpscripttest"
)

func TestInteractionFixtures(t *testing.T) {
	matches, err := filepath.Glob("testdata/interaction/*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("no interaction fixtures found")
	}

	baseURL := startTestServer(t)
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range matches {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			opts := cdpscripttest.CDPScriptRunOptions{
				Verbose:   testing.Verbose(),
				OutputDir: t.TempDir(),
				Env: []string{
					"FIXTURE_BASE_URL=" + baseURL,
					"FIXTURE_ROOT=" + root,
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			t.Cleanup(cancel)

			if err := cdpscripttest.RunCDPScript(ctx, path, opts); err != nil {
				t.Fatalf("%s: %v", path, err)
			}
		})
	}
}
