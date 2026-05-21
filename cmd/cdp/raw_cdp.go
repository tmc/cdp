package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	cdproto "github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

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
	switch method {
	case "Browser.close", "Target.closeTarget":
		return "", fmt.Errorf("%s is not allowed", method)
	}
	return method, nil
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

	var result map[string]any
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		execCtx := ctx
		if target == "browser" {
			c := chromedp.FromContext(ctx)
			if c == nil || c.Browser == nil {
				return fmt.Errorf("browser executor unavailable")
			}
			execCtx = cdproto.WithExecutor(ctx, c.Browser)
		}
		return cdproto.Execute(execCtx, method, params, &result)
	})); err != nil {
		return nil, err
	}
	if result == nil {
		result = map[string]any{}
	}
	return result, nil
}
