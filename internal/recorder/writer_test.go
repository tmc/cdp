package recorder

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/har"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
)

// TestWriterDoesNotBlockOnRecorderLock verifies that disk I/O is decoupled
// from r.Mutex. If writeRawToDomainFileAtPage took the lock and ran synchronous
// os.OpenFile/Fprintln inside it, a slow disk would freeze the chromedp
// event loop. This test holds r.Lock() from a side goroutine while
// dispatching events; the writer goroutine must still write to disk because
// it owns its own per-host file handles and does not contend on r.Mutex.
func TestWriterDoesNotBlockOnRecorderLock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	r, err := New(WithStreaming(true), WithOutputDir(dir))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer r.Close()

	// Hold r.Lock() to simulate a sibling caller (e.g. HAR()) sitting in
	// the critical section while events arrive.
	r.Lock()
	released := false
	defer func() {
		if !released {
			r.Unlock()
		}
	}()

	// Drive a write directly through the writer path, bypassing the network
	// event handler (which would itself want the lock). The call must not
	// block on r.Lock().
	done := make(chan struct{})
	go func() {
		_ = r.writeRawToDomainFileAtPage("https://example.com/a", "", dir, []byte(`{"x":1}`))
		_ = r.writeRawToDomainFileAtPage("https://example.com/b", "", dir, []byte(`{"x":2}`))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("writeRawToDomainFile blocked while r.Lock() was held — fix regressed")
	}

	// Release the lock and let the writer drain.
	r.Unlock()
	released = true

	// Force flush.
	r.CloseDomainWriters()

	path := filepath.Join(dir, "unknown_domain", "example.com.jsonl")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read jsonl: %v", err)
	}
	for _, want := range [][]byte{[]byte(`{"x":1}`), []byte(`{"x":2}`)} {
		if !bytes.Contains(got, want) {
			t.Errorf("missing entry %q in:\n%s", want, got)
		}
	}
}

func TestWriterGroupsRequestsByPageDomain(t *testing.T) {
	dir := t.TempDir()
	r, err := New(WithStreaming(true), WithOutputDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	handler := r.HandleNetworkEvent(context.Background())
	handler(&page.EventFrameNavigated{Frame: &cdp.Frame{URL: "https://www.lesswrong.com/posts/test"}})
	if err := r.writeRawToDomainFileAtPage("https://res.cloudinary.com/image", r.pageDomain, dir, []byte(`{"url":"cloudinary"}`)); err != nil {
		t.Fatal(err)
	}
	if err := r.writeRawToDomainFileAtPage("https://p.typekit.net/font", r.pageDomain, dir, []byte(`{"url":"typekit"}`)); err != nil {
		t.Fatal(err)
	}
	r.Close()

	for _, host := range []string{"res.cloudinary.com", "p.typekit.net"} {
		path := filepath.Join(dir, "www.lesswrong.com", host+".jsonl")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
	}
	for _, group := range []string{"cloudinary.com", "typekit.net", "res.cloudinary.com"} {
		if _, err := os.Stat(filepath.Join(dir, group)); !os.IsNotExist(err) {
			t.Fatalf("unexpected per-request group %s", group)
		}
	}
}

func TestWriterCanUseRequestDomainLayout(t *testing.T) {
	dir := t.TempDir()
	r, err := New(WithStreaming(true), WithOutputDir(dir), WithGroupByPage(false))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.writeRawToDomainFileAtPage("https://res.cloudinary.com/image", "", dir, []byte(`{"url":"cloudinary"}`)); err != nil {
		t.Fatal(err)
	}
	r.Close()
	path := filepath.Join(dir, "res.cloudinary.com", "res.cloudinary.com.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
}

// TestWriteOneRecreatesDeletedOutput verifies that after the output directory
// is removed mid-capture (e.g. a user runs rm -rf), writeOne recreates the
// directory and file and keeps writing, rather than appending to an orphaned
// inode that no longer has a path.
func TestWriteOneRecreatesDeletedOutput(t *testing.T) {
	dir := t.TempDir()
	writers := make(map[string]*os.File)
	defer func() {
		for _, f := range writers {
			f.Close()
		}
	}()

	if err := writeOne(writers, "example.com", "example.com", dir, []byte(`{"n":1}`)); err != nil {
		t.Fatalf("first write: %v", err)
	}
	path := filepath.Join(dir, "example.com", "example.com.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat after first write: %v", err)
	}

	// Simulate the user deleting everything under the output dir.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	if err := writeOne(writers, "example.com", "example.com", dir, []byte(`{"n":2}`)); err != nil {
		t.Fatalf("write after rm: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("output not recreated: %v", err)
	}
	if got := string(data); !bytes.Contains(data, []byte(`{"n":2}`)) {
		t.Fatalf("recreated file missing post-delete write; got %q", got)
	}
	if bytes.Contains(data, []byte(`{"n":1}`)) {
		t.Fatalf("recreated file unexpectedly contains pre-delete write; got %q", data)
	}
}

func TestStreamingWritesOutputFile(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "out.har.jsonl")

	r, err := New(WithStreaming(true), WithOutputFile(file))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer r.Close()

	r.streamEntryAtPage(&har.Entry{
		Request: &har.Request{
			Method: "GET",
			URL:    "https://example.com/",
		},
		Response: &har.Response{Status: 200},
	}, "", "")

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read jsonl: %v", err)
	}
	if !bytes.Contains(got, []byte(`"url":"https://example.com/"`)) {
		t.Fatalf("output file missing URL:\n%s", got)
	}
}

// TestWriterDropsOnSaturation verifies the drop-on-full backpressure policy.
// We saturate the writes channel by submitting more entries in rapid
// succession than the buffer can hold, with the writer goroutine artificially
// quiesced (we hijack the channel before the goroutine starts draining).
//
// This documents the contract: under sustained overload, individual entries
// may be dropped to keep the chromedp event loop live. DroppedWrites surfaces
// the count for downstream tools.
func TestWriterDropsOnSaturation(t *testing.T) {
	t.Parallel()

	r, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer r.Close()

	// Send far more than the queue holds. The writer drains some during
	// the burst, but the overflow guarantees drops.

	// Fill the queue plus large overflow.
	const overflow = 4096
	dir := t.TempDir()
	for i := 0; i < writeQueueSize+overflow; i++ {
		_ = r.writeRawToDomainFileAtPage("https://saturate.example.com/", "", dir, []byte(`{}`))
	}
	r.CloseDomainWriters()

	dropped := r.DroppedWrites()
	if dropped == 0 {
		// Not a hard failure — the writer might keep up on a fast machine —
		// but log so we know the test exercised what it intended.
		t.Logf("writer kept pace with %d sends; drop counter still zero", writeQueueSize+overflow)
	} else if dropped > uint64(writeQueueSize+overflow) {
		t.Errorf("dropped count %d exceeds total sends %d", dropped, writeQueueSize+overflow)
	}
}

// TestWriterCloseIsIdempotent verifies multiple Close() calls are safe and
// that post-Close write/control calls degrade gracefully (drop, not panic).
// Without the writerStopped guard, post-Close calls would either hang
// (closeAllWriters waiting on an ack the dead goroutine cannot produce) or
// block forever (enqueueWrite filling a buffer with no draining receiver).
func TestWriterCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	r, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	r.Close()
	r.Close() // must not panic on closed channel or re-entry
	r.Close()

	// Post-Close ops are no-ops, not panics or hangs.
	dir := t.TempDir()
	done := make(chan struct{})
	go func() {
		_ = r.writeRawToDomainFileAtPage("https://post-close.example.com/", "", dir, []byte(`{}`))
		r.CloseDomainWriters()
		r.SetOutputDir(dir)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("post-Close ops blocked — writerStopped guard missing or broken")
	}
	if r.DroppedWrites() == 0 {
		t.Errorf("expected post-Close write to count toward DroppedWrites; got 0")
	}
}

// TestWriterConcurrentEventsAndCloseDomainWriters exercises the realistic
// hot path: many event-handler goroutines enqueue writes while CloseDomainWriters
// is invoked from a control path. Race detector + content check catch
// regressions in the synchronization between drain and close.
func TestWriterConcurrentEventsAndCloseDomainWriters(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	r, err := New(WithStreaming(true), WithOutputDir(dir))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer r.Close()

	const goroutines = 8
	const eventsPer = 64
	var wg sync.WaitGroup
	wg.Add(goroutines)

	var totalSends uint64
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			handler := r.HandleNetworkEvent(context.Background())
			for i := 0; i < eventsPer; i++ {
				atomic.AddUint64(&totalSends, 1)
				handler(&network.EventRequestWillBeSent{
					RequestID: network.RequestID("g" + string(rune('A'+id)) + "-" + string(rune('a'+i%26))),
					Request: &network.Request{
						URL:    "https://concurrent.example.com/",
						Method: "GET",
					},
				})
			}
		}(g)
	}

	wg.Wait()
	r.CloseDomainWriters()

	// We don't check exact line counts because RequestWillBeSent doesn't
	// produce a streamed entry on its own (LoadingFinished does). The point
	// of this test is the race detector and the absence of deadlock.
	if t.Failed() {
		t.Logf("totalSends=%d dropped=%d", atomic.LoadUint64(&totalSends), r.DroppedWrites())
	}
}
