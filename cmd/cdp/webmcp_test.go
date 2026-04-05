package main

import (
	"encoding/json"
	"testing"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/webmcp"
	"github.com/go-json-experiment/json/jsontext"
)

func TestWebMCPCollector_ToolsAddedRemoved(t *testing.T) {
	c := newWebMCPCollector()

	// Simulate toolsAdded event with two tools.
	c.handleEvent(&webmcp.EventToolsAdded{
		Tools: []*webmcp.Tool{
			{
				Name:        "get_weather",
				Description: "Get weather for a city",
				InputSchema: jsontext.Value(`{"type":"object","properties":{"city":{"type":"string"}}}`),
				FrameID:     cdp.FrameID("frame1"),
			},
			{
				Name:        "search_docs",
				Description: "Search documentation",
				InputSchema: jsontext.Value(`{"type":"object","properties":{"query":{"type":"string"}}}`),
				FrameID:     cdp.FrameID("frame1"),
				Annotations: &webmcp.Annotation{ReadOnly: true},
			},
		},
	})

	tools := c.listTools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	// Get by name.
	w := c.getTool("get_weather")
	if w == nil {
		t.Fatal("expected to find get_weather")
	}
	if w.Description != "Get weather for a city" {
		t.Errorf("description = %q", w.Description)
	}

	s := c.getTool("search_docs")
	if s == nil || s.Annotations == nil || !s.Annotations.ReadOnly {
		t.Error("expected search_docs with ReadOnly annotation")
	}

	// Missing tool.
	if c.getTool("nonexistent") != nil {
		t.Error("expected nil for missing tool")
	}

	// Simulate toolsRemoved.
	c.handleEvent(&webmcp.EventToolsRemoved{
		Tools: []*webmcp.Tool{
			{Name: "get_weather", FrameID: cdp.FrameID("frame1")},
		},
	})

	tools = c.listTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool after removal, got %d", len(tools))
	}
	if c.getTool("get_weather") != nil {
		t.Error("get_weather should have been removed")
	}
	if c.getTool("search_docs") == nil {
		t.Error("search_docs should still exist")
	}
}

func TestWebMCPCollector_Invocations(t *testing.T) {
	c := newWebMCPCollector()

	// Simulate tool invocation start.
	c.handleEvent(&webmcp.EventToolInvoked{
		ToolName:     "get_weather",
		FrameID:      cdp.FrameID("frame1"),
		InvocationID: "inv-001",
		Input:        `{"city":"London"}`,
	})

	invocations := c.listInvocations(0)
	if len(invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(invocations))
	}
	if invocations[0].ToolName != "get_weather" {
		t.Errorf("tool name = %q", invocations[0].ToolName)
	}
	if invocations[0].Input != `{"city":"London"}` {
		t.Errorf("input = %q", invocations[0].Input)
	}
	if invocations[0].Status != "" {
		t.Errorf("status should be empty before response, got %q", invocations[0].Status)
	}

	// Simulate response.
	c.handleEvent(&webmcp.EventToolResponded{
		InvocationID: "inv-001",
		Status:       webmcp.InvocationStatusSuccess,
		Output:       jsontext.Value(`{"temp":18,"unit":"celsius"}`),
	})

	invocations = c.listInvocations(0)
	if invocations[0].Status != "Success" {
		t.Errorf("status = %q, want Success", invocations[0].Status)
	}
	if invocations[0].Output != `{"temp":18,"unit":"celsius"}` {
		t.Errorf("output = %q", invocations[0].Output)
	}
	if invocations[0].EndTime == "" {
		t.Error("expected end_time to be set")
	}
}

func TestWebMCPCollector_InvocationError(t *testing.T) {
	c := newWebMCPCollector()

	c.handleEvent(&webmcp.EventToolInvoked{
		ToolName:     "broken_tool",
		FrameID:      cdp.FrameID("frame1"),
		InvocationID: "inv-err",
		Input:        "{}",
	})

	c.handleEvent(&webmcp.EventToolResponded{
		InvocationID: "inv-err",
		Status:       webmcp.InvocationStatusError,
		ErrorText:    "network timeout",
	})

	invocations := c.listInvocations(0)
	if len(invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(invocations))
	}
	if invocations[0].Status != "Error" {
		t.Errorf("status = %q, want Error", invocations[0].Status)
	}
	if invocations[0].ErrorText != "network timeout" {
		t.Errorf("error_text = %q", invocations[0].ErrorText)
	}
}

func TestWebMCPCollector_InvocationCanceled(t *testing.T) {
	c := newWebMCPCollector()

	c.handleEvent(&webmcp.EventToolInvoked{
		ToolName:     "slow_tool",
		FrameID:      cdp.FrameID("frame1"),
		InvocationID: "inv-cancel",
		Input:        "{}",
	})

	c.handleEvent(&webmcp.EventToolResponded{
		InvocationID: "inv-cancel",
		Status:       webmcp.InvocationStatusCanceled,
	})

	invocations := c.listInvocations(0)
	if invocations[0].Status != "Canceled" {
		t.Errorf("status = %q, want Canceled", invocations[0].Status)
	}
}

func TestWebMCPCollector_MaxEntries(t *testing.T) {
	c := newWebMCPCollector()
	c.maxEntries = 3

	for i := 0; i < 5; i++ {
		c.handleEvent(&webmcp.EventToolInvoked{
			ToolName:     "tool",
			FrameID:      cdp.FrameID("frame1"),
			InvocationID: string(rune('a' + i)),
			Input:        "{}",
		})
	}

	invocations := c.listInvocations(0)
	if len(invocations) != 3 {
		t.Fatalf("expected 3 invocations (maxEntries), got %d", len(invocations))
	}
	// Oldest entries should have been evicted.
	if invocations[0].InvocationID != "c" {
		t.Errorf("oldest invocation should be 'c', got %q", invocations[0].InvocationID)
	}
}

func TestWebMCPCollector_ListInvocationsLast(t *testing.T) {
	c := newWebMCPCollector()

	for i := 0; i < 10; i++ {
		c.handleEvent(&webmcp.EventToolInvoked{
			ToolName:     "tool",
			FrameID:      cdp.FrameID("frame1"),
			InvocationID: string(rune('a' + i)),
			Input:        "{}",
		})
	}

	last3 := c.listInvocations(3)
	if len(last3) != 3 {
		t.Fatalf("expected 3, got %d", len(last3))
	}
	// Should be the last 3.
	if last3[0].InvocationID != "h" {
		t.Errorf("expected 'h', got %q", last3[0].InvocationID)
	}
}

func TestWebMCPCollector_ToolReplace(t *testing.T) {
	c := newWebMCPCollector()

	// Add a tool.
	c.handleEvent(&webmcp.EventToolsAdded{
		Tools: []*webmcp.Tool{
			{Name: "weather", Description: "v1", FrameID: cdp.FrameID("f1")},
		},
	})

	// Re-add with updated description (same name).
	c.handleEvent(&webmcp.EventToolsAdded{
		Tools: []*webmcp.Tool{
			{Name: "weather", Description: "v2", FrameID: cdp.FrameID("f1")},
		},
	})

	tools := c.listTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Description != "v2" {
		t.Errorf("description = %q, want v2", tools[0].Description)
	}
}

func TestWebMCPCollector_DeclarativeTool(t *testing.T) {
	c := newWebMCPCollector()

	c.handleEvent(&webmcp.EventToolsAdded{
		Tools: []*webmcp.Tool{
			{
				Name:          "submit_form",
				Description:   "Submit the login form",
				FrameID:       cdp.FrameID("f1"),
				BackendNodeID: cdp.BackendNodeID(42),
				Annotations:   &webmcp.Annotation{Autosubmit: true},
			},
		},
	})

	tool := c.getTool("submit_form")
	if tool == nil {
		t.Fatal("expected submit_form tool")
	}
	if tool.BackendNodeID != 42 {
		t.Errorf("backend_node_id = %d, want 42", tool.BackendNodeID)
	}
	if !tool.Annotations.Autosubmit {
		t.Error("expected autosubmit annotation")
	}
}

func TestWebMCPCollector_JSONSerialization(t *testing.T) {
	c := newWebMCPCollector()

	c.handleEvent(&webmcp.EventToolInvoked{
		ToolName:     "test_tool",
		FrameID:      cdp.FrameID("frame1"),
		InvocationID: "inv-json",
		Input:        `{"key":"value"}`,
	})
	c.handleEvent(&webmcp.EventToolResponded{
		InvocationID: "inv-json",
		Status:       webmcp.InvocationStatusSuccess,
		Output:       jsontext.Value(`{"result":"ok"}`),
	})

	invocations := c.listInvocations(0)
	data, err := json.Marshal(invocations[0])
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["tool_name"] != "test_tool" {
		t.Errorf("tool_name = %v", parsed["tool_name"])
	}
	if parsed["status"] != "Success" {
		t.Errorf("status = %v", parsed["status"])
	}
}

func TestWebMCPCollector_UnmatchedResponse(t *testing.T) {
	c := newWebMCPCollector()

	// Response with no matching invocation — should not panic.
	c.handleEvent(&webmcp.EventToolResponded{
		InvocationID: "orphan",
		Status:       webmcp.InvocationStatusError,
		ErrorText:    "no match",
	})

	invocations := c.listInvocations(0)
	if len(invocations) != 0 {
		t.Errorf("expected 0 invocations, got %d", len(invocations))
	}
}
