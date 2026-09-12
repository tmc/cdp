package cdpscripttest

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/tmc/cdp/cdpscript"
)

// TestRunCDPScriptAppliesOptions checks that CDPScriptRunOptions.Options reach
// the engine, so the harness picks up cdpscript options it does not mirror as
// fields. The run itself fails on a missing archive, which is enough: options
// are applied when the engine is constructed, before execution.
func TestRunCDPScriptAppliesOptions(t *testing.T) {
	applied := 0
	opts := CDPScriptRunOptions{
		Options: []cdpscript.Option{
			func(*cdpscript.Engine) { applied++ },
			func(*cdpscript.Engine) { applied++ },
		},
	}

	missing := filepath.Join(t.TempDir(), "no-such-script.txtar")
	if err := RunCDPScript(context.Background(), missing, opts); err == nil {
		t.Fatal("RunCDPScript succeeded for a missing archive")
	}
	if applied != 2 {
		t.Fatalf("applied %d options, want 2", applied)
	}
}
