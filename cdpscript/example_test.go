package cdpscript_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tmc/cdp/cdpscript"
)

func Example() {
	var stdout bytes.Buffer
	engine := cdpscript.New(cdpscript.WithStdout(&stdout))
	script := `-- main.cdp --
log hello ${ARG1}
`
	_ = engine.ExecuteReader(context.Background(), "stdin", strings.NewReader(script), []string{"world"})
	fmt.Print(stdout.String())
	// Output:
	// hello world
}

// ExampleEngine_ExecuteScript runs a plain .cdp command body, without the txtar
// wrapper. This is the path MCP-defined tools use.
func ExampleEngine_ExecuteScript() {
	var stdout bytes.Buffer
	engine := cdpscript.New(cdpscript.WithStdout(&stdout))
	body := "log tool ${ARG1} ran\n"
	if err := engine.ExecuteScript(context.Background(), "tool", body, []string{"greet"}); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Print(stdout.String())
	// Output:
	// tool greet ran
}

// ExampleEngine_ExecuteTxtar runs an archive from disk. Arguments arrive as
// ARG1..ARGN, and ARGC holds the count.
func ExampleEngine_ExecuteTxtar() {
	dir, err := os.MkdirTemp("", "cdpscript")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "count.txtar")
	archive := "-- main.cdp --\nlog got ${ARGC} args: ${ARG1} ${ARG2}\n"
	if err := os.WriteFile(path, []byte(archive), 0o644); err != nil {
		fmt.Println("error:", err)
		return
	}

	var stdout bytes.Buffer
	engine := cdpscript.New(cdpscript.WithStdout(&stdout))
	if err := engine.ExecuteTxtar(context.Background(), path, []string{"a", "b"}); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Print(stdout.String())
	// Output:
	// got 2 args: a b
}
