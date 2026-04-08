package cdpscripttest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"rsc.io/script"
	"rsc.io/script/scripttest"
)

// ErrSkip is returned by the skip command to signal that the current script
// should be skipped rather than failed. The CLI and test runner treat this
// as a skip, not a failure.
var ErrSkip = errors.New("skip")

// ErrStop is returned by the stop command to halt a script early without
// marking it as failed or skipped. Useful for conditional early exit:
//
//	eval '...'
//	[stdout false] stop
var ErrStop = errors.New("stop")

// DefaultCmds returns the full command set: scripttest's defaults plus all
// CDP-aware commands. Callers may add, replace, or remove entries.
//
// Aliases mirror the cmd/cdp interactive shell for familiarity:
//
//	goto        → navigate
//	wait        → waitVisible
//	js          → eval
//	jsfile      → evalfile
//	pause       → sleep
//	type, fill  → sendKeys
func DefaultCmds() map[string]script.Cmd {
	cmds := scripttest.DefaultCmds()

	nav := Navigate()
	cmds["navigate"] = nav
	cmds["goto"] = nav // alias: matches cmd/cdp shell

	wv := WaitVisible()
	cmds["waitVisible"] = wv
	cmds["wait"] = wv // alias: matches cmd/cdp shell

	cmds["waitNotVisible"] = WaitNotVisible()
	cmds["timeout"] = Timeout()
	cmds["click"] = Click()

	sk := SendKeys()
	cmds["sendKeys"] = sk
	cmds["type"] = sk // alias: matches cmd/cdp shell
	cmds["fill"] = sk // alias: matches cmd/cdp shell

	ev := Eval()
	cmds["eval"] = ev
	cmds["js"] = ev // alias: matches cmd/cdp shell

	ef := EvalFile()
	cmds["evalfile"] = ef
	cmds["jsfile"] = ef // alias: matches cmd/cdp shell

	cmds["text"] = Text()
	cmds["html"] = HTML()
	cmds["title"] = Title()
	cmds["url"] = URL()
	cmds["screenshot"] = Screenshot()
	cmds["screenshot-sel"] = ScreenshotSel()
	cmds["screenshot-compare"] = ScreenshotCompare()

	sl := Sleep()
	cmds["sleep"] = sl
	cmds["pause"] = sl // alias: matches cmd/cdp shell

	cmds["setBaseURL"] = SetBaseURL()

	cmds["inject"] = Inject()
	cmds["inject-clear"] = InjectClear()

	// Override scripttest's skip with our own that returns exported ErrSkip,
	// allowing callers to distinguish skip from failure.
	cmds["skip"] = Skip()
	cmds["stop"] = Stop()

	// Content extraction via external tools (gated by exec: conditions).
	cmds["distill"] = Distill()
	cmds["markdown"] = Markdown()

	// WebRTC monitoring and stats commands.
	for k, v := range WebRTCCmds() {
		cmds[k] = v
	}

	// Network emulation commands.
	for k, v := range NetworkCmds() {
		cmds[k] = v
	}

	return cmds
}

// Navigate returns a command that navigates to baseURL+path and waits for body.
//
// Usage: navigate <path>
func Navigate() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "navigate to baseURL+<path> and wait for <body>",
			Args:    "<path>",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 1 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			url := cs.baseURL + args[0]
			return func(s *script.State) (stdout, stderr string, err error) {
				err = chromedp.Run(cs.cdpCtx,
					chromedp.Navigate(url),
					chromedp.WaitReady("body", chromedp.ByQuery),
				)
				if err != nil {
					return "", "", err
				}
				return "", "", nil
			}, nil
		},
	)
}

// WaitVisible returns a command that waits for a CSS selector to be visible.
//
// Usage: waitVisible [--timeout <duration>] <selector>
func WaitVisible() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "wait for a CSS selector to become visible",
			Args:    "[--timeout <duration>] <selector>",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			timeout, args := parseWaitTimeout(args)
			if len(args) != 1 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			sel := args[0]
			return func(s *script.State) (stdout, stderr string, err error) {
				out, err := runWithWaitTimeout(cs, "waitVisible", sel, timeout,
					chromedp.WaitVisible(sel, chromedp.ByQuery),
				)
				return out, "", err
			}, nil
		},
	)
}

// WaitNotVisible returns a command that waits for a CSS selector to disappear.
//
// Usage: waitNotVisible [--timeout <duration>] <selector>
func WaitNotVisible() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "wait for a CSS selector to become not visible",
			Args:    "[--timeout <duration>] <selector>",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			timeout, args := parseWaitTimeout(args)
			if len(args) != 1 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			sel := args[0]
			return func(s *script.State) (stdout, stderr string, err error) {
				out, err := runWithWaitTimeout(cs, "waitNotVisible", sel, timeout,
					chromedp.WaitNotPresent(sel, chromedp.ByQuery),
				)
				return out, "", err
			}, nil
		},
	)
}

// Timeout returns a command that sets the default wait timeout for subsequent
// wait commands (waitVisible, waitNotVisible) in the current script.
//
// Usage: timeout <duration>
func Timeout() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "set the default timeout for wait commands",
			Args:    "<duration>",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 1 {
				return nil, script.ErrUsage
			}
			d, err := time.ParseDuration(args[0])
			if err != nil {
				return nil, fmt.Errorf("timeout: %w", err)
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			cs.SetWaitTimeout(d)
			return nil, nil
		},
	)
}

// RunWithWaitTimeout runs chromedp actions with the state's wait timeout.
// If the timeout expires, it returns a descriptive error instead of the
// raw "context deadline exceeded". On failure, it captures a full-page
// screenshot to the artifact directory so the report shows page state.
// The screenshot path is returned in stdout.
//
// Use this in custom commands to get consistent timeout and failure
// screenshot behavior. Pass 0 for timeout to use the state default.
func RunWithWaitTimeout(cs *State, cmdName, sel string, timeout time.Duration, actions ...chromedp.Action) (stdout string, err error) {
	return runWithWaitTimeout(cs, cmdName, sel, timeout, actions...)
}

func runWithWaitTimeout(cs *State, cmdName, sel string, timeout time.Duration, actions ...chromedp.Action) (stdout string, err error) {
	if timeout <= 0 {
		timeout = cs.WaitTimeout()
	}
	ctx, cancel := context.WithTimeout(cs.cdpCtx, timeout)
	defer cancel()
	err = chromedp.Run(ctx, actions...)
	if err == nil {
		return "", nil
	}
	if ctx.Err() != nil && cs.cdpCtx.Err() == nil {
		// Our sub-timeout expired, not the parent test context.
		err = fmt.Errorf("%s %q: timed out after %s", cmdName, sel, timeout)
	}
	// Capture failure screenshot using the parent context (still alive).
	stdout = captureFailureScreenshot(cs, cmdName, sel)
	return stdout, err
}

// captureFailureScreenshot takes a full-page screenshot on wait failure
// and saves it to the artifact directory. Returns the path for stdout
// (so the report can show it), or empty string on error.
func captureFailureScreenshot(cs *State, cmdName, sel string) string {
	dir := cs.artifactDirFn()
	if dir == "" {
		return ""
	}
	_ = os.MkdirAll(dir, 0o777)
	// Sanitize selector for filename.
	safe := strings.NewReplacer(
		"#", "", ".", "-", " ", "-", "'", "", "\"", "",
		"[", "", "]", "", "*", "", ",", "-",
	).Replace(sel)
	if len(safe) > 40 {
		safe = safe[:40]
	}
	name := fmt.Sprintf("%s-fail-%s.png", cmdName, safe)
	dest := filepath.Join(dir, name)

	var buf []byte
	if err := chromedp.Run(cs.cdpCtx, chromedp.FullScreenshot(&buf, 90)); err != nil {
		return ""
	}
	if err := os.WriteFile(dest, buf, 0o666); err != nil {
		return ""
	}
	return dest + "\n"
}

// parseWaitTimeout extracts an optional --timeout flag from args, returning
// the timeout duration (0 means use default) and remaining args.
func parseWaitTimeout(args []string) (time.Duration, []string) {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--timeout" {
			d, err := time.ParseDuration(args[i+1])
			if err == nil {
				return d, append(args[:i], args[i+2:]...)
			}
		}
	}
	return 0, args
}

// Click returns a command that clicks a CSS selector.
//
// Usage: click <selector>
func Click() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "click a CSS selector",
			Args:    "<selector>",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 1 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			sel := args[0]
			return func(s *script.State) (stdout, stderr string, err error) {
				stdout, err = runWithWaitTimeout(cs, "click", sel, 0,
					chromedp.Click(sel, chromedp.ByQuery),
				)
				return stdout, "", err
			}, nil
		},
	)
}

// SendKeys returns a command that sends keystrokes to a CSS selector.
//
// Usage: sendKeys <selector> <text>
func SendKeys() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "send keystrokes to a CSS selector",
			Args:    "<selector> <text>",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 2 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			sel, text := args[0], args[1]
			return func(s *script.State) (stdout, stderr string, err error) {
				stdout, err = runWithWaitTimeout(cs, "sendKeys", sel, 0,
					chromedp.SendKeys(sel, text, chromedp.ByQuery),
				)
				return stdout, "", err
			}, nil
		},
	)
}

// Eval returns a command that evaluates JavaScript and writes the result to stdout.
//
// Usage: eval <js>
func Eval() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "evaluate JavaScript; result written to stdout",
			Args:    "<js>",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) == 0 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			js := strings.Join(args, " ")
			return func(s *script.State) (stdout, stderr string, err error) {
				var result interface{}
				err = chromedp.Run(cs.cdpCtx,
					chromedp.Evaluate(js, &result),
				)
				if err != nil {
					return "", "", err
				}
				return fmt.Sprintf("%v\n", result), "", nil
			}, nil
		},
	)
}

// EvalFile reads a file from the working directory and evaluates its contents
// as JavaScript in the current page context. The file is typically embedded as
// a txtar section in the script archive.
//
// Usage: evalfile <filename>
func EvalFile() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "evaluate a JS file from the working directory in the page context",
			Args:    "<filename>",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 1 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			path := s.Path(args[0])
			return func(s *script.State) (stdout, stderr string, err error) {
				data, err := os.ReadFile(path)
				if err != nil {
					return "", "", err
				}
				var result interface{}
				if err := chromedp.Run(cs.cdpCtx,
					chromedp.Evaluate(string(data), &result),
				); err != nil {
					return "", "", err
				}
				return "", "", nil
			}, nil
		},
	)
}

// URL returns a command that reads the current page URL and writes it to stdout.
//
// Usage: url
func URL() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "get the current page URL; result written to stdout",
			Args:    "",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				var u string
				if err := chromedp.Run(cs.cdpCtx,
					chromedp.Evaluate(`window.location.href`, &u),
				); err != nil {
					return "", "", err
				}
				return u + "\n", "", nil
			}, nil
		},
	)
}

// Text returns a command that gets the trimmed inner text of a selector,
// writing it to stdout.
//
// Usage: text <selector>
func Text() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "get trimmed inner text of a CSS selector; result written to stdout",
			Args:    "<selector>",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 1 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			sel := args[0]
			return func(s *script.State) (stdout, stderr string, err error) {
				var text string
				err = chromedp.Run(cs.cdpCtx,
					chromedp.Text(sel, &text, chromedp.ByQuery),
				)
				if err != nil {
					return "", "", err
				}
				return strings.TrimSpace(text) + "\n", "", nil
			}, nil
		},
	)
}

// HTML returns a command that gets the inner HTML of a selector,
// writing it to stdout.
//
// Usage: html <selector>
func HTML() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "get inner HTML of a CSS selector; result written to stdout",
			Args:    "<selector>",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 1 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			sel := args[0]
			return func(s *script.State) (stdout, stderr string, err error) {
				var inner string
				err = chromedp.Run(cs.cdpCtx,
					chromedp.InnerHTML(sel, &inner, chromedp.ByQuery),
				)
				if err != nil {
					return "", "", err
				}
				return inner + "\n", "", nil
			}, nil
		},
	)
}

// Title returns a command that reads the page title and writes it to stdout.
//
// Usage: title
func Title() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "get the page title; result written to stdout",
			Args:    "",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				var title string
				err = chromedp.Run(cs.cdpCtx, chromedp.Title(&title))
				if err != nil {
					return "", "", err
				}
				return title + "\n", "", nil
			}, nil
		},
	)
}

// Screenshot returns a command that captures a full-page PNG screenshot.
//
// Without an argument, the file is saved as <seq>.png in the artifact or temp
// directory. With an argument, it is treated as a filename within that directory.
// The saved path is written to stdout.
//
// Optional --blur <selector> flags apply a CSS blur filter to matching
// elements before capture, useful for masking dynamic content like IDs
// or timestamps. Multiple --blur flags are allowed.
//
// Usage: screenshot [--blur <selector>]... [filename]
func Screenshot() script.Cmd {
	var seq atomic.Int64
	return script.Command(
		script.CmdUsage{
			Summary: "capture a full-page PNG screenshot; saved path written to stdout",
			Args:    "[--blur <selector>]... [filename]",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			blurSels, args := parseBlurFlags(args)
			if len(args) > 1 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			n := seq.Add(1)
			filename := fmt.Sprintf("%04d.png", n)
			if len(args) == 1 {
				filename = args[0]
			}
			dir := cs.artifactDirFn()
			dest := filepath.Join(dir, filename)
			captureFull := func() ([]byte, error) {
				var buf []byte
				err := chromedp.Run(cs.cdpCtx, chromedp.FullScreenshot(&buf, 100))
				return buf, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				if err := os.MkdirAll(filepath.Dir(dest), 0o777); err != nil {
					return "", "", fmt.Errorf("mkdir: %w", err)
				}
				if err := cs.applyBlurIfNeeded(blurSels, captureFull, dest); err != nil {
					return "", "", fmt.Errorf("blur: %w", err)
				}
				buf, err := captureFull()
				if err != nil {
					removeBlur(cs.cdpCtx)
					return "", "", err
				}
				removeBlur(cs.cdpCtx)
				if err := os.WriteFile(dest, buf, 0o666); err != nil {
					return "", "", fmt.Errorf("write %s: %w", dest, err)
				}
				return dest + "\n", "", nil
			}, nil
		},
	)
}

// ScreenshotSel returns a command that captures the bounding box of a CSS
// selector as a PNG.
//
// An optional --padding N flag expands the clip rect by N pixels on each
// side, useful for capturing box shadows or focus rings. Optional --blur
// flags apply a CSS blur filter to matching elements before capture.
//
// Usage: screenshot-sel [--padding N] [--blur <selector>]... <selector> [filename]
func ScreenshotSel() script.Cmd {
	var seq atomic.Int64
	return script.Command(
		script.CmdUsage{
			Summary: "capture a CSS-selector element as a PNG; saved path written to stdout",
			Args:    "[--padding N] [--blur <sel>]... <selector> [filename]",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			blurSels, args := parseBlurFlags(args)
			// Parse optional --padding flag.
			var padding float64
			for len(args) >= 2 && args[0] == "--padding" {
				_, err := fmt.Sscanf(args[1], "%f", &padding)
				if err != nil {
					return nil, fmt.Errorf("--padding: %w", err)
				}
				args = args[2:]
			}
			if len(args) < 1 || len(args) > 2 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			sel := args[0]
			n := seq.Add(1)
			filename := fmt.Sprintf("sel-%04d.png", n)
			if len(args) == 2 {
				filename = args[1]
			}
			dir := cs.artifactDirFn()
			dest := filepath.Join(dir, filename)
			captureSel := func() ([]byte, error) {
				var buf []byte
				if padding == 0 {
					err := chromedp.Run(cs.cdpCtx,
						chromedp.ScrollIntoView(sel, chromedp.ByQuery),
						chromedp.Screenshot(sel, &buf, chromedp.ByQuery),
					)
					return buf, err
				}
				err := chromedp.Run(cs.cdpCtx,
					chromedp.ScrollIntoView(sel, chromedp.ByQuery),
					screenshotWithPadding(sel, padding, &buf),
				)
				return buf, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				if err := os.MkdirAll(filepath.Dir(dest), 0o777); err != nil {
					return "", "", fmt.Errorf("mkdir: %w", err)
				}
				if err := cs.applyBlurIfNeeded(blurSels, captureSel, dest); err != nil {
					return "", "", fmt.Errorf("blur: %w", err)
				}
				buf, err := captureSel()
				if err != nil {
					removeBlur(cs.cdpCtx)
					return "", "", err
				}
				removeBlur(cs.cdpCtx)
				if err := os.WriteFile(dest, buf, 0o666); err != nil {
					return "", "", fmt.Errorf("write %s: %w", dest, err)
				}
				return dest + "\n", "", nil
			}, nil
		},
	)
}

// screenshotWithPadding captures a CSS selector's bounding box expanded by
// pad pixels on each side using page.CaptureScreenshot with an explicit clip.
func screenshotWithPadding(sel string, pad float64, buf *[]byte) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		// Get the element's bounding rect via JS.
		var rect struct {
			X, Y, Width, Height float64
		}
		js := fmt.Sprintf(`(function(){
			var r = document.querySelector(%q).getBoundingClientRect();
			return {X: r.left + window.scrollX, Y: r.top + window.scrollY,
			        Width: r.width, Height: r.height};
		})()`, sel)
		if err := chromedp.Evaluate(js, &rect).Do(ctx); err != nil {
			return err
		}

		x := math.Round(rect.X - pad)
		y := math.Round(rect.Y - pad)
		w := math.Round(rect.Width + 2*pad)
		h := math.Round(rect.Height + 2*pad)
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}

		clip := &page.Viewport{X: x, Y: y, Width: w, Height: h, Scale: 1}
		data, err := page.CaptureScreenshot().
			WithFormat(page.CaptureScreenshotFormatPng).
			WithCaptureBeyondViewport(true).
			WithFromSurface(true).
			WithClip(clip).
			Do(ctx)
		if err != nil {
			return err
		}
		*buf = data
		return nil
	})
}

// blurID is the element ID injected by applyBlur for easy removal.
const blurID = "__cdpst_blur_style"

// applyBlurIfNeeded applies blur unless skipBlur is set on State.
// If emitUnblurred is set, it captures an unblurred screenshot first and
// saves it with an "-unblurred" suffix.
func (cs *State) applyBlurIfNeeded(selectors []string, captureFn func() ([]byte, error), dest string) error {
	if cs.emitUnblurred && !cs.skipBlur && len(selectors) > 0 {
		buf, err := captureFn()
		if err == nil {
			ext := filepath.Ext(dest)
			base := strings.TrimSuffix(dest, ext)
			_ = os.WriteFile(base+"-unblurred"+ext, buf, 0o666)
		}
	}
	if cs.skipBlur {
		return nil
	}
	return applyBlur(cs.cdpCtx, selectors)
}

// applyBlur injects a <style> element that applies a CSS blur filter to
// matching elements. The blur makes text unreadable while preserving its
// visual presence — you can still see that content exists. Use a threshold
// on screenshot-compare to tolerate the small pixel differences that remain
// between different blurred strings.
// Call removeBlur after capturing to restore the page.
func applyBlur(ctx context.Context, selectors []string) error {
	if len(selectors) == 0 {
		return nil
	}
	css := strings.Join(selectors, ", ") +
		" { filter: blur(10px) !important; -webkit-filter: blur(10px) !important; }"
	js := fmt.Sprintf(`(function(){
		var s = document.createElement('style');
		s.id = %q;
		s.textContent = %q;
		document.head.appendChild(s);
	})()`, blurID, css)
	return chromedp.Run(ctx, chromedp.Evaluate(js, nil))
}

// removeBlur removes the style element injected by applyBlur.
func removeBlur(ctx context.Context) error {
	js := fmt.Sprintf(`(function(){
		var s = document.getElementById(%q);
		if (s) s.remove();
	})()`, blurID)
	return chromedp.Run(ctx, chromedp.Evaluate(js, nil))
}

// parseBlurFlags extracts --blur <selector> flags from args, returning the
// selectors and remaining args. Multiple flags are allowed.
func parseBlurFlags(args []string) (blurSelectors, remaining []string) {
	for i := 0; i < len(args); i++ {
		if args[i] == "--blur" && i+1 < len(args) {
			blurSelectors = append(blurSelectors, args[i+1])
			i++ // skip value
		} else {
			remaining = append(remaining, args[i])
		}
	}
	return blurSelectors, remaining
}

// htmlThroughTool gets the outer HTML of sel (default: "html") and pipes it
// through the named external binary, writing the result to stdout.
func htmlThroughTool(toolName, sel string, cs *State) (string, error) {
	if sel == "" {
		sel = "html"
	}
	var outerHTML string
	if err := chromedp.Run(cs.cdpCtx,
		chromedp.OuterHTML(sel, &outerHTML, chromedp.ByQuery),
	); err != nil {
		return "", fmt.Errorf("%s: get html: %w", toolName, err)
	}

	cmd := exec.CommandContext(cs.cdpCtx, toolName, "-") //nolint:gosec
	cmd.Stdin = strings.NewReader(outerHTML)
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w\n%s", toolName, err, errBuf.String())
	}
	return out.String(), nil
}

// Distill returns a command that extracts the main content from the current
// page (or a CSS selector) using the htmldistill tool, writing the result to
// stdout. Requires htmldistill in PATH; gate with [exec:htmldistill].
//
// Usage: distill [selector]
func Distill() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "extract main content via htmldistill; result written to stdout",
			Args:    "[selector]",
			Detail: []string{
				"Requires htmldistill in PATH. Gate with [exec:htmldistill].",
				"Selector defaults to the full page (<html>).",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) > 1 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			sel := ""
			if len(args) == 1 {
				sel = args[0]
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				out, err := htmlThroughTool("htmldistill", sel, cs)
				return out, "", err
			}, nil
		},
	)
}

// Markdown returns a command that converts the current page (or a CSS selector)
// to Markdown using the html2md tool, writing the result to stdout. Requires
// html2md in PATH; gate with [exec:html2md].
//
// Usage: markdown [selector]
func Markdown() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "convert page HTML to Markdown via html2md; result written to stdout",
			Args:    "[selector]",
			Detail: []string{
				"Requires html2md in PATH. Gate with [exec:html2md].",
				"Selector defaults to the full page (<html>).",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) > 1 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			sel := ""
			if len(args) == 1 {
				sel = args[0]
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				out, err := htmlThroughTool("html2md", sel, cs)
				return out, "", err
			}, nil
		},
	)
}

// Sleep returns a command that pauses for a given duration.
//
// Usage: sleep <duration>
func Sleep() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "pause for a duration (e.g. 500ms, 2s)",
			Args:    "<duration>",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 1 {
				return nil, script.ErrUsage
			}
			d, err := time.ParseDuration(args[0])
			if err != nil {
				return nil, err
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				select {
				case <-time.After(d):
				case <-cs.cdpCtx.Done():
					return "", "", cs.cdpCtx.Err()
				}
				return "", "", nil
			}, nil
		},
	)
}

// ScreenshotCompare returns a command that captures a screenshot and compares
// it against a baseline in the artifact directory. If the images differ by
// more than threshold percent of pixels (0–100), the command fails.
//
// Baselines live in the same artifact directory as regular screenshots.
// On first run (no baseline exists), the capture is saved as the baseline.
// On subsequent runs, the existing file is the baseline and the new capture
// is compared against it. Use -update-golden or --update to overwrite.
//
// Optional --blur flags apply a CSS blur filter to matching elements before
// capture, useful for masking dynamic content (IDs, timestamps).
//
// Usage: screenshot-compare [--threshold N] [--update] [--blur <sel>]... <selector> [filename]
func ScreenshotCompare() script.Cmd {
	var seq atomic.Int64
	return script.Command(
		script.CmdUsage{
			Summary: "capture screenshot and compare against baseline; reports diff percent",
			Args:    "[--threshold N] [--update] [--blur <sel>]... <selector> [filename]",
			Detail: []string{
				"Captures the bounding box of <selector> as a PNG.",
				"If a baseline exists in the artifact dir, computes a pixel diff.",
				"Fails if the diff exceeds --threshold (default: 5 percent).",
				"Use --update or -update-golden to overwrite the baseline.",
				"Use --blur <sel> to blur dynamic elements before capture.",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			blurSels, args := parseBlurFlags(args)
			var threshold float64 = 5
			update := false
			for len(args) > 0 {
				switch args[0] {
				case "--update", "--update-baseline":
					update = true
					args = args[1:]
					continue
				case "--threshold":
					if len(args) < 2 {
						return nil, fmt.Errorf("--threshold requires a value")
					}
					if _, err := fmt.Sscanf(args[1], "%f", &threshold); err != nil {
						return nil, fmt.Errorf("--threshold: %w", err)
					}
					args = args[2:]
					continue
				}
				break
			}
			if len(args) < 1 || len(args) > 2 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			sel := args[0]
			n := seq.Add(1)
			filename := fmt.Sprintf("cmp-%04d.png", n)
			if len(args) == 2 {
				filename = args[1]
			}
			dir := cs.artifactDirFn()
			dest := filepath.Join(dir, filename)
			captureSel := func() ([]byte, error) {
				var buf []byte
				err := chromedp.Run(cs.cdpCtx,
					chromedp.ScrollIntoView(sel, chromedp.ByQuery),
					chromedp.Screenshot(sel, &buf, chromedp.ByQuery),
				)
				return buf, err
			}

			return func(s *script.State) (stdout, stderr string, err error) {
				if err := os.MkdirAll(dir, 0o777); err != nil {
					return "", "", fmt.Errorf("mkdir: %w", err)
				}
				if err := cs.applyBlurIfNeeded(blurSels, captureSel, dest); err != nil {
					return "", "", fmt.Errorf("blur: %w", err)
				}
				buf, err := captureSel()
				if err != nil {
					removeBlur(cs.cdpCtx)
					return "", "", fmt.Errorf("capture %q: %w", sel, err)
				}
				removeBlur(cs.cdpCtx)

				// Update mode: overwrite the baseline unconditionally.
				if update || cs.updateGolden {
					if err := os.WriteFile(dest, buf, 0o666); err != nil {
						return "", "", fmt.Errorf("write: %w", err)
					}
					return fmt.Sprintf("%s\nbaseline updated\n", dest), "", nil
				}

				// Read existing baseline from the same artifact dir.
				baselineData, err := os.ReadFile(dest)
				if os.IsNotExist(err) {
					// No baseline yet: save current as the initial baseline.
					if err := os.WriteFile(dest, buf, 0o666); err != nil {
						return "", "", fmt.Errorf("write: %w", err)
					}
					return fmt.Sprintf("%s\nbaseline created\n", dest), "", nil
				}
				if err != nil {
					return "", "", fmt.Errorf("read baseline: %w", err)
				}

				// Compare images pixel-by-pixel.
				dr, err := imageDiff(baselineData, buf)
				if err != nil {
					return "", "", fmt.Errorf("diff: %w", err)
				}

				// Always save the current capture so the report can show it.
				ext := filepath.Ext(dest)
				base := strings.TrimSuffix(dest, ext)
				currentPath := base + ".current" + ext
				_ = os.WriteFile(currentPath, buf, 0o666)

				// Save diff visualization when pixels differ.
				if len(dr.DiffPNG) > 0 {
					diffPath := base + ".diff" + ext
					_ = os.WriteFile(diffPath, dr.DiffPNG, 0o666)
				}

				if dr.Pct > threshold {
					failPath := dest + ".fail.png"
					_ = os.WriteFile(failPath, buf, 0o666)
					// Return the baseline path in stdout so it appears in the
					// engine log for the report, even though the command fails.
					return fmt.Sprintf("%s\ndiff: %.2f%%\n", dest, dr.Pct),
						"", fmt.Errorf("diff %.2f%% exceeds threshold %.2f%%\nbaseline: %s\ncurrent:  %s",
							dr.Pct, threshold, dest, failPath)
				}
				return fmt.Sprintf("%s\ndiff: %.2f%%\n", dest, dr.Pct), "", nil
			}, nil
		},
	)
}

// imageDiffResult holds the result of comparing two images.
type imageDiffResult struct {
	Pct     float64 // percentage of differing pixels (0–100)
	DiffPNG []byte  // PNG-encoded diff visualization (nil if identical)
}

// imageDiff decodes two PNG images, computes the diff percentage, and generates
// a diff visualization PNG. Identical pixels are shown at 25% opacity;
// differing pixels are highlighted in red.
func imageDiff(a, b []byte) (imageDiffResult, error) {
	imgA, err := png.Decode(bytes.NewReader(a))
	if err != nil {
		return imageDiffResult{}, fmt.Errorf("decode baseline: %w", err)
	}
	imgB, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return imageDiffResult{}, fmt.Errorf("decode current: %w", err)
	}
	boundsA := imgA.Bounds()
	boundsB := imgB.Bounds()
	if boundsA != boundsB {
		return imageDiffResult{Pct: 100}, nil
	}
	total := boundsA.Dx() * boundsA.Dy()
	if total == 0 {
		return imageDiffResult{}, nil
	}

	diffImg := image.NewNRGBA(boundsA)
	var diffCount int
	for y := boundsA.Min.Y; y < boundsA.Max.Y; y++ {
		for x := boundsA.Min.X; x < boundsA.Max.X; x++ {
			rA, gA, bA, aA := imgA.At(x, y).RGBA()
			rB, gB, bB, aB := imgB.At(x, y).RGBA()
			if rA != rB || gA != gB || bA != bB || aA != aB {
				diffCount++
				// Highlight diff pixels in red.
				diffImg.SetNRGBA(x, y, color.NRGBA{R: 255, G: 0, B: 40, A: 220})
			} else {
				// Dim identical pixels.
				r, g, b, _ := imgA.At(x, y).RGBA()
				diffImg.SetNRGBA(x, y, color.NRGBA{
					R: uint8(r >> 8),
					G: uint8(g >> 8),
					B: uint8(b >> 8),
					A: 64,
				})
			}
		}
	}

	pct := float64(diffCount) / float64(total) * 100

	if diffCount == 0 {
		return imageDiffResult{Pct: 0}, nil
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, diffImg); err != nil {
		return imageDiffResult{Pct: pct}, nil // non-fatal
	}
	return imageDiffResult{Pct: pct, DiffPNG: buf.Bytes()}, nil
}

// Inject returns a command that installs a JavaScript snippet to run at the
// start of every new document in the browser context, persisting across
// navigations. It uses Page.addScriptToEvaluateOnNewDocument internally.
//
// The identifier is saved so inject-clear can remove it.
//
// Usage: inject <js>
func Inject() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "install JS to run on every new document (persists across navigations)",
			Args:    "<js>",
			Detail: []string{
				"Uses Page.addScriptToEvaluateOnNewDocument.",
				"Useful for installing error handlers or globals before navigation.",
				"Use inject-clear to remove all injected scripts.",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 1 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			src := args[0]
			return func(s *script.State) (stdout, stderr string, err error) {
				var id page.ScriptIdentifier
				if err := chromedp.Run(cs.cdpCtx, chromedp.ActionFunc(func(ctx context.Context) error {
					var err error
					id, err = page.AddScriptToEvaluateOnNewDocument(src).Do(ctx)
					return err
				})); err != nil {
					return "", "", err
				}
				cs.injectedScripts = append(cs.injectedScripts, id)
				return "", "", nil
			}, nil
		},
	)
}

// InjectClear returns a command that removes all scripts previously installed
// with inject for this browser context.
//
// Usage: inject-clear
func InjectClear() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "remove all scripts installed with inject",
			Args:    "",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				ids := cs.injectedScripts
				cs.injectedScripts = nil
				for _, id := range ids {
					id := id
					if err := chromedp.Run(cs.cdpCtx, chromedp.ActionFunc(func(ctx context.Context) error {
						return page.RemoveScriptToEvaluateOnNewDocument(id).Do(ctx)
					})); err != nil {
						return "", "", err
					}
				}
				return "", "", nil
			}, nil
		},
	)
}

// Skip returns a command that marks the current script as skipped.
// It returns ErrSkip, which callers can distinguish from a test failure.
// This overrides scripttest's built-in skip to use the exported ErrSkip sentinel.
//
// Usage: skip [msg]
func Skip() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "skip the current script (not a failure)",
			Args:    "[msg]",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) > 1 {
				return nil, script.ErrUsage
			}
			msg := "skip"
			if len(args) == 1 {
				msg = args[0]
			}
			return nil, fmt.Errorf("%w: %s", ErrSkip, msg)
		},
	)
}

// Stop returns a command that halts the current script immediately without
// marking it as failed or skipped. It returns ErrStop.
//
// Typical use with a [stdout:pattern] condition for conditional early exit:
//
//	eval '(function(){ return someCheck() ? "false" : "ok"; })()'
//	[stdout false] stop
//	# ... rest of script only runs when check passed
//
// Usage: stop [msg]
func Stop() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "halt the script early without failure or skip",
			Args:    "[msg]",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) > 1 {
				return nil, script.ErrUsage
			}
			msg := "stop"
			if len(args) == 1 {
				msg = args[0]
			}
			return nil, fmt.Errorf("%w: %s", ErrStop, msg)
		},
	)
}

// SetBaseURL returns a command that overrides the base URL for subsequent commands.
//
// Usage: setBaseURL <url>
func SetBaseURL() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "override $BASE_URL for subsequent navigate commands",
			Args:    "<url>",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 1 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			u := args[0]
			return func(s *script.State) (stdout, stderr string, err error) {
				return "", "", cs.SetBaseURL(u)
			}, nil
		},
	)
}
