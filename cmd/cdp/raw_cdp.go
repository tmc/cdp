package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"unicode"

	cdproto "github.com/chromedp/cdproto/cdp"
	"github.com/gorilla/websocket"
	"github.com/tmc/cdp/internal/chromedp"
)

type rawCDPResult map[string]any

func (r *rawCDPResult) UnmarshalJSON(data []byte) error {
	if len(bytes.TrimSpace(data)) == 0 {
		*r = rawCDPResult{}
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	if m == nil {
		m = map[string]any{}
	}
	*r = rawCDPResult(m)
	return nil
}

func isEmptyRawCDPResultError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "unexpected EOF") || strings.Contains(msg, "unexpected end of JSON input")
}

func parseRawCDPCommand(command string) (string, map[string]any, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", nil, fmt.Errorf("empty command")
	}

	method := command
	paramText := "{}"
	if i := strings.IndexFunc(command, unicode.IsSpace); i >= 0 {
		method = command[:i]
		paramText = strings.TrimSpace(command[i:])
		if paramText == "" {
			paramText = "{}"
		}
	}

	method, err := validateRawCDPMethod(method)
	if err != nil {
		return "", nil, err
	}

	var params map[string]any
	if paramText == "{}" {
		return method, map[string]any{}, nil
	}
	if err := json.Unmarshal([]byte(paramText), &params); err != nil {
		return "", nil, fmt.Errorf("invalid JSON parameters: %w", err)
	}
	if params == nil {
		params = map[string]any{}
	}
	return method, params, nil
}

func validateRawCDPMethod(method string) (string, error) {
	method = strings.TrimSpace(method)
	if method == "" {
		return "", fmt.Errorf("method is required")
	}
	if strings.ContainsAny(method, " \t\r\n") || strings.Count(method, ".") != 1 {
		return "", fmt.Errorf("invalid method %q", method)
	}
	if rawCDPDeniedMethods[method] {
		return "", fmt.Errorf("%s is not allowed; use the new_tab and close_tab tools", method)
	}
	return method, nil
}

// rawCDPDeniedMethods are the CDP methods raw_cdp refuses to issue. Tab and
// browser lifecycle belongs to the session, which keeps at most one MCP-owned
// tab and one browser and cancels the previous one when it is replaced. A raw
// call that creates a target or a browser context produces one the session
// does not track and never closes; a raw call that closes one destroys what
// the other tools still point at. Both directions are refused, so lifecycle
// runs through new_tab and close_tab, which are tracked.
var rawCDPDeniedMethods = map[string]bool{
	"Browser.close":               true,
	"Target.closeTarget":          true,
	"Target.createTarget":         true,
	"Target.createBrowserContext": true,
}

func isRawCDPCommandName(name string) bool {
	_, err := validateRawCDPMethod(name)
	return err == nil
}

func validateRawCDPTarget(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		target = "target"
	}
	switch target {
	case "target", "browser":
		return target, nil
	default:
		return "", fmt.Errorf("invalid target %q", target)
	}
}

func runRawCDP(ctx context.Context, method string, params map[string]any, target string) (map[string]any, error) {
	method, err := validateRawCDPMethod(method)
	if err != nil {
		return nil, err
	}
	target, err = validateRawCDPTarget(target)
	if err != nil {
		return nil, err
	}
	if params == nil {
		params = map[string]any{}
	}

	var result rawCDPResult
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		execCtx := ctx
		if target == "browser" {
			c := chromedp.FromContext(ctx)
			if c == nil || c.Browser == nil {
				return fmt.Errorf("browser executor unavailable")
			}
			execCtx = cdproto.WithExecutor(ctx, c.Browser)
		}
		if err := cdproto.Execute(execCtx, method, params, &result); err != nil {
			if isEmptyRawCDPResultError(err) {
				result = rawCDPResult{}
				return nil
			}
			return err
		}
		return nil
	})); err != nil {
		return nil, err
	}
	if result == nil {
		result = rawCDPResult{}
	}
	return map[string]any(result), nil
}

var rawCDPWebSocketID int64

type rawCDPWebSocketResponse struct {
	ID     int64          `json:"id"`
	Result rawCDPResult   `json:"result"`
	Error  map[string]any `json:"error"`
}

func runRawCDPWebSocket(ctx context.Context, wsURL, method string, params map[string]any) (map[string]any, error) {
	method, err := validateRawCDPMethod(method)
	if err != nil {
		return nil, err
	}
	if params == nil {
		params = map[string]any{}
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("dial target websocket: %w", err)
	}
	defer conn.Close()

	id := atomic.AddInt64(&rawCDPWebSocketID, 1)
	req := map[string]any{
		"id":     id,
		"method": method,
		"params": params,
	}
	if err := conn.WriteJSON(req); err != nil {
		return nil, fmt.Errorf("write raw CDP command: %w", err)
	}

	for {
		var resp rawCDPWebSocketResponse
		if err := conn.ReadJSON(&resp); err != nil {
			if isEmptyRawCDPResultError(err) {
				return map[string]any{}, nil
			}
			return nil, fmt.Errorf("read raw CDP response: %w", err)
		}
		if resp.ID == 0 || resp.ID != id {
			continue
		}
		if resp.Error != nil {
			data, err := json.Marshal(resp.Error)
			if err != nil {
				return nil, fmt.Errorf("raw CDP error: %v", resp.Error)
			}
			return nil, fmt.Errorf("raw CDP error: %s", data)
		}
		if resp.Result == nil {
			resp.Result = rawCDPResult{}
		}
		return map[string]any(resp.Result), nil
	}
}
