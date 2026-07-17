package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestActiveCtxWaitsForBrowserReady verifies that activeCtx blocks until browser
// setup signals completion, then returns the populated context. A tool call that
// raced ahead of setup used to read a nil context and panic in
// context.WithTimeout(nil, ...), which the MCP SDK recovered without replying —
// the client then hung until its own timeout.
func TestActiveCtxWaitsForBrowserReady(t *testing.T) {
	s := &mcpSession{browserReady: make(chan struct{})}

	// Populate the context shortly after the call begins, mimicking the
	// background setup goroutine winning the race late.
	want := context.Background()
	go func() {
		time.Sleep(20 * time.Millisecond)
		s.mu.Lock()
		s.ctx = want
		s.mu.Unlock()
		close(s.browserReady)
	}()

	got := s.activeCtx()
	if got != want {
		t.Fatalf("activeCtx() = %v, want the populated context", got)
	}
}

// TestActiveCtxNeverNil verifies that when setup finishes without populating a
// context (the failure path), activeCtx returns an already-cancelled context
// rather than nil, so callers surface a clean error instead of panicking.
func TestActiveCtxNeverNil(t *testing.T) {
	s := &mcpSession{browserReady: make(chan struct{})}
	close(s.browserReady) // setup finished, ctx never populated

	ctx := s.activeCtx()
	if ctx == nil {
		t.Fatal("activeCtx() returned nil; callers would panic in context.WithTimeout")
	}
	if ctx.Err() == nil {
		t.Fatal("activeCtx() returned a live context on failure; want already-cancelled")
	}

	// The whole point: requestToolCtx must not panic on this context.
	tctx, cancel := requestToolCtx(context.Background(), ctx, time.Second)
	defer cancel()
	if tctx == nil {
		t.Fatal("requestToolCtx returned nil context")
	}
}

// TestSignalBrowserReady verifies that signaling unblocks waiters, that
// repeated signals do not panic (the setup goroutine's deferred signal runs
// after the connect tool may have already signaled), and that a session
// without a readiness channel is a no-op.
func TestSignalBrowserReady(t *testing.T) {
	s := &mcpSession{browserReady: make(chan struct{})}
	s.signalBrowserReady()
	s.signalBrowserReady() // second signal must not close twice

	select {
	case <-s.browserReady:
	default:
		t.Fatal("browserReady not closed after signalBrowserReady")
	}

	(&mcpSession{}).signalBrowserReady() // nil channel: no-op, no panic
}

// TestBrowserContextReportsSetupError verifies that browserContext returns the
// recorded setup error (and no context) once setup has failed, instead of
// blocking forever or handing back a nil browser context.
func TestBrowserContextReportsSetupError(t *testing.T) {
	wantErr := errors.New("attach: connection refused")
	s := &mcpSession{browserReady: make(chan struct{}), setupErr: wantErr}
	close(s.browserReady)

	ctx, err := s.browserContext(context.Background())
	if ctx != nil {
		t.Fatalf("browserContext() ctx = %v, want nil on setup failure", ctx)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("browserContext() err = %v, want wrapping %v", err, wantErr)
	}
}

// TestBrowserContextHonorsRequestCancellation verifies that a tool call whose
// request context is cancelled while the browser is still coming up returns
// promptly with an error rather than blocking on browserReady indefinitely.
func TestBrowserContextHonorsRequestCancellation(t *testing.T) {
	s := &mcpSession{browserReady: make(chan struct{})} // never closed: setup still running

	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()

	ctx, err := s.browserContext(reqCtx)
	if ctx != nil {
		t.Fatalf("browserContext() ctx = %v, want nil when request is cancelled", ctx)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("browserContext() err = %v, want context.Canceled", err)
	}
}
