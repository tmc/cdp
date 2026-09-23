package report_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/tmc/cdp/cdpscripttest/report"
)

func Example() {
	dir, err := os.MkdirTemp("", "report-example-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	w, err := report.NewWriter(report.Options{Dir: dir, Combined: true}, []report.Script{
		{Name: "login", Source: []byte("# Log in.\nnavigate /login\n")},
		{Name: "search", Source: []byte("# Search.\nnavigate /search\n")},
	})
	if err != nil {
		log.Fatal(err)
	}
	err = w.Update(report.Script{
		Name:   "login",
		Source: []byte("# Log in.\nnavigate /login\n"),
		Log:    "# Log in. (0.100s)\n> navigate /login\n",
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := w.Close(); err != nil {
		log.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		log.Fatal(err)
	}
	// Print the summary; the per-script sections follow the "---" line.
	summary, _, _ := strings.Cut(string(index), "\n---")
	fmt.Println(summary)
	// Output:
	// # CDP Script Test Report
	//
	// 2 scripts: 1 passed, 0 failed, 1 pending
	//
	// ## Contents
	//
	// - PASS [login](#login) — Log in.
	// - PEND [search](#search) — Search.
}
