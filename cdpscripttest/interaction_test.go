//go:build cdp

package cdpscripttest_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/tmc/cdp/cdpscript"
	"golang.org/x/tools/txtar"
	"gopkg.in/yaml.v3"
)

func TestInteractionFixtures(t *testing.T) {
	matches, err := filepath.Glob("testdata/interaction/*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("no interaction fixtures found")
	}
	sort.Strings(matches)

	baseURL := startTestServer(t)
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range matches {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			prepared := prepareInteractionFixture(t, path, map[string]string{
				"FIXTURE_BASE_URL": baseURL,
				"FIXTURE_ROOT":     root,
			})

			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			t.Cleanup(cancel)

			engine := cdpscript.New(
				cdpscript.WithVerbose(testing.Verbose()),
				cdpscript.WithOutputDir(t.TempDir()),
			)
			if err := engine.ExecuteTxtar(ctx, prepared, nil); err != nil {
				t.Fatalf("%s: %v", path, err)
			}
		})
	}
}

func prepareInteractionFixture(t *testing.T, path string, env map[string]string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	archive := txtar.Parse(data)
	fi := txtarFile(archive, "meta.yaml")
	if fi == nil {
		t.Fatalf("%s: missing meta.yaml", path)
	}

	var meta cdpscript.Metadata
	if err := yaml.Unmarshal(fi.Data, &meta); err != nil {
		t.Fatalf("%s: parse meta.yaml: %v", path, err)
	}
	if meta.Env == nil {
		meta.Env = make(map[string]string)
	}
	for key, value := range env {
		meta.Env[key] = value
	}

	fi.Data, err = yaml.Marshal(&meta)
	if err != nil {
		t.Fatalf("%s: marshal meta.yaml: %v", path, err)
	}

	out := filepath.Join(t.TempDir(), filepath.Base(path))
	if err := os.WriteFile(out, txtar.Format(archive), 0o644); err != nil {
		t.Fatal(err)
	}
	return out
}

func txtarFile(archive *txtar.Archive, name string) *txtar.File {
	for i := range archive.Files {
		if archive.Files[i].Name == name {
			return &archive.Files[i]
		}
	}
	return nil
}
