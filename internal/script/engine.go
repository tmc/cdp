package script

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pkg/errors"
)

// Engine executes CDP scripts
type Engine struct {
	script *Script
	opts   []chromedp.ExecAllocatorOption
	ctx    context.Context
	cancel context.CancelFunc

	verbose bool
	stdout  io.Writer
}

// Option defines a configuration option for the engine
type Option func(*Engine)

// WithVerbose enables verbose logging
func WithVerbose(verbose bool) Option {
	return func(e *Engine) {
		e.verbose = verbose
	}
}

// WithOutput sets the output writer
func WithOutput(w io.Writer) Option {
	return func(e *Engine) {
		e.stdout = w
	}
}

// WithAllocatorOptions adds options for the allocator
func WithAllocatorOptions(opts ...chromedp.ExecAllocatorOption) Option {
	return func(e *Engine) {
		e.opts = append(e.opts, opts...)
	}
}

// NewEngine creates a new script execution engine
func NewEngine(script *Script, opts ...Option) *Engine {
	e := &Engine{
		script: script,
		stdout: os.Stdout,
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// Run executes the script
func (e *Engine) Run(ctx context.Context) error {
	// Initialize context with script options
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("disable-gpu", true),
	)

	// Add engine-level options (e.g., from tests)
	if len(e.opts) > 0 {
		opts = append(opts, e.opts...)
	}

	if e.script.Metadata.Headless {
		opts = append(opts, chromedp.Headless)
	} else {
		opts = append(opts, chromedp.Flag("headless", false))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	var browserCtx context.Context
	var browserCancel context.CancelFunc

	if e.verbose {
		browserCtx, browserCancel = chromedp.NewContext(allocCtx, chromedp.WithLogf(func(format string, args ...interface{}) {
			fmt.Fprintf(e.stdout, format+"\n", args...)
		}))
	} else {
		browserCtx, browserCancel = chromedp.NewContext(allocCtx)
	}
	defer browserCancel()

	// Create context with timeout from metadata if set
	if e.script.Metadata.Timeout > 0 {
		browserCtx, browserCancel = context.WithTimeout(browserCtx, e.script.Metadata.Timeout)
		defer browserCancel()
	}

	e.ctx = browserCtx
	e.cancel = browserCancel // Not strictly necessary to store but good for symmetry

	// Initialize browser
	if err := chromedp.Run(browserCtx); err != nil {
		return errors.Wrap(err, "failed to initialize browser")
	}

	// Execute commands
	for i, cmd := range e.script.Commands {
		if e.verbose {
			fmt.Fprintf(e.stdout, "Executing command %d: %s %v\n", i+1, cmd.Name, cmd.Args)
		}

		if err := e.executeCommand(cmd); err != nil {
			return errors.Wrapf(err, "command %d (%s) failed at line %d", i+1, cmd.Name, cmd.Line)
		}
	}

	return nil
}

func (e *Engine) executeCommand(cmd Command) error {
	// Substitute variables
	args := make([]string, len(cmd.Args))
	for i, arg := range cmd.Args {
		args[i] = e.substituteVariables(arg)
	}

	switch cmd.Name {
	// Navigation
	case "goto":
		if len(args) < 1 {
			return errors.New("goto requires a URL")
		}
		return chromedp.Run(e.ctx, chromedp.Navigate(args[0]))

	case "back":
		return chromedp.Run(e.ctx, chromedp.NavigateBack())

	case "forward":
		return chromedp.Run(e.ctx, chromedp.NavigateForward())

	case "reload":
		return chromedp.Run(e.ctx, chromedp.Reload())

	// Interaction
	case "click":
		if len(args) < 1 {
			return errors.New("click requires a selector")
		}
		return chromedp.Run(e.ctx, chromedp.Click(args[0]))

	case "fill":
		if len(args) < 2 {
			return errors.New("fill requires selector and text")
		}
		return chromedp.Run(e.ctx, chromedp.SendKeys(args[0], args[1]))

	case "type":
		if len(args) < 1 {
			return errors.New("type requires text")
		}
		// Type into currently focused element
		return chromedp.Run(e.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			// This is simplified, real implementation would handle focused element better
			// For now, we need a focused element. If not, we might need 'focus' command first
			// But SendKeys without selector targets focused element in newer chromedp?
			// chromedp.SendKeys does require selector.
			// Let's implement type as "insert text"
			// Input.insertText is what we want
			// But chromedp doesn't expose it directly as an Action easily without a selector?
			// Actually kb.Type could work?
			// Let's use Runtime.evaluate to type into active element for now
			valBytes, _ := json.Marshal(args[0])
			js := fmt.Sprintf(`
				if (document.activeElement) {
					document.activeElement.value = (document.activeElement.value || '') + %s;
					document.activeElement.dispatchEvent(new Event('input', { bubbles: true }));
					document.activeElement.dispatchEvent(new Event('change', { bubbles: true }));
				}
			`, string(valBytes))
			_, exp, err := runtime.Evaluate(js).Do(ctx)
			if err != nil {
				return err
			}
			if exp != nil {
				return exp
			}
			return nil
		}))

	case "clear":
		if len(args) < 1 {
			return errors.New("clear requires a selector")
		}
		return chromedp.Run(e.ctx, chromedp.Clear(args[0]))

	case "focus":
		if len(args) < 1 {
			return errors.New("focus requires a selector")
		}
		return chromedp.Run(e.ctx, chromedp.Focus(args[0]))

	case "select":
		if len(args) < 2 {
			return errors.New("select requires selector and value")
		}
		// Simplified select implementation
		return chromedp.Run(e.ctx, chromedp.SetValue(args[0], args[1]))

	// Waiting
	case "wait":
		if len(args) < 1 {
			return errors.New("wait requires a selector")
		}
		return chromedp.Run(e.ctx, chromedp.WaitVisible(args[0]))

	case "wait_visible":
		if len(args) < 1 {
			return errors.New("wait_visible requires a selector")
		}
		return chromedp.Run(e.ctx, chromedp.WaitVisible(args[0]))

	case "wait_hidden":
		if len(args) < 1 {
			return errors.New("wait_hidden requires a selector")
		}
		return chromedp.Run(e.ctx, chromedp.WaitNotVisible(args[0]))

	case "sleep":
		if len(args) < 1 {
			return errors.New("sleep requires a duration")
		}
		d, err := time.ParseDuration(args[0])
		if err != nil {
			return errors.Wrap(err, "invalid duration")
		}
		return chromedp.Run(e.ctx, chromedp.Sleep(d))

	// Assertions
	case "assert":
		if len(args) < 2 {
			return errors.New("assert requires a type and argument")
		}
		return e.handleAssert(args)

	// Output
	case "screenshot":
		filename := "screenshot.png"
		if len(args) > 0 {
			filename = args[0]
		}
		var buf []byte
		if err := chromedp.Run(e.ctx, chromedp.CaptureScreenshot(&buf)); err != nil {
			return err
		}
		return os.WriteFile(filename, buf, 0644)

	// JavaScript
	case "js", "eval":
		if len(args) < 1 {
			return errors.New("js/eval requires code")
		}
		var res interface{}
		return chromedp.Run(e.ctx, chromedp.Evaluate(args[0], &res))

	case "extract":
		if len(args) < 1 {
			return errors.New("extract requires a selector")
		}
		selector := args[0]
		attr := ""
		if len(args) > 1 {
			attr = args[1]
		}

		var res string
		var err error
		if attr != "" {
			err = chromedp.Run(e.ctx, chromedp.AttributeValue(selector, attr, &res, nil))
		} else {
			err = chromedp.Run(e.ctx, chromedp.Text(selector, &res))
		}

		if err != nil {
			return err
		}
		fmt.Fprintln(e.stdout, res)
		return nil

	default:
		return errors.Errorf("unknown command: %s", cmd.Name)
	}
}

func (e *Engine) handleAssert(args []string) error {
	assertType := args[0]

	switch assertType {
	case "title":
		if len(args) < 2 {
			return errors.New("assert title requires expected title")
		}
		expected := args[1]
		var actual string
		if err := chromedp.Run(e.ctx, chromedp.Title(&actual)); err != nil {
			return err
		}
		if actual != expected {
			return errors.Errorf("assertion failed: expected title %q, got %q", expected, actual)
		}

	case "url":
		if len(args) < 2 {
			return errors.New("assert url requires expected url")
		}
		expected := args[1]
		var actual string
		if err := chromedp.Run(e.ctx, chromedp.Location(&actual)); err != nil {
			return err
		}
		if actual != expected {
			return errors.Errorf("assertion failed: expected url %q, got %q", expected, actual)
		}

	case "text":
		if len(args) < 3 {
			return errors.New("assert text requires selector and expected text")
		}
		selector := args[1]
		expected := args[2]
		var actual string
		if err := chromedp.Run(e.ctx, chromedp.Text(selector, &actual)); err != nil {
			return err
		}
		if strings.TrimSpace(actual) != expected {
			return errors.Errorf("assertion failed: expected text %q at %s, got %q", expected, selector, actual)
		}

	case "visible":
		if len(args) < 2 {
			return errors.New("assert visible requires selector")
		}
		selector := args[1]
		// WaitVisible fails if not visible, but we just want to check status immediately?
		// Usually assert visible implies waiting/verifying it is there.
		// Let's stick with check-like behavior.
		// chromedp doesn't have a simple "is visible" check without waiting or using JS.
		var visible bool
		js := fmt.Sprintf(`
			(function() {
				const el = document.querySelector("%s");
				if (!el) return false;
				const style = window.getComputedStyle(el);
				return style.display !== 'none' && style.visibility !== 'hidden' && style.opacity !== '0';
			})()
		`, strings.ReplaceAll(selector, `"`, `\"`))
		if err := chromedp.Run(e.ctx, chromedp.Evaluate(js, &visible)); err != nil {
			return err
		}
		if !visible {
			return errors.Errorf("assertion failed: element %s is not visible", selector)
		}

	case "status":
		if len(args) < 2 {
			return errors.New("assert status requires status code")
		}
		// This is tricky with chromedp as status code is event-based.
		// Skipping for now or implementing basic check if possible.
		return errors.New("assert status not implemented yet")

	default:
		return errors.Errorf("unknown assertion type: %s", assertType)
	}

	return nil
}

func (e *Engine) substituteVariables(s string) string {
	if e.script.Metadata.Variables == nil {
		return s
	}

	// Replace ${VAR} with value
	return os.Expand(s, func(key string) string {
		if val, ok := e.script.Metadata.Variables[key]; ok {
			return val
		}
		return os.Getenv(key)
	})
}
