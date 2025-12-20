package script

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"

	"github.com/chromedp/chromedp"
)

func TestEngine(t *testing.T) {
	// Start a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`
				<html>
				<head><title>Test Page</title></head>
				<body>
					<div id="content">Hello, World!</div>
					<input id="input" type="text" />
					<button id="btn" onclick="document.getElementById('content').textContent = 'Clicked!'">Click Me</button>
				</body>
				</html>
			`))
		case "/next":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`
				<html>
				<head><title>Next Page</title></head>
				<body>
					<div id="next">Next Content</div>
				</body>
				</html>
			`))
		}
	}))
	defer ts.Close()

	// Create a script
	input := fmt.Sprintf(`
-- meta.yaml --
timeout: 30s
headless: true
variables:
  BASE_URL: "%s"

-- main.cdp --
goto ${BASE_URL}
assert title "Test Page"
assert text #content "Hello, World!"

fill #input "test input"
# Note: assert value check would be nice, but we don't have it yet
# We can use extract to check

click #btn
wait #content
# Wait for update - click is async
sleep 100ms
assert text #content "Clicked!"

# Test js
js {
	document.getElementById('content').textContent = "JS Updated";
}
assert text #content "JS Updated"
`, ts.URL)

	s, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	engine := NewEngine(s, WithVerbose(true))

	// Check if chrome is available, otherwise skip
	chromePath, err := exec.LookPath("google-chrome")
	if err != nil {
		t.Skip("google-chrome not found, skipping engine test")
	}

	// Re-create engine with specific chrome path
	engine = NewEngine(s,
		WithVerbose(true),
		WithAllocatorOptions(chromedp.ExecPath(chromePath)),
	)

	if err := engine.Run(context.Background()); err != nil {
		t.Errorf("Run() error = %v", err)
	}
}

func TestEngine_Variables(t *testing.T) {
	// Create a script
	input := `
-- meta.yaml --
timeout: 10s
headless: true
variables:
  TEST_VAR: "test value"

-- main.cdp --
# This is just parsing test for engine variable substitution
# We need to run it to test engine, but simple echo is enough
# Actually we don't have echo command, so let's just check substitution logic directly
# or use 'goto' with data url
`
	s, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	engine := NewEngine(s)

	got := engine.substituteVariables("Value is ${TEST_VAR}")
	want := "Value is test value"
	if got != want {
		t.Errorf("substituteVariables() = %q, want %q", got, want)
	}

	os.Setenv("ENV_VAR", "env value")
	got = engine.substituteVariables("Env is ${ENV_VAR}")
	want = "Env is env value"
	if got != want {
		t.Errorf("substituteVariables() = %q, want %q", got, want)
	}
}
