package recorder

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
)

// writerCmd is a command sent from any event-loop goroutine to the writer
// goroutine. Exactly one of {write, closeAll, stop} fields is meaningful per
// message.
type writerCmd struct {
	// write fields
	hostname string
	dir      string
	data     []byte

	// control: when ack != nil, the writer signals completion of the
	// preceding command before continuing to the next.
	ack chan struct{}

	// op selects the operation. Default zero is opWrite.
	op writerOp
}

type writerOp uint8

const (
	opWrite writerOp = iota
	opCloseAll
	opStop
)

// writerLoop owns domainWriters exclusively. It runs in a single goroutine
// started by Recorder.New so file I/O does not block the chromedp event loop.
//
// Backpressure: the cmds channel is buffered. When full, callers drop the
// write and increment Recorder.dropped. Liveness of the event loop is
// preferred over completeness of the on-disk record under sustained
// overload; the dropped count surfaces in HAR output and verbose logs.
func (r *Recorder) writerLoop() {
	defer close(r.writerDone)

	domainWriters := make(map[string]*os.File)
	defer func() {
		for _, f := range domainWriters {
			f.Close()
		}
	}()

	for cmd := range r.writes {
		switch cmd.op {
		case opWrite:
			if err := writeOne(domainWriters, cmd.hostname, cmd.dir, cmd.data); err != nil {
				if r.verbose {
					log.Printf("recorder: write %s: %v", cmd.hostname, err)
				}
			}
		case opCloseAll:
			for hostname, f := range domainWriters {
				f.Close()
				delete(domainWriters, hostname)
			}
			if cmd.ack != nil {
				close(cmd.ack)
			}
		case opStop:
			if cmd.ack != nil {
				close(cmd.ack)
			}
			return
		}
	}
}

// writeOne resolves the per-host writer (opening lazily) and writes a single
// JSON line. domainWriters is owned by the writer goroutine; this function
// runs only from there.
func writeOne(domainWriters map[string]*os.File, hostname, dir string, data []byte) error {
	if hostname == "" {
		hostname = "unknown_domain"
	}

	writer, ok := domainWriters[hostname]
	if ok {
		if _, statErr := writer.Stat(); statErr != nil {
			writer.Close()
			delete(domainWriters, hostname)
			ok = false
		}
	}
	if !ok {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		filename := filepath.Join(dir, fmt.Sprintf("%s.jsonl", hostname))
		f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		writer = f
		domainWriters[hostname] = writer
	}

	_, err := fmt.Fprintln(writer, string(data))
	return err
}

// enqueueWrite sends a write to the writer goroutine without blocking. On
// channel saturation (buffer full), it drops the entry and increments the
// dropped counter; the chromedp event loop must not stall on disk I/O.
//
// After Close(), enqueueWrite drops silently and returns nil — the writer
// goroutine is gone, so there is no destination. Callers should not rely on
// post-Close writes landing on disk.
//
// Returns an error only for inputs the writer can never handle (e.g. an
// unparseable URL). Disk errors surface via the writer goroutine's verbose
// log; callers do not see them.
func (r *Recorder) enqueueWrite(rawURL, dir string, data []byte) error {
	if rawURL == "" {
		return fmt.Errorf("no URL")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	hostname := u.Hostname()

	if r.writerStopped.Load() {
		atomic.AddUint64(&r.dropped, 1)
		return nil
	}

	cmd := writerCmd{op: opWrite, hostname: hostname, dir: dir, data: data}
	select {
	case r.writes <- cmd:
	default:
		atomic.AddUint64(&r.dropped, 1)
		if r.verbose {
			n := atomic.LoadUint64(&r.dropped)
			if n == 1 || n%100 == 0 {
				log.Printf("recorder: write queue full, dropped %d entries", n)
			}
		}
	}
	return nil
}

// closeAllWriters synchronously flushes and closes any open per-host writers.
// Used by SetOutputDir to ensure the directory swap is observable on disk.
// No-op after Close.
func (r *Recorder) closeAllWriters() {
	if r.writes == nil || r.writerStopped.Load() {
		return
	}
	ack := make(chan struct{})
	r.writes <- writerCmd{op: opCloseAll, ack: ack}
	<-ack
}

// stopWriter signals the writer goroutine to drain its queue and exit, then
// blocks until it does. Idempotent: callers may invoke Close more than once.
func (r *Recorder) stopWriter() {
	if r.writes == nil {
		return
	}
	r.writerStopOnce.Do(func() {
		ack := make(chan struct{})
		r.writes <- writerCmd{op: opStop, ack: ack}
		<-ack
		<-r.writerDone
		// Mark stopped after the goroutine confirms exit so concurrent
		// callers transitioning post-Close see consistent state.
		r.writerStopped.Store(true)
	})
}

// DroppedWrites returns the count of writes dropped because the writer queue
// was saturated. Useful for surfacing record-quality in HAR metadata.
func (r *Recorder) DroppedWrites() uint64 {
	return atomic.LoadUint64(&r.dropped)
}
