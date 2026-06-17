package cdpscript

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chromedp/chromedp"
	"rsc.io/script"
)

func (e *Engine) cmdSelect() script.Cmd {
	return simpleCmd("select dropdown option", "selector value|text", func(s *script.State, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("select requires selector and option")
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		selector := args[0]
		choice := strings.Join(args[1:], " ")
		if e.verbose {
			fmt.Fprintf(e.stderr, "[select] %s = %s\n", selector, choice)
		}

		var selected string
		err := chromedp.Run(e.browser.Context(), chromedp.Evaluate(selectOptionScript(selector, choice), &selected))
		if err != nil {
			return fmt.Errorf("select option: %w", err)
		}
		s.Setenv("SELECTED", selected)
		return nil
	})
}

func selectOptionScript(selector, choice string) string {
	return fmt.Sprintf(`(function() {
	const selector = %q;
	const choice = %q;
	const el = document.querySelector(selector);
	if (!el) throw new Error("no element matches " + selector);
	if (!(el instanceof HTMLSelectElement)) throw new Error("element is not a select");
	const option = Array.from(el.options).find(function(opt) {
		return opt.value === choice || opt.textContent.trim() === choice;
	});
	if (!option) throw new Error("no option matches " + choice);
	el.value = option.value;
	el.dispatchEvent(new Event("input", {bubbles: true}));
	el.dispatchEvent(new Event("change", {bubbles: true}));
	return el.value;
})()`, selector, choice)
}

func (e *Engine) cmdUpload() script.Cmd {
	return simpleCmd("upload files to a file input", "selector file...", func(s *script.State, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("upload requires selector and file path")
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		selector := args[0]
		files, err := resolveUploadFiles(s, args[1:])
		if err != nil {
			return err
		}
		if e.verbose {
			fmt.Fprintf(e.stderr, "[upload] %s <- %s\n", selector, strings.Join(files, ", "))
		}
		if err := chromedp.Run(e.browser.Context(), chromedp.SetUploadFiles(selector, files, chromedp.ByQuery)); err != nil {
			return fmt.Errorf("upload files: %w", err)
		}
		return nil
	})
}

func resolveUploadFiles(s *script.State, files []string) ([]string, error) {
	out := make([]string, 0, len(files))
	for _, file := range files {
		path, err := resolveUploadFile(s, file)
		if err != nil {
			return nil, err
		}
		out = append(out, path)
	}
	return out, nil
}

func resolveUploadFile(s *script.State, file string) (string, error) {
	if filepath.IsAbs(file) {
		return statUploadFile(file, file)
	}
	if path, err := statUploadFile(filepath.Join(s.Getwd(), file), file); err == nil {
		return path, nil
	}
	return statUploadFile(file, file)
}

func statUploadFile(path, original string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("upload file %q: %w", original, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("upload file %q is a directory", original)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("upload file %q: %w", original, err)
	}
	return abs, nil
}

func (e *Engine) cmdViewport() script.Cmd {
	return simpleCmd("set viewport size", "width height", func(s *script.State, args []string) error {
		width, height, err := parseViewportArgs(args)
		if err != nil {
			return err
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		if e.verbose {
			fmt.Fprintf(e.stderr, "[viewport] %dx%d\n", width, height)
		}
		if err := chromedp.Run(e.browser.Context(), chromedp.EmulateViewport(int64(width), int64(height))); err != nil {
			return fmt.Errorf("set viewport: %w", err)
		}
		return nil
	})
}

func parseViewportArgs(args []string) (int, int, error) {
	if len(args) != 2 {
		return 0, 0, fmt.Errorf("viewport requires width and height")
	}
	width, err := parseViewportSize(args[0])
	if err != nil {
		return 0, 0, fmt.Errorf("viewport width: %w", err)
	}
	height, err := parseViewportSize(args[1])
	if err != nil {
		return 0, 0, fmt.Errorf("viewport height: %w", err)
	}
	return width, height, nil
}

func parseViewportSize(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, fmt.Errorf("must be positive")
	}
	return n, nil
}
