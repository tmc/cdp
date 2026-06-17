package cdpscript_test

import (
	"bytes"
	"context"
	"fmt"
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
