package executor

import (
	"context"
	"testing"

	"github.com/tmc/misc/chrome-to-har/internal/cdpscript/types"
)

func TestExecutorCreation(t *testing.T) {
	ctx := context.Background()
	script := &types.Script{
		Metadata: types.Metadata{
			Name:     "Test Script",
			Browser:  "chrome",
			Headless: true,
			Env:      make(map[string]string),
		},
	}

	executor, err := NewExecutor(ctx, script, WithVerbose(false))
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	if executor == nil {
		t.Fatal("Executor is nil")
	}

	if executor.script != script {
		t.Error("Script not properly set")
	}

	if executor.variables == nil {
		t.Error("Variables map not initialized")
	}
}

func TestExecutorOptions(t *testing.T) {
	ctx := context.Background()
	script := &types.Script{
		Metadata: types.Metadata{
			Name:     "Test Script",
			Browser:  "chrome",
			Headless: true,
			Env:      map[string]string{"TEST_VAR": "test_value"},
		},
	}

	executor, err := NewExecutor(
		ctx,
		script,
		WithOutputDir("/tmp/test-output"),
		WithVerbose(true),
		WithVariables(map[string]string{"EXTRA_VAR": "extra_value"}),
	)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	if executor.outputDir != "/tmp/test-output" {
		t.Errorf("Output dir not set correctly: got %s", executor.outputDir)
	}

	if !executor.verbose {
		t.Error("Verbose not set")
	}

	// Check that script env vars were loaded
	if executor.variables["TEST_VAR"] != "test_value" {
		t.Error("Script environment variables not loaded")
	}

	// Check that extra vars were loaded
	if executor.variables["EXTRA_VAR"] != "extra_value" {
		t.Error("Extra variables not loaded")
	}
}

func TestSaveOutputFile(t *testing.T) {
	ctx := context.Background()
	script := &types.Script{
		Metadata: types.Metadata{
			Name:     "Test Script",
			Browser:  "chrome",
			Headless: true,
			Env:      make(map[string]string),
		},
	}

	executor, err := NewExecutor(ctx, script, WithOutputDir(t.TempDir()))
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	data := []byte("test data")
	err = executor.saveOutputFile("test.txt", data)
	if err != nil {
		t.Errorf("Failed to save output file: %v", err)
	}
}
