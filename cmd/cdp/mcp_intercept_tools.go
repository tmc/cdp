package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// interceptRule defines what to do when a request matches.
type interceptRule struct {
	ID          string            `json:"id"`
	URLPattern  string            `json:"url_pattern"`
	Stage       string            `json:"stage"` // "request" or "response"
	Action      string            `json:"action"` // "block", "modify", "fulfill", "continue"
	StatusCode  int64             `json:"status_code,omitempty"`
	Body        string            `json:"body,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
}

// interceptor manages Fetch domain interception rules and event handling.
type interceptor struct {
	mu      sync.Mutex
	rules   []interceptRule
	counter int
	enabled bool
}

func newInterceptor() *interceptor {
	return &interceptor{}
}

func (ic *interceptor) addRule(rule interceptRule) string {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.counter++
	rule.ID = fmt.Sprintf("rule-%d", ic.counter)
	ic.rules = append(ic.rules, rule)
	return rule.ID
}

func (ic *interceptor) removeRule(id string) bool {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	for i, r := range ic.rules {
		if r.ID == id {
			ic.rules = append(ic.rules[:i], ic.rules[i+1:]...)
			return true
		}
	}
	return false
}

func (ic *interceptor) getRules() []interceptRule {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	result := make([]interceptRule, len(ic.rules))
	copy(result, ic.rules)
	return result
}

func (ic *interceptor) clearRules() int {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	n := len(ic.rules)
	ic.rules = nil
	return n
}

// matchRule finds the first matching rule for a request URL and stage.
func (ic *interceptor) matchRule(url, stage string) *interceptRule {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	for _, r := range ic.rules {
		if r.Stage != stage {
			continue
		}
		if matchPattern(r.URLPattern, url) {
			result := r
			return &result
		}
	}
	return nil
}

// matchPattern does simple wildcard matching (* for any sequence, ? for single char).
func matchPattern(pattern, s string) bool {
	// Empty pattern matches everything.
	if pattern == "" || pattern == "*" {
		return true
	}
	// Simple substring match if no wildcards.
	if !strings.ContainsAny(pattern, "*?") {
		return strings.Contains(s, pattern)
	}
	// Wildcard matching.
	return wildcardMatch(pattern, s)
}

func wildcardMatch(pattern, s string) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			// Skip consecutive stars.
			for len(pattern) > 0 && pattern[0] == '*' {
				pattern = pattern[1:]
			}
			if len(pattern) == 0 {
				return true
			}
			// Try matching rest of pattern at every position.
			for i := 0; i <= len(s); i++ {
				if wildcardMatch(pattern, s[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(s) == 0 {
				return false
			}
			pattern = pattern[1:]
			s = s[1:]
		default:
			if len(s) == 0 || pattern[0] != s[0] {
				return false
			}
			pattern = pattern[1:]
			s = s[1:]
		}
	}
	return len(s) == 0
}

// handleRequestPaused is called from the CDP event listener when a request is paused.
func (ic *interceptor) handleRequestPaused(ctx context.Context, ev *fetch.EventRequestPaused) {
	// Determine stage.
	stage := "request"
	if ev.ResponseStatusCode > 0 || ev.ResponseErrorReason != "" {
		stage = "response"
	}

	rule := ic.matchRule(ev.Request.URL, stage)
	if rule == nil {
		// No matching rule — continue the request unmodified.
		if err := fetch.ContinueRequest(ev.RequestID).Do(ctx); err != nil {
			log.Printf("intercept: continue request: %v", err)
		}
		return
	}

	switch rule.Action {
	case "block":
		if err := fetch.FailRequest(ev.RequestID, network.ErrorReasonBlockedByClient).Do(ctx); err != nil {
			log.Printf("intercept: block request: %v", err)
		}

	case "fulfill":
		code := rule.StatusCode
		if code == 0 {
			code = 200
		}
		var headers []*fetch.HeaderEntry
		ct := rule.ContentType
		if ct == "" {
			ct = "text/plain"
		}
		headers = append(headers, &fetch.HeaderEntry{Name: "Content-Type", Value: ct})
		for k, v := range rule.Headers {
			headers = append(headers, &fetch.HeaderEntry{Name: k, Value: v})
		}

		body := base64.StdEncoding.EncodeToString([]byte(rule.Body))
		if err := fetch.FulfillRequest(ev.RequestID, code).
			WithResponseHeaders(headers).
			WithBody(body).Do(ctx); err != nil {
			log.Printf("intercept: fulfill request: %v", err)
		}

	case "modify":
		if stage == "response" {
			// Modify response headers/status.
			cmd := fetch.ContinueResponse(ev.RequestID)
			if rule.StatusCode > 0 {
				cmd = cmd.WithResponseCode(rule.StatusCode)
			}
			if len(rule.Headers) > 0 {
				// Start with existing headers and override.
				headerMap := make(map[string]string)
				for _, h := range ev.ResponseHeaders {
					headerMap[h.Name] = h.Value
				}
				for k, v := range rule.Headers {
					headerMap[k] = v
				}
				var headers []*fetch.HeaderEntry
				for k, v := range headerMap {
					headers = append(headers, &fetch.HeaderEntry{Name: k, Value: v})
				}
				cmd = cmd.WithResponseHeaders(headers)
			}
			if err := cmd.Do(ctx); err != nil {
				log.Printf("intercept: modify response: %v", err)
			}
		} else {
			// Modify request headers/URL.
			cmd := fetch.ContinueRequest(ev.RequestID)
			if len(rule.Headers) > 0 {
				var headers []*fetch.HeaderEntry
				for k, v := range rule.Headers {
					headers = append(headers, &fetch.HeaderEntry{Name: k, Value: v})
				}
				cmd = cmd.WithHeaders(headers)
			}
			if err := cmd.Do(ctx); err != nil {
				log.Printf("intercept: modify request: %v", err)
			}
		}

	default:
		if err := fetch.ContinueRequest(ev.RequestID).Do(ctx); err != nil {
			log.Printf("intercept: continue request: %v", err)
		}
	}
}

// --- MCP tool registration ---

type InterceptRequestInput struct {
	URLPattern  string            `json:"url_pattern"`
	Action      string            `json:"action"`
	StatusCode  int64             `json:"status_code,omitempty"`
	Body        string            `json:"body,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
}

type InterceptResponseInput struct {
	URLPattern string            `json:"url_pattern"`
	Action     string            `json:"action"`
	StatusCode int64             `json:"status_code,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
}

type RemoveInterceptInput struct {
	ID  string `json:"id,omitempty"`
	All bool   `json:"all,omitempty"`
}

func registerInterceptTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "intercept_request",
		Description: `Intercept outgoing requests matching a URL pattern. Actions: "block" (fail the request), "fulfill" (return custom response with status_code, body, content_type), "modify" (change request headers). Pattern supports wildcards (* and ?).`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input InterceptRequestInput) (*mcp.CallToolResult, any, error) {
		if input.URLPattern == "" {
			return nil, nil, fmt.Errorf("intercept_request: url_pattern is required")
		}
		if input.Action == "" {
			input.Action = "block"
		}

		rule := interceptRule{
			URLPattern:  input.URLPattern,
			Stage:       "request",
			Action:      input.Action,
			StatusCode:  input.StatusCode,
			Body:        input.Body,
			Headers:     input.Headers,
			ContentType: input.ContentType,
		}

		if err := ensureInterceptEnabled(s); err != nil {
			return nil, nil, fmt.Errorf("intercept_request: %w", err)
		}

		id := s.intercepts.addRule(rule)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("intercept rule %s added: %s %s on %s", id, input.Action, input.URLPattern, "request")}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "intercept_response",
		Description: `Intercept responses matching a URL pattern. Actions: "modify" (change response headers/status), "fulfill" (replace response body entirely). Pattern supports wildcards.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input InterceptResponseInput) (*mcp.CallToolResult, any, error) {
		if input.URLPattern == "" {
			return nil, nil, fmt.Errorf("intercept_response: url_pattern is required")
		}
		if input.Action == "" {
			input.Action = "modify"
		}

		rule := interceptRule{
			URLPattern: input.URLPattern,
			Stage:      "response",
			Action:     input.Action,
			StatusCode: input.StatusCode,
			Headers:    input.Headers,
			Body:       input.Body,
		}

		if err := ensureInterceptEnabled(s); err != nil {
			return nil, nil, fmt.Errorf("intercept_response: %w", err)
		}

		id := s.intercepts.addRule(rule)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("intercept rule %s added: %s %s on %s", id, input.Action, input.URLPattern, "response")}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "remove_intercept",
		Description: "Remove an intercept rule by ID, or remove all rules. Use list output from intercept_request/intercept_response to find IDs.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RemoveInterceptInput) (*mcp.CallToolResult, any, error) {
		if s.intercepts == nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no intercepts active"}},
			}, nil, nil
		}

		if input.All {
			n := s.intercepts.clearRules()
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("removed %d intercept rule(s)", n)}},
			}, nil, nil
		}

		if input.ID == "" {
			return nil, nil, fmt.Errorf("remove_intercept: id or all=true required")
		}

		if s.intercepts.removeRule(input.ID) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("removed intercept rule %s", input.ID)}},
			}, nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("rule %s not found", input.ID)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_intercepts",
		Description: "List all active intercept rules",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		if s.intercepts == nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no intercepts active"}},
			}, nil, nil
		}
		rules := s.intercepts.getRules()
		if len(rules) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no intercept rules"}},
			}, nil, nil
		}
		data, err := json.Marshal(rules)
		if err != nil {
			return nil, nil, fmt.Errorf("list_intercepts: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}

// ensureInterceptEnabled enables the Fetch domain if not already active.
func ensureInterceptEnabled(s *mcpSession) error {
	if s.intercepts == nil {
		s.intercepts = newInterceptor()
	}
	if s.intercepts.enabled {
		return nil
	}

	actx := s.activeCtx()

	// Enable Fetch domain with broad patterns to catch both request and response stages.
	if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
		return fetch.Enable().WithPatterns([]*fetch.RequestPattern{
			{URLPattern: "*", RequestStage: fetch.RequestStageRequest},
			{URLPattern: "*", RequestStage: fetch.RequestStageResponse},
		}).Do(ctx)
	})); err != nil {
		return fmt.Errorf("enable fetch domain: %w", err)
	}

	// Listen for paused requests.
	chromedp.ListenTarget(actx, func(ev interface{}) {
		if e, ok := ev.(*fetch.EventRequestPaused); ok {
			// Handle in the event's context (which has CDP access).
			go s.intercepts.handleRequestPaused(actx, e)
		}
	})

	s.intercepts.enabled = true
	return nil
}
