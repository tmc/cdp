package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/target"
	"github.com/tmc/cdp/internal/browser"
	"github.com/tmc/cdp/internal/chromedp"
	"github.com/tmc/cdp/internal/testutil"
)

type targetEventsKey struct{}

func targetLifecycleBrowser(t *testing.T) (context.Context, string, string, []target.ID) {
	t.Helper()
	path := testutil.FindChrome()
	if path == "" {
		t.Skip("no Chromium browser available")
	}
	profile := t.TempDir()
	options := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(path), chromedp.UserDataDir(profile))
	alloc, stop := chromedp.NewExecAllocator(context.Background(), options...)
	t.Cleanup(stop)
	ctx, stop := chromedp.NewContext(alloc)
	t.Cleanup(stop)
	ctx, stop = context.WithTimeout(ctx, 30*time.Second)
	t.Cleanup(stop)
	if err := chromedp.Run(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := chromedp.Cancel(ctx); err != nil {
			t.Errorf("close fixture browser: %v", err)
		}
	})
	raw, err := os.ReadFile(profile + "/DevToolsActivePort")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(parts) != 2 {
		t.Fatal("invalid DevToolsActivePort")
	}
	host := "ws://127.0.0.1:" + parts[0]
	var ids []target.ID
	for i := 0; i < 2; i++ {
		var id target.ID
		err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
			var err error
			id, err = target.CreateTarget("about:blank").Do(cdp.WithExecutor(c, chromedp.FromContext(c).Browser))
			return err
		}))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	events := make(chan target.ID, 16)
	chromedp.ListenBrowser(ctx, func(event any) {
		if e, ok := event.(*target.EventTargetDestroyed); ok {
			select {
			case events <- e.TargetID:
			default:
			}
		}
	})
	ctx = context.WithValue(ctx, targetEventsKey{}, events)
	return ctx, host + parts[1], host, ids
}

func requireTargets(t *testing.T, ctx context.Context, ids []target.ID, want bool) {
	t.Helper()
	if want {
		events, _ := ctx.Value(targetEventsKey{}).(chan target.ID)
		timer := time.NewTimer(300 * time.Millisecond)
		defer timer.Stop()
	wait:
		for {
			select {
			case id := <-events:
				for _, expected := range ids {
					if id == expected {
						t.Errorf("borrowed target destroyed: %s", id)
					}
				}
			case <-timer.C:
				break wait
			}
		}
	}
	targets, err := chromedp.Targets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		found := false
		for _, info := range targets {
			if info.TargetID == id {
				found = true
			}
		}
		if found != want {
			t.Errorf("target %s present=%v want=%v", id, found, want)
		}
	}
}

func TestProductionRemoteDisconnect(t *testing.T) {
	for _, method := range []string{"existing tab", "tab websocket"} {
		t.Run(method, func(t *testing.T) {
			controller, ws, host, ids := targetLifecycleBrowser(t)
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			b, err := browser.New(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if method == "existing tab" {
				err = b.ConnectToExistingTab(ctx, ws, string(ids[0]))
			} else {
				err = b.ConnectToTabWebSocket(ctx, host+"/devtools/page/"+string(ids[0]))
			}
			if err != nil {
				t.Fatal(err)
			}
			var value int
			if err := chromedp.Run(b.Context(), chromedp.Evaluate("1", &value)); err != nil || value != 1 {
				t.Fatalf("attached evaluation=%d err=%v", value, err)
			}
			connection := chromedp.FromContext(b.Context()).Browser
			if err := b.Close(); err != nil {
				t.Fatal(err)
			}
			if b.Context().Err() != context.Canceled {
				t.Fatal("Close did not cancel the attached context")
			}
			select {
			case <-connection.LostConnection:
			case <-time.After(2 * time.Second):
				t.Fatal("Close did not release the remote connection")
			}
			requireTargets(t, controller, ids, true)
		})
	}
}

func TestProductionExplicitPageClose(t *testing.T) {
	controller, ws, _, ids := targetLifecycleBrowser(t)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	b, err := browser.New(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.ConnectToExistingTab(ctx, ws, string(ids[0])); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	if err := chromedp.Run(b.Context()); err != nil {
		t.Fatal(err)
	}
	p, err := b.AttachToTarget(string(ids[1]))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		list, err := chromedp.Targets(controller)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, v := range list {
			if v.TargetID == ids[1] {
				found = true
			}
		}
		if !found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("explicit Page.Close left target alive")
		}
		time.Sleep(10 * time.Millisecond)
	}
	requireTargets(t, controller, ids[:1], true)
}

func TestProductionMonitorDetach(t *testing.T) {
	controller, _, _, ids := targetLifecycleBrowser(t)
	monitor := newAllTabsMonitor(controller, false, func(ctx context.Context) error { return chromedp.Run(ctx, chromedp.Evaluate("1", nil)) })
	if err := monitor.Start(); err != nil {
		t.Fatal(err)
	}
	monitor.Stop()
	if err := monitor.attachToTarget(ids[0]); err != context.Canceled {
		t.Fatalf("attach after Stop: %v", err)
	}
	requireTargets(t, controller, ids, true)
	if len(monitor.attachedTargets) != 0 {
		t.Fatalf("monitor retained %d contexts", len(monitor.attachedTargets))
	}
}

func TestMonitorStopWaitsForAttach(t *testing.T) {
	entered := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{}, 1)
	defer close(release)
	monitor := newAllTabsMonitor(context.Background(), false, func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-release
		return ctx.Err()
	})
	attached := make(chan error, 1)
	go func() { attached <- monitor.attachToTarget("fixture") }()
	<-entered
	stopped := make(chan struct{})
	go func() { monitor.Stop(); close(stopped) }()
	<-canceled
	select {
	case <-stopped:
		t.Fatal("Stop returned before pending attach finished")
	default:
	}
	// A buffered release avoids closing the channel twice on a failing assertion.
	release <- struct{}{}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not wait for canceled attach")
	}
	if err := <-attached; err == nil {
		t.Fatal("canceled attach succeeded")
	}
	if err := monitor.attachToTarget("later"); err != context.Canceled {
		t.Fatalf("attach after Stop=%v", err)
	}
}
