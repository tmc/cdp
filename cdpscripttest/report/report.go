// Package report renders cdpscripttest execution reports.
package report

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Script contains the input for one script report.
type Script struct {
	Name        string
	Source      []byte
	Log         string
	ArtifactDir string
	Failed      bool
}

// Options configures a Writer.
type Options struct {
	Dir      string
	HTML     bool
	Combined bool
}

// Writer writes detailed and combined reports.
type Writer struct {
	opts    Options
	scripts map[string]Script
	results map[string]Script
}

// NewWriter creates a report writer. scripts is the complete fixture manifest.
func NewWriter(opts Options, scripts []Script) (*Writer, error) {
	if opts.Dir == "" {
		return nil, fmt.Errorf("report directory is required")
	}
	if err := os.MkdirAll(opts.Dir, 0o777); err != nil {
		return nil, fmt.Errorf("create report directory: %w", err)
	}
	w := &Writer{opts: opts, scripts: make(map[string]Script, len(scripts)), results: make(map[string]Script, len(scripts))}
	for _, script := range scripts {
		w.scripts[script.Name] = script
	}
	if opts.Combined {
		if err := w.writeCombined(); err != nil {
			return nil, err
		}
	}
	return w, nil
}

// Update writes a completed script report and refreshes combined output.
func (w *Writer) Update(script Script) error {
	if script.Name == "" {
		return fmt.Errorf("script name is required")
	}
	if script.ArtifactDir == "" {
		script.ArtifactDir = filepath.Join(w.opts.Dir, script.Name)
	}
	if err := WriteMarkdown(filepath.Join(script.ArtifactDir, "report.md"), script); err != nil {
		return fmt.Errorf("write markdown report: %w", err)
	}
	if w.opts.HTML {
		if err := WriteHTML(filepath.Join(script.ArtifactDir, "report.html"), script); err != nil {
			return fmt.Errorf("write html report: %w", err)
		}
	}
	w.scripts[script.Name] = script
	w.results[script.Name] = script
	if w.opts.Combined {
		if err := w.writeCombined(); err != nil {
			return err
		}
	}
	return nil
}

// Close flushes remaining output.
func (w *Writer) Close() error {
	if w.opts.Combined {
		return w.writeCombined()
	}
	return nil
}

// WriteMarkdown writes a detailed Markdown report.
func WriteMarkdown(path string, script Script) error {
	var b bytes.Buffer
	renderMarkdown(&b, script, filepath.Dir(path))
	if err := writeFile(path, b.Bytes()); err != nil {
		return fmt.Errorf("write markdown: %w", err)
	}
	return nil
}

// WriteHTML writes a detailed HTML report.
func WriteHTML(path string, script Script) error {
	var b bytes.Buffer
	if err := renderHTML(&b, script, filepath.Dir(path)); err != nil {
		return fmt.Errorf("render html: %w", err)
	}
	if err := writeFile(path, b.Bytes()); err != nil {
		return fmt.Errorf("write html: %w", err)
	}
	return nil
}

func (w *Writer) writeCombined() error {
	names := make([]string, 0, len(w.scripts))
	for name := range w.scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	var markdown bytes.Buffer
	fmt.Fprintln(&markdown, "# CDP Script Test Report")
	fmt.Fprintln(&markdown)
	passed, failed := 0, 0
	for _, script := range w.results {
		if script.Failed {
			failed++
		} else {
			passed++
		}
	}
	pending := len(names) - len(w.results)
	if pending > 0 {
		fmt.Fprintf(&markdown, "%d scripts: %d passed, %d failed, %d pending\n\n", len(names), passed, failed, pending)
	} else {
		fmt.Fprintf(&markdown, "%d scripts: %d passed, %d failed\n\n", len(names), passed, failed)
	}
	fmt.Fprintln(&markdown, "## Contents")
	fmt.Fprintln(&markdown)
	for _, name := range names {
		status := "PEND"
		if script, ok := w.results[name]; ok {
			status = "PASS"
			if script.Failed {
				status = "FAIL"
			}
		}
		summary := firstLine(preamble(w.scripts[name].Source))
		if summary == "" {
			fmt.Fprintf(&markdown, "- %s [%s](#%s)\n", status, name, slug(name))
		} else {
			fmt.Fprintf(&markdown, "- %s [%s](#%s) — %s\n", status, name, slug(name), summary)
		}
	}
	fmt.Fprintln(&markdown, "\n---")
	for _, name := range names {
		script, ok := w.results[name]
		if !ok {
			continue
		}
		open := ""
		status := "PASS"
		if script.Failed {
			open = " open"
			status = "FAIL"
		}
		fmt.Fprintf(&markdown, "\n<details id=\"%s\"%s>\n<summary>%s <strong>%s</strong></summary>\n\n", slug(name), open, status, name)
		renderMarkdown(&markdown, script, w.opts.Dir)
		fmt.Fprintln(&markdown, "</details>")
	}
	if err := writeFile(filepath.Join(w.opts.Dir, "index.md"), markdown.Bytes()); err != nil {
		return fmt.Errorf("write combined markdown: %w", err)
	}
	if !w.opts.HTML {
		return nil
	}
	var html bytes.Buffer
	if err := renderCombinedHTML(&html, names, w.scripts, w.results, w.opts.Dir); err != nil {
		return fmt.Errorf("render combined html: %w", err)
	}
	if err := writeFile(filepath.Join(w.opts.Dir, "index.html"), html.Bytes()); err != nil {
		return fmt.Errorf("write combined html: %w", err)
	}
	return nil
}

type section struct {
	Comment, Timing string
	Commands        []command
}
type command struct {
	Line   string
	Stdout []string
	Images []string
	Result string
}

func parse(log string) []section {
	var sections []section
	current := -1
	stdout := false
	ensure := func() *section {
		if current < 0 {
			sections = append(sections, section{})
			current = len(sections) - 1
		}
		return &sections[current]
	}
	for _, line := range strings.Split(log, "\n") {
		trim := strings.TrimSpace(line)
		switch {
		case trim == "":
			stdout = false
		case strings.HasPrefix(trim, "#"):
			stdout = false
			text := strings.TrimSpace(strings.TrimPrefix(trim, "#"))
			timing := ""
			if i := strings.LastIndex(text, "("); i > 0 && strings.HasSuffix(text, "s)") {
				timing, text = text[i+1:len(text)-1], strings.TrimSpace(text[:i])
			}
			if current >= 0 && len(sections[current].Commands) == 0 {
				if sections[current].Comment != "" {
					sections[current].Comment += "\n"
				}
				sections[current].Comment += text
				if timing != "" {
					sections[current].Timing = timing
				}
			} else {
				sections = append(sections, section{Comment: text, Timing: timing})
				current = len(sections) - 1
			}
		case strings.HasPrefix(trim, ">"):
			stdout = false
			sec := ensure()
			sec.Commands = append(sec.Commands, command{Line: strings.TrimSpace(strings.TrimPrefix(trim, ">"))})
		case trim == "[stdout]":
			stdout = true
		case stdout:
			sec := ensure()
			if len(sec.Commands) == 0 {
				continue
			}
			cmd := &sec.Commands[len(sec.Commands)-1]
			cmd.Stdout = append(cmd.Stdout, trim)
			if artifactPath(trim) {
				cmd.Images = append(cmd.Images, trim)
			}
			if trim == "baseline created" || trim == "baseline updated" || strings.HasPrefix(trim, "diff:") {
				cmd.Result = trim
			}
		default:
			stdout = false
		}
	}
	return sections
}

func renderMarkdown(b *bytes.Buffer, script Script, reportDir string) {
	fmt.Fprintf(b, "# %s\n\n", script.Name)
	if p := preamble(script.Source); p != "" {
		for _, line := range strings.Split(p, "\n") {
			fmt.Fprintf(b, "> %s\n", line)
		}
		fmt.Fprintln(b)
	}
	for _, sec := range parse(script.Log) {
		if sec.Comment != "" {
			heading := sec.Comment
			if sec.Timing != "" {
				heading += " (" + sec.Timing + ")"
			}
			fmt.Fprintf(b, "## %s\n\n", heading)
		}
		for _, cmd := range sec.Commands {
			if assertion(cmd.Line) {
				continue
			}
			fmt.Fprintf(b, "```\n%s\n```\n\n", cmd.Line)
			if cmd.Result != "" {
				fmt.Fprintf(b, "**Result:** %s\n\n", cmd.Result)
			}
			for _, image := range cmd.Images {
				renderImageMarkdown(b, reportDir, image)
			}
		}
	}
}

func renderImageMarkdown(b *bytes.Buffer, reportDir, image string) {
	rel := relative(reportDir, image)
	if _, err := os.Stat(image); err != nil {
		fmt.Fprintf(b, "**Missing artifact:** `%s`\n\n", rel)
		return
	}
	if info, err := os.Stat(image); err == nil && info.IsDir() {
		fmt.Fprintf(b, "[frames manifest](%s)\n\n", relative(reportDir, filepath.Join(image, "manifest.json")))
		return
	}
	if strings.EqualFold(filepath.Ext(image), ".webm") {
		fmt.Fprintf(b, "[video: %s](%s)\n\n", filepath.Base(image), rel)
		return
	}
	fmt.Fprintf(b, "![%s](%s)\n\n", filepath.Base(image), rel)
	for _, companion := range imageCompanions(image) {
		if _, err := os.Stat(companion); err != nil {
			continue
		}
		fmt.Fprintf(b, "![%s](%s)\n\n", filepath.Base(companion), relative(reportDir, companion))
	}
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html><html><head><meta charset="utf-8"><title>{{.Name}}</title><style>body{font-family:system-ui,sans-serif;max-width:1000px;margin:auto;padding:2rem}pre{background:#f6f8fa;padding:1rem;overflow:auto}img{max-width:100%}.missing{color:#b42318}</style></head><body><h1>{{.Name}}</h1>{{range .Preamble}}<p>{{.}}</p>{{end}}{{range .Sections}}<section><h2>{{.Comment}}{{if .Timing}} ({{.Timing}}){{end}}</h2>{{range .Commands}}{{if .Show}}<pre>{{.Line}}</pre>{{if .Output}}<pre>{{range .Output}}{{.}}
{{end}}</pre>{{end}}{{if .Result}}<p><strong>Result:</strong> {{.Result}}</p>{{end}}{{range .Images}}{{if .Exists}}{{if .Directory}}<p><a href="{{.Path}}">frames manifest</a></p>{{else if .Video}}<video controls src="{{.Path}}"></video>{{else}}<figure><img src="{{.Path}}" alt="{{.Name}}"><figcaption>{{.Name}}</figcaption></figure>{{end}}{{else}}<p class="missing">Missing artifact: {{.Path}}</p>{{end}}{{end}}{{end}}{{end}}</section>{{end}}</body></html>`))

type htmlImage struct {
	Name, Path               string
	Exists, Directory, Video bool
}
type htmlCommand struct {
	Line, Result string
	Show         bool
	Output       []string
	Images       []htmlImage
}
type htmlSection struct {
	Comment, Timing string
	Commands        []htmlCommand
}
type htmlPage struct {
	Name     string
	Preamble []string
	Sections []htmlSection
}

func renderHTML(b *bytes.Buffer, script Script, reportDir string) error {
	p := htmlPage{Name: script.Name}
	if text := preamble(script.Source); text != "" {
		p.Preamble = strings.Split(text, "\n")
	}
	for _, sec := range parse(script.Log) {
		hs := htmlSection{Comment: sec.Comment, Timing: sec.Timing}
		for _, cmd := range sec.Commands {
			hc := htmlCommand{Line: cmd.Line, Result: cmd.Result, Show: !assertion(cmd.Line), Output: cmd.Stdout}
			for _, image := range cmd.Images {
				if info, err := os.Stat(image); err == nil && info.IsDir() {
					hc.Images = append(hc.Images, htmlImage{Name: filepath.Base(image), Path: relative(reportDir, filepath.Join(image, "manifest.json")), Exists: true, Directory: true})
					continue
				}
				paths := append([]string{image}, imageCompanions(image)...)
				for _, path := range paths {
					_, err := os.Stat(path)
					hc.Images = append(hc.Images, htmlImage{Name: filepath.Base(path), Path: relative(reportDir, path), Exists: err == nil, Video: strings.EqualFold(filepath.Ext(path), ".webm")})
				}
			}
			hs.Commands = append(hs.Commands, hc)
		}
		p.Sections = append(p.Sections, hs)
	}
	return pageTemplate.Execute(b, p)
}
func renderCombinedHTML(b *bytes.Buffer, names []string, scripts, results map[string]Script, dir string) error {
	fmt.Fprint(b, "<!doctype html><html><head><meta charset=\"utf-8\"><title>CDP Script Test Report</title></head><body><h1>CDP Script Test Report</h1><ul>")
	for _, name := range names {
		status := "PEND"
		if s, ok := results[name]; ok {
			status = "PASS"
			if s.Failed {
				status = "FAIL"
			}
		}
		fmt.Fprintf(b, "<li>%s %s</li>", template.HTMLEscapeString(status), template.HTMLEscapeString(name))
	}
	fmt.Fprint(b, "</ul>")
	for _, name := range names {
		s, ok := results[name]
		if !ok {
			continue
		}
		failed := ""
		if s.Failed {
			failed = " open"
		}
		path := relative(dir, filepath.Join(s.ArtifactDir, "report.html"))
		fmt.Fprintf(b, "<details%s><summary>%s</summary><p><a href=\"%s\">report</a></p></details>", failed, template.HTMLEscapeString(name), template.HTMLEscapeString(path))
	}
	fmt.Fprint(b, "</body></html>")
	return nil
}
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".report-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace report: %w", err)
	}
	return nil
}
func preamble(source []byte) string {
	var lines []string
	for _, line := range strings.Split(string(source), "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" {
			if len(lines) > 0 {
				break
			}
			continue
		}
		if !strings.HasPrefix(trim, "#") {
			break
		}
		lines = append(lines, strings.TrimSpace(strings.TrimPrefix(trim, "#")))
	}
	return strings.Join(lines, "\n")
}
func firstLine(s string) string { return strings.SplitN(s, "\n", 2)[0] }
func assertion(s string) bool {
	f := strings.Fields(s)
	if len(f) == 0 {
		return false
	}
	switch f[0] {
	case "stdout", "stderr", "stdin", "cmp", "cmpenv", "grep", "!":
		return true
	}
	return false
}
func artifactPath(s string) bool {
	ext := strings.ToLower(filepath.Ext(s))
	if ext == ".png" || ext == ".gif" || ext == ".webm" {
		return true
	}
	info, err := os.Stat(s)
	return err == nil && info.IsDir()
}

func imageCompanions(path string) []string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	return []string{
		base + "-unblurred" + ext,
		base + ".diff" + ext,
		base + ".current" + ext,
		path + ".fail.png",
	}
}
func relative(dir, path string) string {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
func slug(s string) string {
	s = strings.ToLower(s)
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		if r == ' ' || r == '_' {
			return '-'
		}
		return -1
	}, s)
}
