package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/tmc/cdp/internal/browser"
	"github.com/tmc/cdp/internal/discovery"
)

type attachCmd struct {
	fs *flag.FlagSet

	host   string
	port   int
	format string
	stdout io.Writer
	stderr io.Writer
}

type attachReport struct {
	Host         string         `json:"host"`
	Ports        []int          `json:"ports"`
	Targets      []attachTarget `json:"targets"`
	Errors       []string       `json:"errors,omitempty"`
	Instructions []string       `json:"instructions,omitempty"`
}

type attachTarget struct {
	Port    int    `json:"port"`
	ID      string `json:"id"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Command string `json:"command"`
}

func newAttachCmd() *attachCmd {
	c := &attachCmd{
		fs:     flag.NewFlagSet("attach", flag.ContinueOnError),
		host:   "localhost",
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	c.fs.SetOutput(c.stderr)
	c.fs.StringVar(&c.host, "host", "localhost", "CDP host to inspect")
	c.fs.IntVar(&c.port, "port", 0, "CDP port to inspect (0 scans common ports)")
	c.fs.StringVar(&c.format, "format", "text", "Output format: text or json")
	return c
}

func (c *attachCmd) run(args []string) error {
	if err := c.fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return fmt.Errorf("usage: %v", err)
	}
	report := discoverAttachReport(c.host, c.port)
	switch c.format {
	case "json":
		enc := json.NewEncoder(c.stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	case "text", "":
		printAttachReport(c.stdout, report)
		return nil
	default:
		return fmt.Errorf("usage: unknown format %q", c.format)
	}
}

func discoverAttachReport(host string, port int) attachReport {
	if host == "" {
		host = "localhost"
	}
	ports := attachPorts(port)
	report := attachReport{
		Host:  host,
		Ports: ports,
	}
	for _, p := range ports {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		tabs, err := listAttachTabs(ctx, host, p)
		cancel()
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("%s:%d: %v", host, p, err))
			continue
		}
		for _, tab := range tabs {
			if tab.Type != "" && tab.Type != "page" {
				continue
			}
			cmd := fmt.Sprintf("cdp --remote-host %s --remote-port %d --tab %s --shell", host, p, tab.ID)
			report.Targets = append(report.Targets, attachTarget{
				Port:    p,
				ID:      tab.ID,
				Type:    tab.Type,
				Title:   tab.Title,
				URL:     tab.URL,
				Command: cmd,
			})
		}
	}
	if len(report.Targets) == 0 {
		report.Instructions = launchInstructions(attachDefaultPort(port))
	}
	return report
}

func attachPorts(port int) []int {
	if port > 0 {
		return []int{port}
	}
	ports := []int{9222, 9223, 9224, 9225}
	for _, c := range discoveredRunningBrowsers() {
		if c.IsRunning && c.DebugPort > 0 {
			ports = append(ports, c.DebugPort)
		}
	}
	sort.Ints(ports)
	out := ports[:0]
	last := -1
	for _, p := range ports {
		if p != last {
			out = append(out, p)
			last = p
		}
	}
	return out
}

func discoveredRunningBrowsers() []BrowserCandidate {
	candidates, err := discoverBrowsers(false)
	if err != nil {
		return nil
	}
	return candidates
}

func listAttachTabs(ctx context.Context, host string, port int) ([]browser.ChromeTab, error) {
	if isLocalHost(host) {
		detector := browser.NewSessionDetector(false)
		ok, err := detector.VerifyDevToolsPort(ctx, port, 500*time.Millisecond)
		if err != nil {
			return nil, fmt.Errorf("verify devtools port: %w", err)
		}
		if !ok {
			return nil, fmt.Errorf("devtools endpoint not reachable")
		}
		raw, err := detector.EnumerateTabsWithRetry(ctx, port, 3)
		if err != nil {
			return nil, fmt.Errorf("enumerate tabs: %w", err)
		}
		var tabs []browser.ChromeTab
		if err := json.Unmarshal([]byte(raw), &tabs); err != nil {
			return nil, fmt.Errorf("parse tab list: %w", err)
		}
		return tabs, nil
	}
	return listTabsHTTP(ctx, host, port)
}

func listTabsHTTP(ctx context.Context, host string, port int) ([]browser.ChromeTab, error) {
	url := fmt.Sprintf("http://%s:%d/json", host, port)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create tab-list request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connect to devtools endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("devtools endpoint returned status %d", resp.StatusCode)
	}
	var tabs []browser.ChromeTab
	if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
		return nil, fmt.Errorf("parse tab list: %w", err)
	}
	return tabs, nil
}

func isLocalHost(host string) bool {
	switch strings.ToLower(host) {
	case "", "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	default:
		return false
	}
}

func attachDefaultPort(port int) int {
	if port > 0 {
		return port
	}
	return 9222
}

func launchInstructions(port int) []string {
	var out []string
	for _, candidate := range discovery.DiscoverBrowsers() {
		if !isChromiumBrowser(candidate.Name) {
			continue
		}
		out = append(out, fmt.Sprintf("%s --remote-debugging-port=%d --user-data-dir=\"$(mktemp -d)\"", shellQuote(candidate.Path), port))
		if len(out) == 3 {
			break
		}
	}
	if len(out) > 0 {
		out = append(out, fmt.Sprintf("cdp attach --port %d", port))
		return out
	}
	switch runtime.GOOS {
	case "darwin":
		return []string{
			fmt.Sprintf(`open -na "Brave Browser" --args --remote-debugging-port=%d --user-data-dir="$(mktemp -d)"`, port),
			fmt.Sprintf(`open -na "Google Chrome" --args --remote-debugging-port=%d --user-data-dir="$(mktemp -d)"`, port),
			fmt.Sprintf("cdp attach --port %d", port),
		}
	case "windows":
		return []string{
			fmt.Sprintf(`start chrome --remote-debugging-port=%d`, port),
			fmt.Sprintf("cdp attach --port %d", port),
		}
	default:
		return []string{
			fmt.Sprintf("brave-browser --remote-debugging-port=%d --user-data-dir=\"$(mktemp -d)\"", port),
			fmt.Sprintf("google-chrome --remote-debugging-port=%d --user-data-dir=\"$(mktemp -d)\"", port),
			fmt.Sprintf("cdp attach --port %d", port),
		}
	}
}

func isChromiumBrowser(name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, "brave") ||
		strings.Contains(name, "chrome") ||
		strings.Contains(name, "chromium") ||
		strings.Contains(name, "edge")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$`") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func printAttachReport(w io.Writer, report attachReport) {
	if len(report.Targets) == 0 {
		fmt.Fprintf(w, "No attachable CDP targets found on %s.\n", report.Host)
		if len(report.Errors) > 0 {
			fmt.Fprintln(w, "Probe diagnostics:")
			for _, err := range report.Errors {
				fmt.Fprintf(w, "  %s\n", err)
			}
		}
		fmt.Fprintln(w, "Start a browser with remote debugging enabled, then rerun attach:")
		for _, inst := range report.Instructions {
			fmt.Fprintf(w, "  %s\n", inst)
		}
		return
	}
	fmt.Fprintf(w, "Attachable CDP targets on %s:\n", report.Host)
	for i, target := range report.Targets {
		title := target.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Fprintf(w, "[%d] port=%d type=%s title=%s\n", i, target.Port, target.Type, title)
		if target.URL != "" {
			fmt.Fprintf(w, "    url: %s\n", target.URL)
		}
		fmt.Fprintf(w, "    id: %s\n", target.ID)
		fmt.Fprintf(w, "    command: %s\n", target.Command)
	}
}
