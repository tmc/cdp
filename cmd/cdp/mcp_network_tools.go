package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// networkEntry represents a captured network request/response pair.
type networkEntry struct {
	RequestID string            `json:"request_id"`
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	Status    int64             `json:"status,omitempty"`
	MIMEType  string            `json:"mime_type,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Timestamp string            `json:"timestamp"`
	Duration  float64           `json:"duration_ms,omitempty"`
	Size      float64           `json:"size,omitempty"`
	Finished  bool              `json:"finished"`
}

// networkCollector captures live network events via the CDP Network domain.
type networkCollector struct {
	mu         sync.Mutex
	entries    map[string]*networkEntry // keyed by request ID
	order      []string                // request IDs in order
	maxEntries int
	running    bool
}

func newNetworkCollector() *networkCollector {
	return &networkCollector{
		entries:    make(map[string]*networkEntry),
		maxEntries: 2000,
	}
}

func (nc *networkCollector) handleEvent(ev any) {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	switch e := ev.(type) {
	case *network.EventRequestWillBeSent:
		id := string(e.RequestID)
		ts := ""
		if e.Timestamp != nil {
			ts = time.Time(*e.Timestamp).Format(time.RFC3339Nano)
		}
		headers := make(map[string]string)
		if e.Request.Headers != nil {
			for k, v := range e.Request.Headers {
				if s, ok := v.(string); ok {
					headers[k] = s
				}
			}
		}
		entry := &networkEntry{
			RequestID: id,
			URL:       e.Request.URL,
			Method:    e.Request.Method,
			Headers:   headers,
			Timestamp: ts,
		}
		nc.entries[id] = entry
		nc.order = append(nc.order, id)
		// Evict oldest if over limit.
		if len(nc.order) > nc.maxEntries {
			old := nc.order[0]
			nc.order = nc.order[1:]
			delete(nc.entries, old)
		}

	case *network.EventResponseReceived:
		id := string(e.RequestID)
		entry, ok := nc.entries[id]
		if !ok {
			return
		}
		entry.Status = e.Response.Status
		entry.MIMEType = e.Response.MimeType

	case *network.EventLoadingFinished:
		id := string(e.RequestID)
		entry, ok := nc.entries[id]
		if !ok {
			return
		}
		entry.Finished = true
		entry.Size = e.EncodedDataLength
		if e.Timestamp != nil {
			// Compute duration from request timestamp.
			if entry.Timestamp != "" {
				if start, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
					entry.Duration = float64(time.Time(*e.Timestamp).Sub(start).Milliseconds())
				}
			}
		}
	}
}

func (nc *networkCollector) getEntries(urlFilter string, limit int) []networkEntry {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	var result []networkEntry
	for _, id := range nc.order {
		entry, ok := nc.entries[id]
		if !ok {
			continue
		}
		if urlFilter != "" && !strings.Contains(entry.URL, urlFilter) {
			continue
		}
		result = append(result, *entry)
	}
	if limit > 0 && len(result) > limit {
		result = result[len(result)-limit:]
	}
	return result
}

func (nc *networkCollector) clear() {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	nc.entries = make(map[string]*networkEntry)
	nc.order = nil
}

// --- MCP tool registration ---

type StartNetworkLogInput struct{}

type GetNetworkLogInput struct {
	URL   string `json:"url,omitempty"`   // filter by URL substring
	Limit int    `json:"limit,omitempty"` // max entries to return
	Clear bool   `json:"clear,omitempty"` // clear log after reading
}

func registerNetworkTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_network_log",
		Description: `Start live network request logging via CDP Network domain. Captures URL, method, status, headers, timing. Use get_network_log to read entries.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StartNetworkLogInput) (*mcp.CallToolResult, any, error) {
		s.mu.Lock()
		if s.networkLog != nil && s.networkLog.running {
			s.mu.Unlock()
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "network log already active"}},
			}, nil, nil
		}

		nc := newNetworkCollector()
		s.networkLog = nc
		s.mu.Unlock()

		actx := s.activeCtx()
		if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
			return network.Enable().Do(ctx)
		})); err != nil {
			return nil, nil, fmt.Errorf("start_network_log: enable network: %w", err)
		}

		chromedp.ListenTarget(actx, nc.handleEvent)
		nc.mu.Lock()
		nc.running = true
		nc.mu.Unlock()

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "network log started"}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_network_log",
		Description: `Get captured network requests from start_network_log. Optional URL filter (substring match). Set clear=true to reset.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetNetworkLogInput) (*mcp.CallToolResult, any, error) {
		s.mu.Lock()
		nc := s.networkLog
		s.mu.Unlock()

		if nc == nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "network log not started — call start_network_log first"}},
			}, nil, nil
		}

		entries := nc.getEntries(input.URL, input.Limit)
		if input.Clear {
			nc.clear()
		}

		if len(entries) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no network entries"}},
			}, nil, nil
		}

		data, err := json.Marshal(entries)
		if err != nil {
			return nil, nil, fmt.Errorf("get_network_log: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}
