package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"google.golang.org/genai"
)

// BrowserAgent orchestrates the AI-computer interaction loop.
type BrowserAgent struct {
	computer Computer
	client   *genai.Client
	query    string
	history  []*genai.Content
	maxTurns int
	verbose  bool
	model    string
	config   *genai.GenerateContentConfig
}

// NewBrowserAgent creates a new agent with any Computer backend.
// If vmMode is true, configures for VM/desktop control instead of browser automation.
func NewBrowserAgent(ctx context.Context, computer Computer, apiKey, modelName, query string, verbose bool, vmMode bool) (*BrowserAgent, error) {
	// Create client
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("creating genai client: %w", err)
	}

	// System instruction depends on mode
	var systemPrompt string
	var excludedFunctions []string

	if vmMode {
		systemPrompt = `You are a desktop automation assistant controlling a macOS virtual machine. You can see screenshots of the VM screen and control it using keyboard and mouse.

Your task is to help users accomplish their goals by clicking UI elements, typing text, and using keyboard shortcuts.

When given a task:
1. Analyze the current screenshot to understand what's visible on the desktop
2. Use click_at() to click on buttons, menu items, and other UI elements
3. Use type_text_at() to enter text in text fields
4. Use key_combination() for keyboard shortcuts (e.g., ["Cmd", "C"] for copy)
5. Use scroll_at() or scroll_document() when needed
6. Continue until the task is complete

Coordinates are on a 0-1000 scale where (0,0) is top-left and (1000,1000) is bottom-right.

Always use the tools to accomplish tasks. Do not just describe what you see - actually perform the actions needed.
Do NOT use browser-specific functions like navigate, go_back, go_forward, or search - they don't apply to desktop control.`

		// Exclude browser-specific functions for VM mode
		excludedFunctions = []string{"open_web_browser", "navigate", "go_back", "go_forward", "search"}
	} else {
		systemPrompt = `You are a browser automation assistant. You can see screenshots of the current web page and control the browser using the available tools.

Your task is to help users accomplish their goals by navigating websites, clicking elements, typing text, and scrolling.

When given a task:
1. Analyze the current screenshot to understand what's visible
2. Use the available tools to interact with the page
3. Navigate to URLs using the navigate() function
4. Click on elements using click_at() with coordinates
5. Type text using type_text_at() with coordinates
6. Scroll using scroll_document() when needed
7. Continue until the task is complete

Coordinates are on a 0-1000 scale where (0,0) is top-left and (1000,1000) is bottom-right.

Always use the tools to accomplish tasks. Do not just describe what you see - actually perform the actions needed.`

		excludedFunctions = []string{} // Match Python SDK: explicitly set to empty array
	}

	// Configure generation parameters
	config := &genai.GenerateContentConfig{
		Temperature:     genai.Ptr[float32](1.0),
		TopP:            genai.Ptr[float32](0.95),
		TopK:            genai.Ptr[float32](40),
		MaxOutputTokens: 8192,
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: systemPrompt},
			},
		},
		Tools: []*genai.Tool{
			{
				// Use ComputerUse tool like Google's Python implementation
				ComputerUse: &genai.ComputerUse{
					Environment:                 genai.EnvironmentBrowser,
					ExcludedPredefinedFunctions: excludedFunctions,
				},
			},
		},
	}

	return &BrowserAgent{
		computer: computer,
		client:   client,
		query:    query,
		history:  make([]*genai.Content, 0),
		maxTurns: 50,
		verbose:  verbose,
		model:    modelName,
		config:   config,
	}, nil
}

// Close closes the agent's Gemini client.
func (a *BrowserAgent) Close() error {
	// No Close method on new SDK client
	return nil
}

// Run executes the agent loop.
func (a *BrowserAgent) Run(ctx context.Context) error {
	// Get initial screenshot
	state, err := a.computer.CurrentState()
	if err != nil {
		return fmt.Errorf("getting initial state: %w", err)
	}

	// Add initial user query with screenshot
	a.addUserMessageWithScreenshot(a.query, state.Screenshot)

	if a.verbose {
		log.Printf("Starting agent loop with query: %s\n", a.query)
	}

	return a.executeLoop(ctx)
}

// ProcessQuery processes a single user query in shell mode (doesn't close client).
func (a *BrowserAgent) ProcessQuery(ctx context.Context, query string) error {
	// Get current screenshot
	state, err := a.computer.CurrentState()
	if err != nil {
		return fmt.Errorf("getting current state: %w", err)
	}

	// Add user query with screenshot to conversation history
	a.addUserMessageWithScreenshot(query, state.Screenshot)

	if a.verbose {
		log.Printf("Processing query: %s\n", query)
	}

	return a.executeLoop(ctx)
}

// executeLoop runs the agent execution loop
func (a *BrowserAgent) executeLoop(ctx context.Context) error {
	for turn := 0; turn < a.maxTurns; turn++ {
		if a.verbose {
			log.Printf("\n=== Turn %d/%d ===\n", turn+1, a.maxTurns)
		}

		// Get model response
		resp, err := a.getModelResponse(ctx)
		if err != nil {
			return fmt.Errorf("getting model response: %w", err)
		}

		if a.verbose {
			// Print raw response in grey
			fmt.Fprintf(os.Stderr, "\033[90m")  // Grey color
			if resp == nil {
				log.Println("Response is nil")
			} else if len(resp.Candidates) == 0 {
				log.Println("Response has no candidates")
				if resp.PromptFeedback != nil {
					log.Printf("Prompt feedback: %+v\n", resp.PromptFeedback)
					log.Printf("Raw response: %+v\n", resp)
				}
			} else if resp.Candidates[0].Content == nil {
				log.Println("Response candidate has nil content")
				log.Printf("Candidate finish reason: %v\n", resp.Candidates[0].FinishReason)
				if len(resp.Candidates[0].SafetyRatings) > 0 {
					log.Printf("Safety ratings: %+v\n", resp.Candidates[0].SafetyRatings)
				}
				log.Printf("Full candidate: %+v\n", resp.Candidates[0])
			} else {
				log.Printf("Response has %d parts\n", len(resp.Candidates[0].Content.Parts))
				if len(resp.Candidates[0].Content.Parts) > 0 {
					log.Printf("First part type: %T\n", resp.Candidates[0].Content.Parts[0])
				}
				// Print the actual response content
				for i, part := range resp.Candidates[0].Content.Parts {
					if part.Text != "" {
						log.Printf("Part %d [Text]: %s\n", i, part.Text)
					} else if part.FunctionCall != nil {
						log.Printf("Part %d [FunctionCall]: %s(%+v)\n", i, part.FunctionCall.Name, part.FunctionCall.Args)
					} else {
						log.Printf("Part %d [%T]: %+v\n", i, part, part)
					}
				}
			}
			fmt.Fprintf(os.Stderr, "\033[0m")  // Reset color
		}

		// Extract reasoning text and function calls
		calls := a.extractFunctionCalls(resp)
		var reasoning string
		if resp != nil && len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
			for _, part := range resp.Candidates[0].Content.Parts {
				if part.Text != "" {
					reasoning = part.Text
					break
				}
			}
		}

		// Check if we're done (no function calls)
		if len(calls) == 0 {
			if a.verbose {
				log.Println("No more function calls - task complete")
			}
			// Print final response
			if reasoning != "" {
				fmt.Printf("\nAgent: %s\n", reasoning)
			} else if a.verbose {
				log.Println("Warning: Cannot print final response - response structure is incomplete")
			}
			return nil
		}

		// Print reasoning and function calls in a formatted table
		if !a.verbose {
			// In non-verbose mode, show a simplified view
			if reasoning != "" {
				fmt.Printf("\n💭 Reasoning: %s\n", reasoning)
			}
			for _, call := range calls {
				fmt.Printf("🔧 Action: %s", call.Name)
				if call.Args != nil && len(call.Args) > 0 {
					fmt.Printf("(")
					first := true
					for k, v := range call.Args {
						if !first {
							fmt.Printf(", ")
						}
						fmt.Printf("%s=%v", k, v)
						first = false
					}
					fmt.Printf(")")
				}
				fmt.Println()
			}
		}

		// Add model response to history FIRST (before function responses)
		// This is critical: the conversation must be user → model → user (function response)
		if resp != nil && len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
			a.history = append(a.history, resp.Candidates[0].Content)
		}

		// Execute ALL function calls and collect responses
		// CRITICAL: All function responses must be in ONE user turn to maintain
		// user → model → user alternation (not user → model → user → user)
		var allParts []*genai.Part
		for _, call := range calls {
			if a.verbose {
				log.Printf("Executing: %s\n", call.Name)
			}

			state, err := a.handleAction(call)
			if err != nil {
				return fmt.Errorf("handling action %s: %w", call.Name, err)
			}

			// Build function response part with screenshot embedded
			var functionResponseParts []*genai.FunctionResponsePart
			if len(state.Screenshot) > 0 {
				functionResponseParts = append(functionResponseParts, &genai.FunctionResponsePart{
					InlineData: &genai.FunctionResponseBlob{
						MIMEType: "image/png",
						Data:     state.Screenshot,
					},
				})
			}

			// Add this function response to the collection
			allParts = append(allParts, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: call.Name,
					Response: map[string]interface{}{
						"url": state.URL,
					},
					Parts: functionResponseParts, // Screenshot goes INSIDE FunctionResponse.Parts
				},
			})
		}

		// Add ALL function responses as a SINGLE user turn
		if len(allParts) > 0 {
			a.history = append(a.history, &genai.Content{
				Role:  "user", // Function responses use "user" role
				Parts: allParts,
			})
		}

		// Trim old screenshots to manage context
		a.trimOldScreenshots(3)
	}

	return fmt.Errorf("max turns reached without completion")
}

// getModelResponse calls the Gemini API with retry logic.
func (a *BrowserAgent) getModelResponse(ctx context.Context) (*genai.GenerateContentResponse, error) {
	maxRetries := 5
	initialWait := 2 * time.Second

	var lastErr error
	wait := initialWait

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			if a.verbose {
				log.Printf("Retrying API call (attempt %d/%d) after %v...\n", attempt, maxRetries, wait)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}

		// Use new SDK API - GenerateContent with history
		contents := a.history

		// Debug: print request structure if verbose
		if a.verbose && attempt == 0 {
			log.Printf("DEBUG: Sending %d content items in history\n", len(contents))
			for i, content := range contents {
				log.Printf("  Content %d: role=%s, parts=%d\n", i, content.Role, len(content.Parts))
				for j, part := range content.Parts {
					if part.Text != "" {
						log.Printf("    Part %d: text (%d chars)\n", j, len(part.Text))
					} else if part.InlineData != nil {
						log.Printf("    Part %d: inlineData (%d bytes)\n", j, len(part.InlineData.Data))
					} else if part.FunctionCall != nil {
						log.Printf("    Part %d: functionCall %s\n", j, part.FunctionCall.Name)
					} else if part.FunctionResponse != nil {
						log.Printf("    Part %d: functionResponse %s (parts=%d)\n", j, part.FunctionResponse.Name, len(part.FunctionResponse.Parts))
						for k, frp := range part.FunctionResponse.Parts {
							if frp.InlineData != nil {
								log.Printf("      FunctionResponse.Part %d: inlineData (%d bytes)\n", k, len(frp.InlineData.Data))
							}
						}
					}
				}
			}
		}

		resp, err := a.client.Models.GenerateContent(ctx, a.model, contents, a.config)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		// Always log API errors with full details
		log.Printf("API error (attempt %d): %v", attempt+1, err)
		if a.verbose {
			log.Printf("Full error details: %+v", err)
		}
		wait = time.Duration(float64(wait) * 2)
		if wait > 32*time.Second {
			wait = 32 * time.Second
		}
	}

	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// extractFunctionCalls extracts function calls from the model response.
func (a *BrowserAgent) extractFunctionCalls(resp *genai.GenerateContentResponse) []*FunctionCall {
	if resp == nil || len(resp.Candidates) == 0 {
		return nil
	}

	if resp.Candidates[0].Content == nil {
		return nil
	}

	var calls []*FunctionCall
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.FunctionCall != nil {
			calls = append(calls, &FunctionCall{
				Name: part.FunctionCall.Name,
				Args: part.FunctionCall.Args,
			})
		}
	}

	return calls
}

// handleAction executes a browser action.
func (a *BrowserAgent) handleAction(call *FunctionCall) (*EnvState, error) {
	switch call.Name {
	case "open_web_browser":
		// Like Python implementation, just return current state - browser is already open
		// at the URL specified via --url flag at startup
		return a.computer.CurrentState()

	case "click_at":
		x := getInt(call.Args, "x")
		y := getInt(call.Args, "y")
		return a.computer.ClickAt(x, y)

	case "hover_at":
		x := getInt(call.Args, "x")
		y := getInt(call.Args, "y")
		return a.computer.HoverAt(x, y)

	case "type_text_at":
		x := getInt(call.Args, "x")
		y := getInt(call.Args, "y")
		text := getString(call.Args, "text", "")
		pressEnter := getBool(call.Args, "press_enter")
		clearBefore := getBool(call.Args, "clear_before_typing")
		return a.computer.TypeTextAt(x, y, text, pressEnter, clearBefore)

	case "drag_and_drop":
		x := getInt(call.Args, "x")
		y := getInt(call.Args, "y")
		destX := getInt(call.Args, "destination_x")
		destY := getInt(call.Args, "destination_y")
		return a.computer.DragAndDrop(x, y, destX, destY)

	case "navigate":
		url := getString(call.Args, "url", "")
		return a.computer.Navigate(url)

	case "go_back":
		return a.computer.GoBack()

	case "go_forward":
		return a.computer.GoForward()

	case "search":
		return a.computer.Search()

	case "scroll_document":
		direction := getString(call.Args, "direction", "down")
		return a.computer.ScrollDocument(direction)

	case "scroll_at":
		x := getInt(call.Args, "x")
		y := getInt(call.Args, "y")
		direction := getString(call.Args, "direction", "down")
		magnitude := getFloat(call.Args, "magnitude", 100)
		return a.computer.ScrollAt(x, y, direction, magnitude)

	case "key_combination":
		keys := getStringSlice(call.Args, "keys")
		return a.computer.KeyCombination(keys)

	case "wait_5_seconds":
		return a.computer.Wait5Seconds()

	default:
		return nil, fmt.Errorf("unknown function: %s", call.Name)
	}
}

// addUserMessage adds a user message to the conversation.
func (a *BrowserAgent) addUserMessage(text string) {
	a.history = append(a.history, &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{Text: text},
		},
	})
}

// addUserMessageWithScreenshot adds a user message with a screenshot.
func (a *BrowserAgent) addUserMessageWithScreenshot(text string, screenshot []byte) {
	parts := []*genai.Part{
		{Text: text},
	}

	if len(screenshot) > 0 {
		parts = append(parts, &genai.Part{
			InlineData: &genai.Blob{
				MIMEType: "image/png",
				Data:     screenshot,
			},
		})
	}

	a.history = append(a.history, &genai.Content{
		Role:  "user",
		Parts: parts,
	})
}

// trimOldScreenshots removes screenshots from old turns to save tokens.
func (a *BrowserAgent) trimOldScreenshots(maxTurns int) {
	if len(a.history) <= maxTurns*2 {
		return
	}

	keepFromIndex := len(a.history) - (maxTurns * 2)
	if keepFromIndex < 1 {
		keepFromIndex = 1
	}

	// Remove blobs from old messages
	for i := 1; i < keepFromIndex; i++ {
		newParts := make([]*genai.Part, 0)
		for _, part := range a.history[i].Parts {
			if part.InlineData == nil {
				newParts = append(newParts, part)
			}
		}
		a.history[i].Parts = newParts
	}
}

// FunctionCall represents a function call from the AI.
type FunctionCall struct {
	Name string
	Args map[string]interface{}
}

// Helper functions to extract typed values from Args map
func getString(m map[string]interface{}, key, defaultVal string) string {
	if m == nil {
		return defaultVal
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultVal
}

func getInt(m map[string]interface{}, key string) int {
	if m == nil {
		return 0
	}
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case int64:
			return int(val)
		case float64:
			return int(math.Round(val))
		case json.Number:
			if i, err := val.Int64(); err == nil {
				return int(i)
			}
		}
	}
	return 0
}

func getFloat(m map[string]interface{}, key string, defaultVal float64) float64 {
	if m == nil {
		return defaultVal
	}
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case int:
			return float64(val)
		case int64:
			return float64(val)
		}
	}
	return defaultVal
}

func getBool(m map[string]interface{}, key string) bool {
	if m == nil {
		return false
	}
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func getStringSlice(m map[string]interface{}, key string) []string {
	if m == nil {
		return nil
	}
	if v, ok := m[key]; ok {
		if slice, ok := v.([]interface{}); ok {
			result := make([]string, 0, len(slice))
			for _, item := range slice {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
		if s, ok := v.(string); ok {
			// Handle string with + separator (like "Cmd+C")
			return []string{s}
		}
	}
	return nil
}
