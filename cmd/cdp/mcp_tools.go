package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/har"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerMCPTools registers all MCP tool handlers on the given server.
func registerMCPTools(server *mcp.Server, session *mcpSession) {
	registerNavigationTools(server, session)
	registerObservationTools(server, session)
	registerInteractionTools(server, session)
	registerJavaScriptTools(server, session)
	registerTabTools(server, session)
	registerContextTools(server, session)
	registerHARTools(server, session)
	registerCookieTools(server, session)
	registerSourceTools(server, session)
	registerSourceBrowsingTools(server, session)
	registerCoverageTools(server, session)
	registerFindElementTool(server, session)
	registerElementQueryTools(server, session)
	registerConsoleTools(server, session)
	registerInputTools(server, session)
	registerFrameTools(server, session)
	registerScrollTool(server, session)
	registerDialogTools(server, session)
	registerFileTools(server, session)
	registerStorageTools(server, session)
	registerEmulationTools(server, session)
	registerInterceptTools(server, session)
	registerPDFTools(server, session)
	registerTraceTools(server, session)
	registerStateTools(server, session)
	registerDomDiffTools(server, session)
	registerSourcemapTools(server, session)
}

// --- Navigation tools ---

type NavigateInput struct {
	URL string `json:"url"`
}

type NavigateOutput struct {
	Title    string `json:"title"`
	Location string `json:"location"`
}

type NavigationOutput struct {
	Title string `json:"title"`
}

func registerNavigationTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "navigate",
		Description: "Navigate to a URL",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input NavigateInput) (*mcp.CallToolResult, NavigateOutput, error) {
		if err := chromedp.Run(s.activeCtx(), chromedp.Navigate(input.URL)); err != nil {
			return nil, NavigateOutput{}, fmt.Errorf("navigate: %w", err)
		}
		var out NavigateOutput
		if err := chromedp.Run(s.activeCtx(), chromedp.Title(&out.Title), chromedp.Location(&out.Location)); err != nil {
			return nil, NavigateOutput{}, fmt.Errorf("navigate: %w", err)
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "navigate_back",
		Description: "Navigate back in browser history",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, NavigationOutput, error) {
		if err := chromedp.Run(s.activeCtx(), chromedp.NavigateBack()); err != nil {
			return nil, NavigationOutput{}, fmt.Errorf("navigate_back: %w", err)
		}
		var out NavigationOutput
		if err := chromedp.Run(s.activeCtx(), chromedp.Title(&out.Title)); err != nil {
			return nil, NavigationOutput{}, fmt.Errorf("navigate_back: %w", err)
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "navigate_forward",
		Description: "Navigate forward in browser history",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, NavigationOutput, error) {
		if err := chromedp.Run(s.activeCtx(), chromedp.NavigateForward()); err != nil {
			return nil, NavigationOutput{}, fmt.Errorf("navigate_forward: %w", err)
		}
		var out NavigationOutput
		if err := chromedp.Run(s.activeCtx(), chromedp.Title(&out.Title)); err != nil {
			return nil, NavigationOutput{}, fmt.Errorf("navigate_forward: %w", err)
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "reload",
		Description: "Reload the current page",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, NavigationOutput, error) {
		if err := chromedp.Run(s.activeCtx(), chromedp.Reload()); err != nil {
			return nil, NavigationOutput{}, fmt.Errorf("reload: %w", err)
		}
		var out NavigationOutput
		if err := chromedp.Run(s.activeCtx(), chromedp.Title(&out.Title)); err != nil {
			return nil, NavigationOutput{}, fmt.Errorf("reload: %w", err)
		}
		return nil, out, nil
	})
}

// --- Observation tools ---

type ScreenshotInput struct {
	Selector string `json:"selector,omitempty"`
	FullPage bool   `json:"full_page,omitempty"`
}

type GetPageContentInput struct {
	Selector string `json:"selector,omitempty"`
	Format   string `json:"format,omitempty"`
}

func registerObservationTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "screenshot",
		Description: "Take a screenshot of the page or a specific element",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ScreenshotInput) (*mcp.CallToolResult, any, error) {
		var buf []byte
		if input.Selector != "" && !input.FullPage {
			if err := chromedp.Run(s.activeCtx(), chromedp.Screenshot(input.Selector, &buf, chromedp.ByQuery)); err != nil {
				return nil, nil, fmt.Errorf("screenshot: %w", err)
			}
		} else {
			if err := chromedp.Run(s.activeCtx(), chromedp.FullScreenshot(&buf, 100)); err != nil {
				return nil, nil, fmt.Errorf("screenshot: %w", err)
			}
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.ImageContent{
					Data:     buf,
					MIMEType: "image/png",
				},
			},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_page_content",
		Description: "Get the text or HTML content of the page or a specific element",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetPageContentInput) (*mcp.CallToolResult, any, error) {
		format := input.Format
		if format == "" {
			format = "text"
		}
		var text string
		if input.Selector != "" {
			switch format {
			case "html", "markdown":
				if err := chromedp.Run(s.activeCtx(), chromedp.OuterHTML(input.Selector, &text, chromedp.ByQuery)); err != nil {
					return nil, nil, fmt.Errorf("get_page_content: %w", err)
				}
			default:
				if err := chromedp.Run(s.activeCtx(), chromedp.Text(input.Selector, &text, chromedp.ByQuery)); err != nil {
					return nil, nil, fmt.Errorf("get_page_content: %w", err)
				}
			}
		} else {
			switch format {
			case "text":
				if err := chromedp.Run(s.activeCtx(), chromedp.Text("body", &text, chromedp.ByQuery)); err != nil {
					return nil, nil, fmt.Errorf("get_page_content: %w", err)
				}
			default:
				if err := chromedp.Run(s.activeCtx(), chromedp.OuterHTML("html", &text, chromedp.ByQuery)); err != nil {
					return nil, nil, fmt.Errorf("get_page_content: %w", err)
				}
			}
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "page_snapshot",
		Description: "Get an accessibility tree snapshot of the page. Interactive elements are annotated with @ref numbers (e.g. @1, @2) that can be used with click, type_text, and other interaction tools instead of CSS selectors.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		result, err := buildAXSnapshot(s.activeCtx(), s.refs)
		if err != nil {
			return nil, nil, fmt.Errorf("page_snapshot: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: result},
			},
		}, nil, nil
	})
}

// --- Interaction tools ---

type ClickInput struct {
	Selector string `json:"selector"`
}

type TypeTextInput struct {
	Selector string `json:"selector"`
	Text     string `json:"text"`
	Submit   bool   `json:"submit,omitempty"`
}

type WaitForInput struct {
	Selector string `json:"selector"`
	Timeout  int    `json:"timeout,omitempty"`
}

func registerInteractionTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "click",
		Description: "Click an element by CSS selector or @ref (e.g. @1 from page_snapshot)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ClickInput) (*mcp.CallToolResult, any, error) {
		actx := s.activeCtx()
		backendID, err := resolveRef(s.refs, input.Selector)
		if err != nil {
			return nil, nil, fmt.Errorf("click: %w", err)
		}
		if backendID != 0 {
			if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
				return clickByBackendNodeID(ctx, backendID)
			})); err != nil {
				return nil, nil, fmt.Errorf("click: %w", err)
			}
		} else {
			if err := chromedp.Run(actx, chromedp.Click(input.Selector, chromedp.ByQuery)); err != nil {
				return nil, nil, fmt.Errorf("click: %w", err)
			}
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "clicked " + input.Selector},
			},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "type_text",
		Description: "Type text into an element by CSS selector or @ref (e.g. @1 from page_snapshot)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input TypeTextInput) (*mcp.CallToolResult, any, error) {
		text := input.Text
		if input.Submit {
			text += "\n"
		}
		actx := s.activeCtx()
		backendID, err := resolveRef(s.refs, input.Selector)
		if err != nil {
			return nil, nil, fmt.Errorf("type_text: %w", err)
		}
		if backendID != 0 {
			if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
				return typeByBackendNodeID(ctx, backendID, text)
			})); err != nil {
				return nil, nil, fmt.Errorf("type_text: %w", err)
			}
		} else {
			if err := chromedp.Run(actx, chromedp.SendKeys(input.Selector, text, chromedp.ByQuery)); err != nil {
				return nil, nil, fmt.Errorf("type_text: %w", err)
			}
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "typed into " + input.Selector},
			},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "wait_for",
		Description: "Wait for an element to be visible by CSS selector or @ref",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input WaitForInput) (*mcp.CallToolResult, any, error) {
		actx := s.activeCtx()
		if input.Timeout > 0 {
			var cancel context.CancelFunc
			actx, cancel = context.WithTimeout(actx, time.Duration(input.Timeout)*time.Second)
			defer cancel()
		}
		// For @refs, check that the element exists (it was visible at snapshot time).
		backendID, err := resolveRef(s.refs, input.Selector)
		if err != nil {
			return nil, nil, fmt.Errorf("wait_for: %w", err)
		}
		if backendID != 0 {
			// Verify the node still exists by describing it.
			if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
				_, err := dom.DescribeNode().WithBackendNodeID(backendID).Do(ctx)
				return err
			})); err != nil {
				return nil, nil, fmt.Errorf("wait_for: ref element no longer exists: %w", err)
			}
		} else {
			if err := chromedp.Run(actx, chromedp.WaitVisible(input.Selector, chromedp.ByQuery)); err != nil {
				return nil, nil, fmt.Errorf("wait_for: %w", err)
			}
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "element visible: " + input.Selector},
			},
		}, nil, nil
	})
}

// --- JavaScript tools ---

type EvaluateInput struct {
	Expression   string `json:"expression"`
	AwaitPromise bool   `json:"await_promise,omitempty"`
}

func registerJavaScriptTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "evaluate",
		Description: "Evaluate a JavaScript expression in the page context",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EvaluateInput) (*mcp.CallToolResult, any, error) {
		var result any
		var err error
		if input.AwaitPromise {
			err = chromedp.Run(s.activeCtx(), chromedp.EvaluateAsDevTools(input.Expression, &result))
		} else {
			err = chromedp.Run(s.activeCtx(), chromedp.Evaluate(input.Expression, &result))
		}
		if err != nil {
			return nil, nil, fmt.Errorf("evaluate: %w", err)
		}
		data, err := json.Marshal(result)
		if err != nil {
			return nil, nil, fmt.Errorf("evaluate: marshal result: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(data)},
			},
		}, nil, nil
	})
}

// --- Tab management tools ---

type TabInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

type ListTabsOutput struct {
	Tabs []TabInfo `json:"tabs"`
}

type SwitchTabInput struct {
	ID string `json:"id"`
}

type NewTabInput struct {
	URL string `json:"url,omitempty"`
}

type TabOutput struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

func registerTabTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tabs",
		Description: "List all open browser tabs",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, ListTabsOutput, error) {
		targets, err := chromedp.Targets(s.browserCtx)
		if err != nil {
			return nil, ListTabsOutput{}, fmt.Errorf("list_tabs: %w", err)
		}
		var tabs []TabInfo
		for _, t := range targets {
			if t.Type == "page" {
				tabs = append(tabs, TabInfo{
					ID:    string(t.TargetID),
					Title: t.Title,
					URL:   t.URL,
				})
			}
		}
		return nil, ListTabsOutput{Tabs: tabs}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "switch_tab",
		Description: "Switch to a browser tab by target ID",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SwitchTabInput) (*mcp.CallToolResult, TabOutput, error) {
		tabCtx, tabCancel := chromedp.NewContext(s.browserCtx, chromedp.WithTargetID(target.ID(input.ID)))
		// Run a no-op to ensure the context attaches to the target.
		if err := chromedp.Run(tabCtx); err != nil {
			tabCancel()
			return nil, TabOutput{}, fmt.Errorf("switch_tab: %w", err)
		}
		s.setActiveCtx(tabCtx, tabCancel)
		var out TabOutput
		out.ID = input.ID
		_ = chromedp.Run(tabCtx, chromedp.Title(&out.Title), chromedp.Location(&out.URL))
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "new_tab",
		Description: "Open a new browser tab, optionally navigating to a URL",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input NewTabInput) (*mcp.CallToolResult, TabOutput, error) {
		tabCtx, tabCancel := chromedp.NewContext(s.browserCtx)
		if input.URL != "" {
			if err := chromedp.Run(tabCtx, chromedp.Navigate(input.URL)); err != nil {
				tabCancel()
				return nil, TabOutput{}, fmt.Errorf("new_tab: %w", err)
			}
		} else {
			if err := chromedp.Run(tabCtx); err != nil {
				tabCancel()
				return nil, TabOutput{}, fmt.Errorf("new_tab: %w", err)
			}
		}
		s.setActiveCtx(tabCtx, tabCancel)
		var out TabOutput
		_ = chromedp.Run(tabCtx, chromedp.Title(&out.Title), chromedp.Location(&out.URL))
		return nil, out, nil
	})
}

// --- Context tools ---

type PushContextInput struct {
	Name string `json:"name"`
}

type ContextOutput struct {
	Path string `json:"path"`
}

func registerContextTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "push_context",
		Description: "Push a new recording context to isolate traffic",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PushContextInput) (*mcp.CallToolResult, ContextOutput, error) {
		path := s.pushContext(input.Name)
		return nil, ContextOutput{Path: path}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pop_context",
		Description: "Pop the current recording context",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, ContextOutput, error) {
		path, err := s.popContext()
		if err != nil {
			return nil, ContextOutput{}, fmt.Errorf("pop_context: %w", err)
		}
		return nil, ContextOutput{Path: path}, nil
	})
}

// --- HAR/Network tools ---

type GetHAREntriesInput struct {
	Domain string `json:"domain,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

func registerHARTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_har_entries",
		Description: "Get captured HAR network entries, optionally filtered by domain",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetHAREntriesInput) (*mcp.CallToolResult, any, error) {
		if s.recorder == nil {
			return nil, nil, fmt.Errorf("get_har_entries: no recorder active")
		}
		h, err := s.recorder.HAR()
		if err != nil {
			return nil, nil, fmt.Errorf("get_har_entries: %w", err)
		}
		entries := h.Log.Entries
		if input.Domain != "" {
			var filtered []*har.Entry
			for _, e := range entries {
				u, err := url.Parse(e.Request.URL)
				if err != nil {
					continue
				}
				if strings.Contains(u.Host, input.Domain) {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
		}
		if input.Limit > 0 && len(entries) > input.Limit {
			entries = entries[len(entries)-input.Limit:]
		}
		data, err := json.Marshal(entries)
		if err != nil {
			return nil, nil, fmt.Errorf("get_har_entries: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(data)},
			},
		}, nil, nil
	})
}

// --- Cookie tools ---

type GetCookiesInput struct {
	Domain string `json:"domain,omitempty"`
}

type SetCookieInput struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path,omitempty"`
}

func registerCookieTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_cookies",
		Description: "Get browser cookies, optionally filtered by domain",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetCookiesInput) (*mcp.CallToolResult, any, error) {
		cookies, err := network.GetCookies().Do(s.activeCtx())
		if err != nil {
			return nil, nil, fmt.Errorf("get_cookies: %w", err)
		}
		if input.Domain != "" {
			var filtered []*network.Cookie
			for _, c := range cookies {
				if strings.Contains(c.Domain, input.Domain) {
					filtered = append(filtered, c)
				}
			}
			cookies = filtered
		}
		data, err := json.Marshal(cookies)
		if err != nil {
			return nil, nil, fmt.Errorf("get_cookies: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(data)},
			},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_cookie",
		Description: "Set a browser cookie",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetCookieInput) (*mcp.CallToolResult, any, error) {
		cmd := network.SetCookie(input.Name, input.Value).WithDomain(input.Domain)
		if input.Path != "" {
			cmd = cmd.WithPath(input.Path)
		}
		if err := cmd.Do(s.activeCtx()); err != nil {
			return nil, nil, fmt.Errorf("set_cookie: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("cookie %s set for %s", input.Name, input.Domain)},
			},
		}, nil, nil
	})
}

// --- Source capture tools ---

type SaveSourcesOutput struct {
	ScriptsCount int    `json:"scripts_count"`
	StylesCount  int    `json:"styles_count"`
	OutputDir    string `json:"output_dir"`
}

func registerSourceTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "save_sources",
		Description: "Capture all JS/CSS sources (including sourcemapped originals) from the current page and write to disk",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, SaveSourcesOutput, error) {
		if s.sourceCollector == nil {
			return nil, SaveSourcesOutput{}, fmt.Errorf("save_sources: source capture not enabled (use --save-sources flag)")
		}
		if err := s.sourceCollector.CaptureAll(s.activeCtx()); err != nil {
			return nil, SaveSourcesOutput{}, fmt.Errorf("save_sources: capture: %w", err)
		}
		if err := s.sourceCollector.WriteToDisk(); err != nil {
			return nil, SaveSourcesOutput{}, fmt.Errorf("save_sources: write: %w", err)
		}
		scripts := s.sourceCollector.Scripts()
		styles := s.sourceCollector.Styles()
		return nil, SaveSourcesOutput{
			ScriptsCount: len(scripts),
			StylesCount:  len(styles),
			OutputDir:    s.sourceCollector.OutputDir(),
		}, nil
	})
}
