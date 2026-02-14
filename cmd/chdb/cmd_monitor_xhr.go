package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/chromedp/cdproto/debugger"
	"github.com/chromedp/cdproto/domdebugger"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/spf13/cobra"
)

var (
	monitorXHRUrlPattern string
	monitorXHRDumpDir    string
)

var monitorXHRCmd = &cobra.Command{
	Use:   "xhr",
	Short: "Monitor XHR requests and dump stack state",
	Long:  `Sets an XHR breakpoint and dumps the full stack state (variables, scopes) to JSON files whenever the breakpoint is hit.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := runMonitorXHR(ctx, tabID); err != nil {
			log.Fatalf("XHR Monitor failed: %v", err)
		}
	},
}

func init() {
	monitorCmd.AddCommand(monitorXHRCmd)
	monitorXHRCmd.Flags().StringVarP(&monitorXHRUrlPattern, "pattern", "p", "", "URL pattern to break on (contains)")
	monitorXHRCmd.Flags().StringVarP(&monitorXHRDumpDir, "out", "o", "xhr_dumps", "Output directory for dump files")
	monitorXHRCmd.Flags().String("tab", "", "Target tab ID")
}

func runMonitorXHR(ctx context.Context, tabID string) error {
	debuggerClient := NewChromeDebugger(port, verbose)
	defer debuggerClient.Close()

	if err := debuggerClient.Connect(ctx, tabID); err != nil {
		return err
	}

	// Prepare output directory
	if err := os.MkdirAll(monitorXHRDumpDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	// Enable domains
	if err := debuggerClient.EnableDomains(ctx, "Debugger", "Runtime", "DOMDebugger"); err != nil {
		return err
	}

	// Set XHR Breakpoint
	log.Printf("Setting XHR breakpoint for pattern: '%s'", monitorXHRUrlPattern)
	if err := chromedp.Run(debuggerClient.chromeCtx, domdebugger.SetXHRBreakpoint(monitorXHRUrlPattern)); err != nil {
		return fmt.Errorf("failed to set XHR breakpoint: %w", err)
	}

	// Setup event listener
	log.Println("Waiting for XHR requests... Press Ctrl+C to stop.")

	// Channel to signal resume
	resumeChan := make(chan bool)

	chromedp.ListenTarget(debuggerClient.chromeCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *debugger.EventPaused:
			log.Printf("Paused! Reason: %s", e.Reason)

			// Process the pause in a goroutine to not block the listener?
			// Actually listener is sync, so we can do work here.
			// But we need to use the context to make CDP calls.

			// Dump state
			if err := dumpStackState(debuggerClient.chromeCtx, e, monitorXHRDumpDir); err != nil {
				log.Printf("Error dumping state: %v", err)
			}

			// Resume
			go func() {
				// Resume asynchronously to allow this listener to return
				// (Though for Paused event, we usually want to handle it before resuming)
				// Small delay to ensure dump writes? No need.
				resumeChan <- true
			}()
		}
	})

	// Main loop to handle resume and interrupt
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-sigs:
			log.Println("Stopping monitor...")
			return nil
		case <-resumeChan:
			// Resume execution
			if err := chromedp.Run(debuggerClient.chromeCtx, debugger.Resume()); err != nil {
				log.Printf("Failed to resume: %v", err)
			} else {
				log.Println("Resumed execution.")
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

type StackDump struct {
	Timestamp time.Time    `json:"timestamp"`
	Reason    string       `json:"reason"`
	Frames    []FrameState `json:"frames"`
}

type FrameState struct {
	Index        int          `json:"index"`
	FunctionName string       `json:"functionName"`
	File         string       `json:"file"`
	Line         int64        `json:"line"`
	Scopes       []ScopeState `json:"scopes"`
}

type ScopeState struct {
	Type      string                 `json:"type"`
	Variables map[string]interface{} `json:"variables"`
}

func dumpStackState(ctx context.Context, ev *debugger.EventPaused, outDir string) error {
	dump := StackDump{
		Timestamp: time.Now(),
		Reason:    ev.Reason.String(),
		Frames:    []FrameState{},
	}

	log.Printf("Capturing stack state (%d frames)...", len(ev.CallFrames))

	for i, frame := range ev.CallFrames {
		fs := FrameState{
			Index:        i,
			FunctionName: frame.FunctionName,
			Line:         frame.Location.LineNumber,
			Scopes:       []ScopeState{},
		}
		// frame.URL is not available directly on CallFrame in the current cdproto version
		// We would need to look up the script by ID (frame.Location.ScriptID)
		fs.File = string(frame.Location.ScriptID)

		// Inspect scopes (limit to Local and Closure to avoid massive Global dumps)
		for _, scope := range frame.ScopeChain {
			// Extract variables
			vars, err := getScopeVariables(ctx, scope.Object.ObjectID)
			if err != nil {
				log.Printf("  Warning: failed to get scope vars: %v", err)
			}

			ss := ScopeState{
				Type:      scope.Type.String(),
				Variables: vars,
			}
			fs.Scopes = append(fs.Scopes, ss)
		}

		dump.Frames = append(dump.Frames, fs)
	}

	// Write to file
	filename := fmt.Sprintf("xhr_dump_%d.json", time.Now().UnixNano())
	path := filepath.Join(outDir, filename)

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(dump); err != nil {
		return err
	}

	log.Printf("Saved stack dump to %s", path)
	return nil
}

func getScopeVariables(ctx context.Context, objectID runtime.RemoteObjectID) (map[string]interface{}, error) {
	// Get properties of the scope object
	var props []*runtime.PropertyDescriptor
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Own properties only, generate preview for values
			// GetProperties returns: result, internalProperties, privateProperties, exceptionDetails, err
			p, _, _, _, err := runtime.GetProperties(objectID).
				WithOwnProperties(true).
				WithGeneratePreview(true).
				Do(ctx)
			props = p
			return err
		}),
	)
	if err != nil {
		return nil, err
	}

	variables := make(map[string]interface{})
	for _, prop := range props {
		if prop.Value != nil {
			// Simplify value representation
			val := prop.Value
			if val.Type == "object" && val.Preview != nil {
				// Use preview for objects
				variables[prop.Name] = fmt.Sprintf("[Object] %s", val.Description)
			} else if val.Value != nil {
				// Primitive value
				var v interface{}
				json.Unmarshal(val.Value, &v)
				variables[prop.Name] = v
			} else {
				variables[prop.Name] = fmt.Sprintf("[%s] %s", val.Type, val.Description)
			}
		}
	}

	return variables, nil
}
