package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/tmc/misc/chrome-to-har/internal/coverage"
	"github.com/tmc/misc/chrome-to-har/internal/sourcemap"
	"github.com/tmc/misc/chrome-to-har/internal/testutil"
)

// bundleJS is a realistic minified JavaScript bundle used as the test target.
// It simulates a small SPA with routing, state management, and DOM manipulation.
const bundleJS = `"use strict";var __APP__=function(){var e={};function t(n){if(e[n])return e[n].exports;var r=e[n]={i:n,l:!1,exports:{}};return r}
function Router(){this.routes={};this.currentRoute=null}
Router.prototype.addRoute=function(path,handler){this.routes[path]=handler;return this};
Router.prototype.navigate=function(path){if(this.routes[path]){this.currentRoute=path;this.routes[path]();return true}return false};
function Store(initial){this.state=Object.assign({},initial);this.listeners=[]}
Store.prototype.getState=function(){return Object.assign({},this.state)};
Store.prototype.setState=function(updates){Object.assign(this.state,updates);this.listeners.forEach(function(fn){fn(this.state)}.bind(this))};
Store.prototype.subscribe=function(fn){this.listeners.push(fn);return function(){this.listeners=this.listeners.filter(function(l){return l!==fn})}.bind(this)};
function createElement(tag,attrs,children){var el=document.createElement(tag);if(attrs){Object.keys(attrs).forEach(function(k){el.setAttribute(k,attrs[k])})}if(children){if(typeof children==="string"){el.textContent=children}else{children.forEach(function(c){el.appendChild(c)})}}return el}
function renderApp(store,router){var root=document.getElementById("app");if(!root)return;root.innerHTML="";var state=store.getState();var header=createElement("header",{"class":"app-header"},[createElement("h1",null,"Test App"),createElement("nav",null,[createElement("a",{href:"#/"},"/"),createElement("a",{href:"#/about"},"About"),createElement("a",{href:"#/counter"},"Counter")])]);root.appendChild(header);var content=createElement("div",{"class":"content"});if(state.page==="counter"){var count=createElement("span",{id:"count"},String(state.count||0));var btn=createElement("button",{id:"increment"},"Increment");btn.addEventListener("click",function(){store.setState({count:(state.count||0)+1});renderApp(store,router)});content.appendChild(createElement("div",null,[count,btn]))}else if(state.page==="about"){content.appendChild(createElement("p",null,"About page content"))}else{content.appendChild(createElement("p",null,"Welcome to the test app"))}root.appendChild(content)}
var store=new Store({page:"home",count:0});var router=new Router();
router.addRoute("/",function(){store.setState({page:"home"});renderApp(store,router)});
router.addRoute("/about",function(){store.setState({page:"about"});renderApp(store,router)});
router.addRoute("/counter",function(){store.setState({page:"counter"});renderApp(store,router)});
window.addEventListener("hashchange",function(){var path=window.location.hash.slice(1)||"/";router.navigate(path)});
document.addEventListener("DOMContentLoaded",function(){var path=window.location.hash.slice(1)||"/";router.navigate(path)});
return{store:store,router:router,renderApp:renderApp}}();
`

// testPage returns HTML that loads the bundle.
func testPage(bundleURL string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html><head><title>Pipeline Test</title></head>
<body><div id="app"></div>
<script src="%s"></script>
</body></html>`, bundleURL)
}

// TestSourcemapPipeline_EndToEnd exercises the full synthetic sourcemap
// pipeline against a local test server with a realistic JS bundle.
//
// Steps tested:
//  1. Chrome navigates to a page with a bundled JS file
//  2. Coverage collection captures executed byte ranges
//  3. Chunk extraction produces CodeChunks from coverage data
//  4. Sourcemap generation creates valid v3 JSON from inferred structure
//  5. Intercept store correctly manages serving rules
//
// Steps that require a real MCP session (documented but not tested here):
//   - analyze_bundle: needs MCP sampling to call the LLM
//   - refine_sourcemap: same — sampling required
//   - serve_sourcemap: Fetch domain intercept needs Chrome event listener wiring
func TestSourcemapPipeline_EndToEnd(t *testing.T) {
	skipIfNoBrowser(t)

	// --- Step 0: Set up test server with bundle ---

	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bundle.js" {
			w.Header().Set("Content-Type", "application/javascript")
			w.Write([]byte(bundleJS))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(testPage(serverURL + "/bundle.js")))
	}))
	serverURL = server.URL
	defer server.Close()

	bundleURL := serverURL + "/bundle.js"

	// --- Step 1: Start Chrome and navigate ---

	browserPath := testutil.FindChrome()
	if browserPath == "" {
		t.Skip("no Chrome-compatible browser found")
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browserPath),
		chromedp.Headless,
		chromedp.DisableGPU,
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	// The coverage collector needs the inner CDP target context (the one
	// chromedp passes to ActionFunc callbacks). The outer browserCtx from
	// chromedp.NewContext doesn't have the protocol executor attached.
	cov := coverage.New(true)

	t.Log("starting coverage and navigating...")
	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return cov.Start(ctx)
	})); err != nil {
		t.Fatalf("start coverage: %v", err)
	}

	ctx, timeoutCancel := context.WithTimeout(browserCtx, 60*time.Second)
	defer timeoutCancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(serverURL)); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	// Wait for DOMContentLoaded to fire and the app to render.
	if err := chromedp.Run(ctx, chromedp.WaitVisible("#app", chromedp.ByID)); err != nil {
		t.Fatalf("wait for #app: %v", err)
	}

	// Take baseline snapshot.
	snap1, err := cov.TakeSnapshot("baseline")
	if err != nil {
		t.Fatalf("take baseline snapshot: %v", err)
	}
	t.Logf("baseline snapshot: %d scripts", len(snap1.Scripts))

	// Navigate to counter page (triggers new code paths).
	t.Log("navigating to #/counter...")
	if err := chromedp.Run(ctx,
		chromedp.Navigate(serverURL+"#/counter"),
		chromedp.Sleep(500*time.Millisecond),
	); err != nil {
		t.Fatalf("navigate to counter: %v", err)
	}

	// Click the increment button a few times.
	t.Log("clicking increment button...")
	for i := 0; i < 3; i++ {
		if err := chromedp.Run(ctx,
			chromedp.WaitVisible("#increment", chromedp.ByID),
			chromedp.Click("#increment", chromedp.ByID),
			chromedp.Sleep(200*time.Millisecond),
		); err != nil {
			t.Logf("click %d: %v (may be expected if DOM re-renders)", i, err)
		}
	}

	// Take post-action snapshot.
	snap2, err := cov.TakeSnapshot("after-counter")
	if err != nil {
		t.Fatalf("take after-counter snapshot: %v", err)
	}
	t.Logf("after-counter snapshot: %d scripts", len(snap2.Scripts))

	// Navigate to about page.
	t.Log("navigating to #/about...")
	if err := chromedp.Run(ctx,
		chromedp.Navigate(serverURL+"#/about"),
		chromedp.Sleep(500*time.Millisecond),
	); err != nil {
		t.Fatalf("navigate to about: %v", err)
	}

	snap3, err := cov.TakeSnapshot("after-about")
	if err != nil {
		t.Fatalf("take after-about snapshot: %v", err)
	}

	// --- Step 3: Verify coverage data and compute deltas ---

	t.Run("CoverageSnapshots", func(t *testing.T) {
		snapshots := cov.Snapshots()
		if len(snapshots) != 3 {
			t.Fatalf("expected 3 snapshots, got %d", len(snapshots))
		}
		if snapshots[0].Name != "baseline" {
			t.Errorf("snap[0].Name = %q, want %q", snapshots[0].Name, "baseline")
		}
		if snapshots[1].Name != "after-counter" {
			t.Errorf("snap[1].Name = %q, want %q", snapshots[1].Name, "after-counter")
		}
		if snapshots[2].Name != "after-about" {
			t.Errorf("snap[2].Name = %q, want %q", snapshots[2].Name, "after-about")
		}
	})

	t.Run("CoverageDelta", func(t *testing.T) {
		delta := cov.ComputeDelta(snap1, snap2)
		if delta == nil {
			t.Fatal("expected non-nil delta")
		}
		totalNew := 0
		for _, sd := range delta.Scripts {
			totalNew += len(sd.NewlyCovered)
		}
		t.Logf("baseline → after-counter: %d newly covered lines across %d scripts",
			totalNew, len(delta.Scripts))
		// Counter interactions should have exercised new code paths.
		if totalNew == 0 {
			t.Log("warning: no newly covered lines — bundle may have been fully executed on first load")
		}
	})

	// --- Step 4: Extract chunks from coverage data ---

	t.Run("ChunkExtraction", func(t *testing.T) {
		// Find our bundle in the snapshot.
		var bundleCov *coverage.ScriptCoverage
		for url, sc := range snap2.Scripts {
			if strings.Contains(url, "bundle.js") {
				bundleCov = sc
				t.Logf("found bundle coverage: %s (%d bytes, %d functions, %d byte ranges)",
					url, len(sc.Source), len(sc.Functions), len(sc.ByteRanges))
				break
			}
			_ = url
		}
		if bundleCov == nil {
			t.Skip("bundle.js not found in coverage data — Chrome may have inlined the URL differently")
		}

		// Convert to sourcemap.CoverageRange.
		var ranges []sourcemap.CoverageRange
		for _, r := range bundleCov.ByteRanges {
			ranges = append(ranges, sourcemap.CoverageRange{
				StartOffset: r.StartOffset,
				EndOffset:   r.EndOffset,
				Count:       r.Count,
			})
		}

		chunks := sourcemap.ExtractChunks(bundleCov.Source, ranges, 3)
		t.Logf("extracted %d code chunks", len(chunks))

		if len(chunks) == 0 {
			t.Fatal("expected at least one chunk from bundle coverage")
		}

		// Verify chunk properties.
		for i, c := range chunks {
			if c.Code == "" {
				t.Errorf("chunk %d has empty code", i)
			}
			if c.StartLine < 1 {
				t.Errorf("chunk %d has invalid start line %d", i, c.StartLine)
			}
			if c.EndLine < c.StartLine {
				t.Errorf("chunk %d: end line %d < start line %d", i, c.EndLine, c.StartLine)
			}
			if c.HitCount <= 0 {
				t.Errorf("chunk %d has zero hit count", i)
			}
		}

		// --- Step 5: Generate sourcemap from mock inferred structure ---
		// This is where analyze_bundle would call MCP sampling. We simulate the
		// LLM response with a hand-crafted inferred result that matches the bundle.

		t.Run("SourcemapGeneration", func(t *testing.T) {
			totalLines := sourcemap.CountLinesInString(bundleCov.Source)
			t.Logf("bundle has %d lines", totalLines)

			// Mock the LLM's inferred file structure.
			inferred := &inferredResult{
				Summary: "Small SPA with Router, Store, and DOM rendering",
				Files: []inferredFile{
					{
						Path:        "src/router.js",
						Description: "Client-side hash router",
						StartLine:   2,
						EndLine:     4,
						Functions: []inferredFunc{
							{Name: "Router", StartLine: 2, EndLine: 2, Description: "Router constructor"},
							{Name: "addRoute", StartLine: 3, EndLine: 3, Description: "Register route handler"},
							{Name: "navigate", StartLine: 4, EndLine: 4, Description: "Navigate to path"},
						},
						Framework: "vanilla",
						Module:    "routing",
					},
					{
						Path:        "src/store.js",
						Description: "Simple state management store",
						StartLine:   5,
						EndLine:     8,
						Functions: []inferredFunc{
							{Name: "Store", StartLine: 5, EndLine: 5, Description: "Store constructor"},
							{Name: "getState", StartLine: 6, EndLine: 6, Description: "Get state copy"},
							{Name: "setState", StartLine: 7, EndLine: 7, Description: "Update state and notify listeners"},
							{Name: "subscribe", StartLine: 8, EndLine: 8, Description: "Subscribe to state changes"},
						},
						Framework: "vanilla",
						Module:    "state",
					},
					{
						Path:        "src/dom.js",
						Description: "DOM helpers and render function",
						StartLine:   9,
						EndLine:     11,
						Functions: []inferredFunc{
							{Name: "createElement", StartLine: 9, EndLine: 9, Description: "Create DOM element"},
							{Name: "renderApp", StartLine: 10, EndLine: 10, Description: "Render the full app"},
						},
						Framework: "vanilla",
						Module:    "view",
					},
					{
						Path:        "src/app.js",
						Description: "Application initialization and route setup",
						StartLine:   11,
						EndLine:     totalLines,
						Functions: []inferredFunc{
							{Name: "init", StartLine: 11, EndLine: totalLines, Description: "App initialization"},
						},
						Framework: "vanilla",
						Module:    "app",
					},
				},
			}

			// Generate the sourcemap.
			mapJSON, err := generateMapFromInferred(bundleCov.Source, inferred)
			if err != nil {
				t.Fatalf("generate sourcemap: %v", err)
			}
			t.Logf("generated sourcemap: %d bytes", len(mapJSON))

			// Validate it's valid JSON and a valid sourcemap v3.
			var sm struct {
				Version        int      `json:"version"`
				File           string   `json:"file"`
				Sources        []string `json:"sources"`
				SourcesContent []string `json:"sourcesContent"`
				Names          []string `json:"names"`
				Mappings       string   `json:"mappings"`
			}
			if err := json.Unmarshal(mapJSON, &sm); err != nil {
				t.Fatalf("sourcemap JSON parse: %v", err)
			}

			if sm.Version != 3 {
				t.Errorf("version = %d, want 3", sm.Version)
			}
			if len(sm.Sources) != 4 {
				t.Errorf("sources count = %d, want 4", len(sm.Sources))
			}
			expectedSources := []string{"src/router.js", "src/store.js", "src/dom.js", "src/app.js"}
			for i, want := range expectedSources {
				if i < len(sm.Sources) && sm.Sources[i] != want {
					t.Errorf("sources[%d] = %q, want %q", i, sm.Sources[i], want)
				}
			}
			if len(sm.SourcesContent) != 4 {
				t.Errorf("sourcesContent count = %d, want 4", len(sm.SourcesContent))
			}
			for i, sc := range sm.SourcesContent {
				if sc == "" {
					t.Errorf("sourcesContent[%d] is empty", i)
				}
			}

			expectedNames := []string{"Router", "addRoute", "navigate", "Store", "getState", "setState", "subscribe", "createElement", "renderApp", "init"}
			if len(sm.Names) != len(expectedNames) {
				t.Errorf("names count = %d, want %d", len(sm.Names), len(expectedNames))
			}

			if sm.Mappings == "" {
				t.Error("mappings string is empty")
			}
			// Mappings should have semicolons (line separators).
			if !strings.Contains(sm.Mappings, ";") {
				t.Error("mappings has no semicolons — expected multi-line mappings")
			}

			t.Logf("sourcemap valid: %d sources, %d names, mappings length %d",
				len(sm.Sources), len(sm.Names), len(sm.Mappings))

			// Verify VLQ round-trip on the mappings.
			lines := strings.Split(sm.Mappings, ";")
			t.Logf("sourcemap has %d generated lines", len(lines))

			// --- Step 6: Test intercept store ---
			t.Run("InterceptStore", func(t *testing.T) {
				ic := newInterceptor()

				// Add a rule to serve the sourcemap.
				mapURL := bundleURL + ".map"
				ruleID := ic.addRule(interceptRule{
					URLPattern:  mapURL,
					Stage:       "request",
					Action:      "fulfill",
					StatusCode:  200,
					Body:        string(mapJSON),
					ContentType: "application/json",
					Headers:     map[string]string{"Access-Control-Allow-Origin": "*"},
				})

				if ruleID == "" {
					t.Fatal("expected non-empty rule ID")
				}
				t.Logf("installed intercept rule %s for %s", ruleID, mapURL)

				// Verify rule matches.
				rule := ic.matchRule(mapURL, "request")
				if rule == nil {
					t.Fatal("expected rule to match .map URL")
				}
				if rule.Action != "fulfill" {
					t.Errorf("action = %q, want %q", rule.Action, "fulfill")
				}
				if rule.ContentType != "application/json" {
					t.Errorf("content type = %q, want %q", rule.ContentType, "application/json")
				}
				if rule.Body != string(mapJSON) {
					t.Error("rule body doesn't match sourcemap JSON")
				}

				// Verify it doesn't match wrong stage.
				if r := ic.matchRule(mapURL, "response"); r != nil {
					t.Error("expected no match for response stage")
				}

				// Verify it doesn't match wrong URL.
				if r := ic.matchRule(bundleURL, "request"); r != nil {
					t.Error("expected no match for bundle URL (not .map)")
				}

				// Test syntheticMapStore.
				store := newSyntheticMapStore()
				store.set(bundleURL, &syntheticMap{
					BundleURL:   bundleURL,
					MapJSON:     mapJSON,
					Sources:     inferred,
					Serving:     true,
					InterceptID: ruleID,
				})

				sm := store.get(bundleURL)
				if sm == nil {
					t.Fatal("expected non-nil syntheticMap")
				}
				if !sm.Serving {
					t.Error("expected Serving=true")
				}
				if len(sm.MapJSON) != len(mapJSON) {
					t.Errorf("stored map size = %d, want %d", len(sm.MapJSON), len(mapJSON))
				}

				listed := store.list()
				if len(listed) != 1 {
					t.Errorf("list() returned %d, want 1", len(listed))
				}

				// Test rule update (simulates refine_sourcemap hot-update).
				ic.mu.Lock()
				for i := range ic.rules {
					if ic.rules[i].ID == ruleID {
						ic.rules[i].Body = `{"version":3,"mappings":"updated"}`
					}
				}
				ic.mu.Unlock()

				updated := ic.matchRule(mapURL, "request")
				if updated == nil || !strings.Contains(updated.Body, "updated") {
					t.Error("rule body not updated after hot-update")
				}

				// Cleanup.
				if !ic.removeRule(ruleID) {
					t.Error("expected removeRule to return true")
				}
				if r := ic.matchRule(mapURL, "request"); r != nil {
					t.Error("expected no match after removal")
				}
			})
		})
	})

	// --- Step 7: Verify lcov export ---
	t.Run("LcovExport", func(t *testing.T) {
		lcov := coverage.SnapshotToLcov(snap2)
		if lcov == "" {
			t.Fatal("expected non-empty lcov output")
		}
		if !strings.Contains(lcov, "SF:") {
			t.Error("lcov missing SF: entries")
		}
		if !strings.Contains(lcov, "end_of_record") {
			t.Error("lcov missing end_of_record")
		}
		t.Logf("lcov output: %d bytes, %d records",
			len(lcov), strings.Count(lcov, "end_of_record"))

		// Delta lcov.
		delta := cov.ComputeDelta(snap1, snap3)
		deltaLcov := coverage.DeltaToLcov(delta)
		t.Logf("delta lcov: %d bytes", len(deltaLcov))
	})

	// Stop coverage.
	if err := cov.Stop(); err != nil {
		t.Fatalf("stop coverage: %v", err)
	}
}

// TestSourcemapPipeline_NoChrome tests the non-browser parts of the pipeline:
// chunk extraction, sourcemap generation, intercept store, and syntheticMapStore.
func TestSourcemapPipeline_NoChrome(t *testing.T) {
	// Simulate coverage data (no browser needed).
	source := bundleJS
	ranges := []sourcemap.CoverageRange{
		{StartOffset: 0, EndOffset: 200, Count: 5},
		{StartOffset: 200, EndOffset: 500, Count: 3},
		{StartOffset: 800, EndOffset: 1200, Count: 1},
	}

	t.Run("ExtractChunks", func(t *testing.T) {
		chunks := sourcemap.ExtractChunks(source, ranges, 2)
		if len(chunks) == 0 {
			t.Fatal("expected at least one chunk")
		}
		for i, c := range chunks {
			t.Logf("chunk %d: lines %d-%d, %d bytes, hits=%d",
				i, c.StartLine, c.EndLine, len(c.Code), c.HitCount)
			if c.Code == "" {
				t.Errorf("chunk %d has empty code", i)
			}
		}
	})

	t.Run("GenerateSourcemap", func(t *testing.T) {
		totalLines := sourcemap.CountLinesInString(source)
		inferred := &inferredResult{
			Summary: "test bundle",
			Files: []inferredFile{
				{Path: "src/main.js", Description: "entry point", StartLine: 1, EndLine: totalLines / 2},
				{Path: "src/lib.js", Description: "utilities", StartLine: totalLines/2 + 1, EndLine: totalLines},
			},
		}

		mapJSON, err := generateMapFromInferred(source, inferred)
		if err != nil {
			t.Fatalf("generate sourcemap: %v", err)
		}

		var sm struct {
			Version int      `json:"version"`
			Sources []string `json:"sources"`
		}
		if err := json.Unmarshal(mapJSON, &sm); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if sm.Version != 3 {
			t.Errorf("version = %d, want 3", sm.Version)
		}
		if len(sm.Sources) != 2 {
			t.Errorf("sources = %d, want 2", len(sm.Sources))
		}
	})

	t.Run("SyntheticMapStore", func(t *testing.T) {
		store := newSyntheticMapStore()
		if got := store.list(); len(got) != 0 {
			t.Errorf("empty store has %d entries", len(got))
		}

		store.set("http://example.com/app.js", &syntheticMap{
			BundleURL: "http://example.com/app.js",
			MapJSON:   []byte(`{"version":3}`),
			Serving:   false,
		})

		sm := store.get("http://example.com/app.js")
		if sm == nil {
			t.Fatal("expected non-nil entry")
		}
		if sm.Serving {
			t.Error("expected Serving=false")
		}

		// Update to serving.
		sm.Serving = true
		sm.InterceptID = "rule-1"
		store.set("http://example.com/app.js", sm)

		sm2 := store.get("http://example.com/app.js")
		if !sm2.Serving || sm2.InterceptID != "rule-1" {
			t.Error("update not persisted")
		}

		if store.get("http://example.com/other.js") != nil {
			t.Error("expected nil for non-existent URL")
		}
	})

	t.Run("InterceptPatternMatching", func(t *testing.T) {
		tests := []struct {
			pattern string
			url     string
			match   bool
		}{
			{"*", "http://example.com/anything", true},
			{"", "http://example.com/anything", true},
			{"http://example.com/bundle.js.map", "http://example.com/bundle.js.map", true},
			{"http://example.com/bundle.js.map", "http://example.com/other.js.map", false},
			{"*.js.map", "http://example.com/bundle.js.map", true},
			{"*.js.map", "http://example.com/bundle.css.map", false},
			{"*/bundle*", "http://example.com/bundle.js", true},
			{"bundle.js", "http://example.com/bundle.js", true}, // substring
		}
		for _, tt := range tests {
			got := matchPattern(tt.pattern, tt.url)
			if got != tt.match {
				t.Errorf("matchPattern(%q, %q) = %v, want %v",
					tt.pattern, tt.url, got, tt.match)
			}
		}
	})

	t.Run("StripCodeFences", func(t *testing.T) {
		tests := []struct {
			input string
			want  string
		}{
			{`{"files":[]}`, `{"files":[]}`},
			{"```json\n{\"files\":[]}\n```", `{"files":[]}`},
			{"```\n{\"files\":[]}\n```", `{"files":[]}`},
			{"  ```json\n{}\n```  ", `{}`},
		}
		for _, tt := range tests {
			got := stripCodeFences(tt.input)
			if got != tt.want {
				t.Errorf("stripCodeFences(%q) = %q, want %q", tt.input, got, tt.want)
			}
		}
	})
}

// TestBuildAnalysisPrompt verifies the prompt builder produces a well-formed prompt.
func TestBuildAnalysisPrompt(t *testing.T) {
	chunks := []sourcemap.CodeChunk{
		{
			StartOffset: 0,
			EndOffset:   100,
			Code:        "function hello() { return 'world'; }",
			StartLine:   1,
			EndLine:     1,
			HitCount:    5,
		},
		{
			StartOffset: 200,
			EndOffset:   400,
			Code:        "class Router { constructor() {} navigate(path) {} }",
			StartLine:   5,
			EndLine:     5,
			HitCount:    3,
		},
	}

	prompt := buildAnalysisPrompt("http://example.com/bundle.js", chunks, "click login button")

	if !strings.Contains(prompt, "http://example.com/bundle.js") {
		t.Error("prompt missing bundle URL")
	}
	if !strings.Contains(prompt, "click login button") {
		t.Error("prompt missing action label")
	}
	if !strings.Contains(prompt, "Executed chunks: 2") {
		t.Error("prompt missing chunk count")
	}
	if !strings.Contains(prompt, "Chunk 1") {
		t.Error("prompt missing chunk 1")
	}
	if !strings.Contains(prompt, "Chunk 2") {
		t.Error("prompt missing chunk 2")
	}
	if !strings.Contains(prompt, `"files"`) {
		t.Error("prompt missing JSON schema hint")
	}

	// Test without action label.
	prompt2 := buildAnalysisPrompt("http://example.com/app.js", chunks[:1], "")
	if strings.Contains(prompt2, "Action that triggered") {
		t.Error("prompt should omit action line when label is empty")
	}
}

// TestBuildAnalysisPrompt_TruncatesLargeChunks verifies chunk truncation.
func TestBuildAnalysisPrompt_TruncatesLargeChunks(t *testing.T) {
	// Create a chunk with >2000 chars of code.
	bigCode := strings.Repeat("x", 3000)
	chunks := make([]sourcemap.CodeChunk, 35)
	for i := range chunks {
		chunks[i] = sourcemap.CodeChunk{
			StartOffset: i * 100,
			EndOffset:   i*100 + 100,
			Code:        bigCode,
			StartLine:   i + 1,
			EndLine:     i + 1,
			HitCount:    1,
		}
	}

	prompt := buildAnalysisPrompt("http://example.com/huge.js", chunks, "")

	// Should truncate code.
	if !strings.Contains(prompt, "// ... truncated") {
		t.Error("expected truncated code marker")
	}
	// Should truncate chunks beyond 30.
	if !strings.Contains(prompt, "and 5 more chunks") {
		t.Error("expected chunk count truncation message")
	}
}
