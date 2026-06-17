package cdpscript

import (
	"context"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"rsc.io/script"
)

type dialogAction struct {
	accept     bool
	promptText string
}

func (e *Engine) cmdDialog() script.Cmd {
	return simpleCmd("handle next JavaScript dialog", "accept|dismiss [prompt-text]", func(s *script.State, args []string) error {
		action, err := parseDialogArgs(args)
		if err != nil {
			return err
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		if err := e.armDialog(action); err != nil {
			return err
		}
		if e.verbose {
			name := "dismiss"
			if action.accept {
				name = "accept"
			}
			fmt.Fprintf(e.stderr, "[dialog] armed %s\n", name)
		}
		return nil
	})
}

func parseDialogArgs(args []string) (*dialogAction, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("dialog requires accept or dismiss")
	}
	switch args[0] {
	case "accept":
		return &dialogAction{
			accept:     true,
			promptText: strings.Join(args[1:], " "),
		}, nil
	case "dismiss":
		if len(args) != 1 {
			return nil, fmt.Errorf("dialog dismiss does not accept prompt text")
		}
		return &dialogAction{}, nil
	default:
		return nil, fmt.Errorf("unknown dialog action %q", args[0])
	}
}

func (e *Engine) armDialog(action *dialogAction) error {
	if e.browser == nil || e.browser.Context() == nil {
		return fmt.Errorf("browser not initialized")
	}
	ctx := e.browser.Context()

	e.dialogMu.Lock()
	e.dialogAction = action
	needListen := !e.dialogListening
	e.dialogMu.Unlock()

	if needListen {
		if err := chromedp.Run(ctx, page.Enable()); err != nil {
			return fmt.Errorf("enable dialog events: %w", err)
		}
		chromedp.ListenTarget(ctx, func(ev any) {
			e.handleDialogEvent(ctx, ev)
		})
		e.dialogMu.Lock()
		e.dialogListening = true
		e.dialogMu.Unlock()
	}
	return nil
}

func (e *Engine) handleDialogEvent(ctx context.Context, ev any) {
	if _, ok := ev.(*page.EventJavascriptDialogOpening); !ok {
		return
	}

	e.dialogMu.Lock()
	action := e.dialogAction
	e.dialogAction = nil
	e.dialogMu.Unlock()
	if action == nil {
		return
	}

	go func() {
		cmd := page.HandleJavaScriptDialog(action.accept)
		if action.promptText != "" {
			cmd = cmd.WithPromptText(action.promptText)
		}
		if err := chromedp.Run(ctx, cmd); err != nil && e.verbose {
			fmt.Fprintf(e.stderr, "[dialog] handle: %v\n", err)
		}
	}()
}
