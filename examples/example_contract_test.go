package examples_test

import (
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tmc/cdp/cdpscript"
	"golang.org/x/tools/txtar"
)

func TestLiveDomainExamplesHaveRunnableContract(t *testing.T) {
	for _, path := range liveOnlyExamples(t) {
		path := path
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			ar := txtar.Parse(data)
			header := string(ar.Comment)
			for _, want := range []string{"Purpose:", "Usage:", "Inputs:", "Verification:"} {
				if !strings.Contains(header, want) {
					t.Fatalf("%s header missing %q:\n%s", path, want, header)
				}
			}

			files := map[string]bool{}
			for _, file := range ar.Files {
				files[file.Name] = true
			}
			if !files["main.cdp"] {
				t.Fatalf("%s missing main.cdp", path)
			}
			for _, name := range jsFiles(string(fileBody(t, ar, "main.cdp"))) {
				if !files[name] {
					t.Fatalf("%s references missing jsfile %s", path, name)
				}
			}
		})
	}
}

func TestTopLevelExamplesValidate(t *testing.T) {
	paths, err := filepath.Glob("*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := cdpscript.ValidateReader(path, f); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestTopLevelExamplesHaveHeaderContract(t *testing.T) {
	paths, err := filepath.Glob("*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		header := string(txtar.Parse(data).Comment)
		for _, want := range []string{"Purpose:", "Usage:", "Inputs:", "Verification:"} {
			if !strings.Contains(header, want) {
				t.Errorf("%s header missing %q", path, want)
			}
		}
	}
}

func TestAistudioFunctionCallingHasOneCanonicalExample(t *testing.T) {
	paths, err := filepath.Glob("aistudio-fc*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "aistudio-fc.txtar" {
		t.Fatalf("canonical AI Studio FC examples = %v, want [aistudio-fc.txtar]", paths)
	}
}

func TestExamplesIncludeParameterizedScript(t *testing.T) {
	paths, err := filepath.Glob("*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "${ARG1}") || strings.Contains(string(data), "$ARG1") {
			return
		}
	}
	t.Fatal("no top-level example uses ARG1")
}

func TestSensitiveExamplesDeclareLiveOnlyBoundary(t *testing.T) {
	paths, err := filepath.Glob("*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if !sensitiveExampleName(path) {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		header := string(txtar.Parse(data).Comment)
		if !strings.Contains(header, "Verification: live-only;") {
			t.Errorf("%s handles credentials or account state but lacks live-only verification boundary", path)
		}
	}
}

func TestExternalExamplesDeclareVerificationBoundary(t *testing.T) {
	paths, err := filepath.Glob("*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		ar := txtar.Parse(data)
		if !usesExternalBrowserTarget(ar) {
			continue
		}
		header := string(ar.Comment)
		if !strings.Contains(header, "Verification:") || !strings.Contains(strings.ToLower(header), "live") {
			t.Errorf("%s targets an external browser site but lacks a live verification boundary", path)
		}
	}
}

func TestExamplesReadmeDocumentsLiveOnlyPolicy(t *testing.T) {
	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	for _, want := range []string{
		"Live-only examples depend on a real site",
		"browser target or profile",
		"secret redaction needs",
		"repeat evidence",
		"Purpose:",
		"Usage:",
		"Inputs:",
		"Verification:",
		"cdp attach --port 9222",
		"cdpscript --tab <target-id> --port 9222 examples/gdoc-to-markdown.txtar",
		"live-workflow-intake.md",
		"live-workflow-template.md",
		"live-workflows.md",
		"example_contract_test.go",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("README missing %q", want)
		}
	}
}

func TestLiveWorkflowIntakeDocumentsUserBoundary(t *testing.T) {
	data, err := os.ReadFile("live-workflow-intake.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	for _, want := range []string{
		"Browser target or profile:",
		"Required login state:",
		"Starting URL or tab:",
		"Expected output artifact or stdout:",
		"Verification boundary:",
		"Secret handling and redaction:",
		"Repeat evidence:",
		"May the workflow change live data:",
		"Must the workflow be read-only:",
		"cdp attach --port 9222",
		"cdpscript --tab <target-id> --port 9222 examples/<workflow>.txtar",
		"Do not infer an authenticated account",
		"`Verification` says `live-only;`",
		"Secrets, cookies, bearer tokens, API keys",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("live workflow intake missing %q", want)
		}
	}
}

func TestLiveWorkflowTemplateDocumentsPromotionContract(t *testing.T) {
	data, err := os.ReadFile("live-workflow-template.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	for _, want := range []string{
		"Required login state:",
		"Browser target or profile:",
		"Verification boundary:",
		"Secret handling and redaction:",
		"Repeat evidence:",
		"Use the command printed by `cdp attach`; do not guess tab IDs.",
		"Capture checklist",
		"Redact:",
		"Generalize:",
		"Contract:",
		"cdp attach --port 9222",
		"cdpscript --tab <target-id> --port 9222 examples/<workflow>.txtar",
		"# Purpose:",
		"# Usage:",
		"# Inputs:",
		"# Verification: live-only;",
		"screenshot 01-start.png",
		"screenshot 02-result.png",
		"Do not promote a one-off exploratory script",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("live workflow template missing %q", want)
		}
	}
}

func TestProposedLiveWorkflowsDocumentFirstWorkflow(t *testing.T) {
	data, err := os.ReadFile("live-workflows.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	for _, want := range []string{
		"Google Docs to Markdown",
		"Workflow name: `google-docs-to-markdown`",
		"Status: curated live-only workflow.",
		"Existing script: `examples/gdoc-to-markdown.txtar`",
		"Required login state: signed in to a Google account",
		"Expected output artifact or stdout: Markdown printed to stdout.",
		"Verification boundary: live-only;",
		"Secret handling and redaction:",
		"Repeat evidence:",
		"Current evidence: ran twice on 2026-05-16",
		"It also ran once",
		"against a second disposable document",
		"May the workflow change live data: no.",
		"Must the workflow be read-only: yes.",
		"cdp attach --port 9222",
		"cdpscript --tab <target-id> --port 9222 examples/gdoc-to-markdown.txtar",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("live-workflows.md missing %q", want)
		}
	}
}

func TestGDocToMarkdownStdoutIsConvertedDocumentOnly(t *testing.T) {
	data, err := os.ReadFile("gdoc-to-markdown.txtar")
	if err != nil {
		t.Fatal(err)
	}
	ar := txtar.Parse(data)
	main := string(fileBody(t, ar, "main.cdp"))
	if strings.Contains(main, "\nlog ") || strings.HasPrefix(main, "log ") {
		t.Fatalf("gdoc-to-markdown should not write progress logs to stdout:\n%s", main)
	}
	if !strings.Contains(main, "jsfile extract-and-convert.js") {
		t.Fatalf("gdoc-to-markdown missing extraction jsfile command")
	}
	js := string(fileBody(t, ar, "extract-and-convert.js"))
	for _, want := range []string{
		"exportPlainText()",
		"/export?format=txt",
		"const markdown = exported && exported.trim() ? exported : extractGoogleDocContent();",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("extract-and-convert.js missing %q", want)
		}
	}
}

func fileBody(t *testing.T, ar *txtar.Archive, name string) []byte {
	t.Helper()

	for _, file := range ar.Files {
		if file.Name == name {
			return file.Data
		}
	}
	t.Fatalf("archive missing %s", name)
	return nil
}

func liveOnlyExamples(t *testing.T) []string {
	t.Helper()

	paths, err := filepath.Glob("*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(txtar.Parse(data).Comment), "Verification: live-only;") {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	if len(out) == 0 {
		t.Fatal("no live-only examples found")
	}
	return out
}

func sensitiveExampleName(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	for _, word := range []string{"apikey", "auth", "cookie", "oauth", "token"} {
		if strings.Contains(name, word) {
			return true
		}
	}
	return false
}

func usesExternalBrowserTarget(ar *txtar.Archive) bool {
	text := string(ar.Comment)
	for _, file := range ar.Files {
		text += "\n" + string(file.Data)
	}
	for _, marker := range []string{
		"goto http://",
		"goto https://",
		"OAUTH_URL",
		"GDOC_URL",
		"aistudio.google",
		"accounts.google",
		"app.slack",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return hasExternalHTTPURL(text)
}

func hasExternalHTTPURL(text string) bool {
	for _, scheme := range []string{"http://", "https://"} {
		for rest := text; ; {
			i := strings.Index(rest, scheme)
			if i < 0 {
				break
			}
			rest = rest[i:]
			end := strings.IndexAny(rest, " \t\r\n\"'<>)]}")
			raw := rest
			if end >= 0 {
				raw = rest[:end]
			}
			raw = strings.TrimRight(raw, ".,;:")
			u, err := url.Parse(raw)
			if err == nil && isExternalHost(u.Hostname()) {
				return true
			}
			rest = rest[len(scheme):]
		}
	}
	return false
}

func isExternalHost(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return true
	}
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified()
}

func jsFiles(script string) []string {
	var names []string
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "jsfile" {
			names = append(names, fields[1])
		}
	}
	return names
}
