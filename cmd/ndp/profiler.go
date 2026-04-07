package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	"errors"

	"github.com/chromedp/cdproto/heapprofiler"
	"github.com/chromedp/cdproto/profiler"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/gorilla/websocket"
)

// ProfileType represents the type of profile
type ProfileType string

const (
	ProfileTypeCPU  ProfileType = "cpu"
	ProfileTypeHeap ProfileType = "heap"
)

// CPUProfile represents a CPU profile
type CPUProfile struct {
	Nodes      []ProfileNode `json:"nodes"`
	StartTime  int64         `json:"startTime"`
	EndTime    int64         `json:"endTime"`
	Samples    []int         `json:"samples"`
	TimeDeltas []int         `json:"timeDeltas"`
}

// ProfileNode represents a node in the profile tree
type ProfileNode struct {
	ID            int            `json:"id"`
	CallFrame     CallFrame      `json:"callFrame"`
	HitCount      int            `json:"hitCount,omitempty"`
	Children      []int          `json:"children,omitempty"`
	PositionTicks []PositionTick `json:"positionTicks,omitempty"`
}

// CallFrame represents a call frame
type CallFrame struct {
	FunctionName string `json:"functionName"`
	ScriptID     string `json:"scriptId"`
	URL          string `json:"url"`
	LineNumber   int    `json:"lineNumber"`
	ColumnNumber int    `json:"columnNumber"`
}

// PositionTick represents position-specific timing
type PositionTick struct {
	Line  int `json:"line"`
	Ticks int `json:"ticks"`
}

// HeapSnapshot represents a heap snapshot
type HeapSnapshot struct {
	Snapshot   interface{} `json:"snapshot"`
	TotalBytes int64       `json:"totalBytes"`
	UsedBytes  int64       `json:"usedBytes"`
	Timestamp  time.Time   `json:"timestamp"`
}

// MemoryInfo represents memory usage information
type MemoryInfo struct {
	JSHeapSizeLimit int64 `json:"jsHeapSizeLimit"`
	TotalJSHeapSize int64 `json:"totalJSHeapSize"`
	UsedJSHeapSize  int64 `json:"usedJSHeapSize"`
}

// Profiler handles CPU and heap profiling
type Profiler struct {
	manager      *SessionManager
	session      *Session
	verbose      bool
	cpuProfile   *CPUProfile
	heapSnapshot *HeapSnapshot
	rawConn      *websocket.Conn
}

// ... NewProfiler remains same ...

// executeRaw runs a raw CDP method via WebSocket
func (p *Profiler) executeRaw(ctx context.Context, method string, params interface{}) (interface{}, error) {
	if p.session == nil || p.session.Target.WebSocketDebuggerURL == "" {
		return nil, errors.New("no active raw websocket session")
	}

	// Ensure connection exists
	if p.rawConn == nil {
		conn, _, err := websocket.DefaultDialer.Dial(p.session.Target.WebSocketDebuggerURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to dial raw websocket: %w", err)
		}
		p.rawConn = conn
	}

	// Send request
	id := 2000 + (time.Now().UnixNano() % 10000)
	req := map[string]interface{}{
		"id":     id,
		"method": method,
		"params": params,
	}

	if err := p.rawConn.WriteJSON(req); err != nil {
		return nil, fmt.Errorf("failed to write generic request: %w", err)
	}

	// Read loop to find response
	// Set deadline based on context
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}
	p.rawConn.SetReadDeadline(deadline)

	for {
		var msg map[string]interface{}
		if err := p.rawConn.ReadJSON(&msg); err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		// Check if this is our response
		if msgID, ok := msg["id"].(float64); ok && int64(msgID) == int64(id) {
			if errObj, ok := msg["error"]; ok {
				return nil, fmt.Errorf("CDP error: %v", errObj)
			}
			return msg["result"], nil
		}
	}
}

// NewProfiler creates a new profiler
func NewProfiler(verbose bool) *Profiler {
	return &Profiler{
		manager: NewSessionManager(verbose),
		verbose: verbose,
	}
}

// ProfileCPU starts CPU profiling
func (p *Profiler) ProfileCPU(ctx context.Context, duration string, outputFile string, targetID string) error {
	// Parse duration
	dur, err := time.ParseDuration(duration)
	if err != nil {
		return fmt.Errorf("invalid duration format: %w", err)
	}

	// Get or create session
	if err := p.ensureSession(ctx, targetID); err != nil {
		return err
	}

	fmt.Printf("Starting CPU profiling for %s...\n", duration)

	// Start profiling
	if err := p.startCPUProfiling(ctx); err != nil {
		return fmt.Errorf("failed to start CPU profiling: %w", err)
	}

	// Wait for duration
	select {
	case <-time.After(dur):
		// Duration elapsed
	case <-ctx.Done():
		// Context cancelled
		return ctx.Err()
	}

	// Stop profiling
	profile, err := p.stopCPUProfiling(ctx)
	if err != nil {
		return fmt.Errorf("failed to stop CPU profiling: %w", err)
	}

	p.cpuProfile = profile

	// Save profile
	if outputFile != "" {
		if err := p.saveCPUProfile(outputFile); err != nil {
			return fmt.Errorf("failed to save CPU profile: %w", err)
		}
		fmt.Printf("CPU profile saved to: %s\n", outputFile)
	}

	// Print summary
	p.printCPUProfileSummary()

	return nil
}

// startCPUProfiling starts CPU profiling
func (p *Profiler) startCPUProfiling(ctx context.Context) error {
	// Node.js support via raw WebSocket
	if p.session.Target.Type == SessionTypeNode {
		if p.verbose {
			log.Println("Enabling Profiler domain...")
		}
		if _, err := p.executeRaw(ctx, "Profiler.enable", nil); err != nil {
			return fmt.Errorf("failed to enable profiler: %w", err)
		}
		if p.verbose {
			log.Println("Starting Profiler...")
		}
		if _, err := p.executeRaw(ctx, "Profiler.start", nil); err != nil {
			return fmt.Errorf("failed to start profiler: %w", err)
		}
		if p.verbose {
			log.Println("CPU profiling started (Node.js raw)")
		}
		return nil
	}

	return chromedp.Run(p.session.ChromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Enable profiler
			if err := profiler.Enable().Do(ctx); err != nil {
				return err
			}

			// Start profiling
			if err := profiler.Start().Do(ctx); err != nil {
				return err
			}

			if p.verbose {
				log.Println("CPU profiling started")
			}

			return nil
		}),
	)
}

// stopCPUProfiling stops CPU profiling and returns the profile
func (p *Profiler) stopCPUProfiling(ctx context.Context) (*CPUProfile, error) {
	var profile *profiler.Profile

	// Node.js support via raw WebSocket
	if p.session.Target.Type == SessionTypeNode {
		res, err := p.executeRaw(ctx, "Profiler.stop", nil)
		if err != nil {
			return nil, err
		}

		// Map result to profile
		if resMap, ok := res.(map[string]interface{}); ok {
			if profileMap, ok := resMap["profile"]; ok {
				// Marshal back to JSON to unmarshal into struct
				// Efficient? No. Reliable? Yes.
				bytes, _ := json.Marshal(profileMap)
				profile = &profiler.Profile{}
				if err := json.Unmarshal(bytes, profile); err != nil {
					return nil, fmt.Errorf("failed to decode profile: %w", err)
				}
			}
		}

		if _, err := p.executeRaw(ctx, "Profiler.disable", nil); err != nil {
			log.Printf("Warning: failed to disable profiler: %v", err)
		}
	} else {
		err := chromedp.Run(p.session.ChromeCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				// Stop profiling
				var err error
				profile, err = profiler.Stop().Do(ctx)
				if err != nil {
					return err
				}

				// Disable profiler
				if err := profiler.Disable().Do(ctx); err != nil {
					if p.verbose {
						log.Printf("Warning: failed to disable profiler: %v", err)
					}
				}

				return nil
			}),
		)
		if err != nil {
			return nil, err
		}
	}

	if profile == nil {
		return nil, errors.New("no profile data received")
	}

	// Convert to our format
	cpuProfile := &CPUProfile{
		StartTime: int64(profile.StartTime),
		EndTime:   int64(profile.EndTime),
	}

	// Convert nodes
	for _, node := range profile.Nodes {
		profileNode := ProfileNode{
			ID: int(node.ID),
			CallFrame: CallFrame{
				FunctionName: node.CallFrame.FunctionName,
				ScriptID:     string(node.CallFrame.ScriptID),
				URL:          node.CallFrame.URL,
				LineNumber:   int(node.CallFrame.LineNumber),
				ColumnNumber: int(node.CallFrame.ColumnNumber),
			},
			HitCount: int(node.HitCount),
		}

		if node.Children != nil {
			children := make([]int, len(node.Children))
			for i, child := range node.Children {
				children[i] = int(child)
			}
			profileNode.Children = children
		}

		cpuProfile.Nodes = append(cpuProfile.Nodes, profileNode)
	}

	if profile.Samples != nil {
		samples := make([]int, len(profile.Samples))
		for i, sample := range profile.Samples {
			samples[i] = int(sample)
		}
		cpuProfile.Samples = samples
	}

	if profile.TimeDeltas != nil {
		timeDeltas := make([]int, len(profile.TimeDeltas))
		for i, delta := range profile.TimeDeltas {
			timeDeltas[i] = int(delta)
		}
		cpuProfile.TimeDeltas = timeDeltas
	}

	return cpuProfile, nil
}

// saveCPUProfile saves the CPU profile to a file
func (p *Profiler) saveCPUProfile(filename string) error {
	if p.cpuProfile == nil {
		return errors.New("no CPU profile available")
	}

	data, err := json.MarshalIndent(p.cpuProfile, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filename, data, 0644)
}

// printCPUProfileSummary prints a summary of the CPU profile
func (p *Profiler) printCPUProfileSummary() {
	if p.cpuProfile == nil {
		return
	}

	fmt.Println("\nCPU Profile Summary:")
	fmt.Printf("Duration: %dms\n", (p.cpuProfile.EndTime - p.cpuProfile.StartTime))
	fmt.Printf("Samples: %d\n", len(p.cpuProfile.Samples))
	fmt.Printf("Functions: %d\n", len(p.cpuProfile.Nodes))

	// Find top functions by hit count
	type functionStat struct {
		name     string
		hitCount int
	}

	var stats []functionStat
	for _, node := range p.cpuProfile.Nodes {
		if node.HitCount > 0 {
			stats = append(stats, functionStat{
				name:     node.CallFrame.FunctionName,
				hitCount: node.HitCount,
			})
		}
	}

	// Sort by hit count
	// Simple bubble sort for top 10
	for i := 0; i < len(stats) && i < 10; i++ {
		for j := i + 1; j < len(stats); j++ {
			if stats[j].hitCount > stats[i].hitCount {
				stats[i], stats[j] = stats[j], stats[i]
			}
		}
	}

	if len(stats) > 0 {
		fmt.Println("\nTop Functions by CPU Time:")
		for i, stat := range stats {
			if i >= 10 {
				break
			}
			name := stat.name
			if name == "" {
				name = "(anonymous)"
			}
			fmt.Printf("  %2d. %s - %d samples\n", i+1, name, stat.hitCount)
		}
	}
}

// ProfileHeap takes a heap snapshot
func (p *Profiler) ProfileHeap(ctx context.Context, outputFile string, targetID string) error {
	// Get or create session
	if err := p.ensureSession(ctx, targetID); err != nil {
		return err
	}

	fmt.Println("Taking heap snapshot...")

	// Get memory info first
	memInfo, err := p.getMemoryInfo(ctx)
	if err != nil && p.verbose {
		log.Printf("Warning: failed to get memory info: %v", err)
	}

	if memInfo != nil {
		fmt.Printf("Current memory usage: %.2f MB / %.2f MB\n",
			float64(memInfo.UsedJSHeapSize)/1024/1024,
			float64(memInfo.TotalJSHeapSize)/1024/1024)
	}

	// Take heap snapshot
	snapshot, err := p.takeHeapSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("failed to take heap snapshot: %w", err)
	}

	p.heapSnapshot = snapshot

	// Save snapshot
	if outputFile != "" {
		if err := p.saveHeapSnapshot(outputFile); err != nil {
			return fmt.Errorf("failed to save heap snapshot: %w", err)
		}
		fmt.Printf("Heap snapshot saved to: %s\n", outputFile)
	}

	// Print summary
	p.printHeapSnapshotSummary()

	return nil
}

// takeHeapSnapshot takes a heap snapshot
func (p *Profiler) takeHeapSnapshot(ctx context.Context) (*HeapSnapshot, error) {
	// Node.js support
	if p.session.Target.Type == SessionTypeNode {
		return p.takeHeapSnapshotNode(ctx)
	}

	var chunks []string

	err := chromedp.Run(p.session.ChromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Enable heap profiler
			if err := heapprofiler.Enable().Do(ctx); err != nil {
				return err
			}

			// Listen for heap snapshot chunks
			chromedp.ListenTarget(ctx, func(ev interface{}) {
				switch ev := ev.(type) {
				case *heapprofiler.EventAddHeapSnapshotChunk:
					chunks = append(chunks, ev.Chunk)
					if p.verbose {
						log.Printf("Received heap snapshot chunk: %d bytes", len(ev.Chunk))
					}

				case *heapprofiler.EventReportHeapSnapshotProgress:
					if ev.Done > 0 && ev.Total > 0 {
						progress := float64(ev.Done) / float64(ev.Total) * 100
						fmt.Printf("\rProgress: %.0f%%", progress)
					}
				}
			})

			// Take snapshot
			if err := heapprofiler.TakeHeapSnapshot().Do(ctx); err != nil {
				return err
			}

			// Disable heap profiler
			if err := heapprofiler.Disable().Do(ctx); err != nil {
				if p.verbose {
					log.Printf("Warning: failed to disable heap profiler: %v", err)
				}
			}

			return nil
		}),
	)

	if err != nil {
		return nil, err
	}

	return p.processSnapshotChunks(chunks)
}

func (p *Profiler) takeHeapSnapshotNode(ctx context.Context) (*HeapSnapshot, error) {
	if _, err := p.executeRaw(ctx, "HeapProfiler.enable", nil); err != nil {
		return nil, fmt.Errorf("failed to enable heap profiler: %w", err)
	}

	var chunks []string

	// We need a specialized executeRaw that captures events
	// Or we create a one-off loop here.
	if p.rawConn == nil {
		// Should have been connected by ensureSession -> executeRaw
		// But ensureSession calls executeRaw, which dials.
		// If executeRaw closes it? No.
		// So p.rawConn should be valid or we redial.
		if _, err := p.executeRaw(ctx, "Runtime.enable", nil); err != nil { // Dummy to ensure conn
			return nil, err
		}
	}

	id := 3000 + (time.Now().UnixNano() % 10000)
	req := map[string]interface{}{
		"id":     id,
		"method": "HeapProfiler.takeHeapSnapshot",
		"params": map[string]interface{}{
			"reportProgress": true,
		},
	}

	if err := p.rawConn.WriteJSON(req); err != nil {
		return nil, fmt.Errorf("failed to write snapshot request: %w", err)
	}

	// Read loop
	// We expect chunks and progress events, then a result for our ID.
	done := false
	for !done {
		var msg map[string]interface{}
		if err := p.rawConn.ReadJSON(&msg); err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		// Check for method (Event)
		if method, ok := msg["method"].(string); ok {
			params, _ := msg["params"].(map[string]interface{})
			switch method {
			case "HeapProfiler.addHeapSnapshotChunk":
				if chunk, ok := params["chunk"].(string); ok {
					chunks = append(chunks, chunk)
					if p.verbose {
						// log.Printf("Chunk: %d bytes", len(chunk))
					}
				}
			case "HeapProfiler.reportHeapSnapshotProgress":
				doneVal, _ := params["done"].(float64)
				totalVal, _ := params["total"].(float64)
				if totalVal > 0 {
					fmt.Printf("\rProgress: %.0f%%", doneVal/totalVal*100)
				}
			}
		}

		// Check for result (Response)
		if msgID, ok := msg["id"].(float64); ok && int64(msgID) == int64(id) {
			if errObj, ok := msg["error"]; ok {
				return nil, fmt.Errorf("CDP error: %v", errObj)
			}
			// Done
			done = true
		}
	}
	fmt.Println() // Newline after progress

	if _, err := p.executeRaw(ctx, "HeapProfiler.disable", nil); err != nil {
		// ignore
	}

	return p.processSnapshotChunks(chunks)
}

func (p *Profiler) processSnapshotChunks(chunks []string) (*HeapSnapshot, error) {
	// Combine chunks
	snapshotData := ""
	totalBytes := int64(0)
	for _, chunk := range chunks {
		snapshotData += chunk
		totalBytes += int64(len(chunk))
	}

	// Parse snapshot
	var snapshotObj interface{}
	// Only unmarshal if reasonable size to avoid OOM in tool?
	// Heap snapshots can be 50MB+.
	// We'll try.
	if err := json.Unmarshal([]byte(snapshotData), &snapshotObj); err != nil {
		if p.verbose {
			log.Printf("Warning: failed to parse heap snapshot: %v", err)
		}
		snapshotObj = snapshotData
	}

	return &HeapSnapshot{
		Snapshot:   snapshotObj,
		TotalBytes: totalBytes,
		Timestamp:  time.Now(),
	}, nil
}

// saveHeapSnapshot saves the heap snapshot to a file
func (p *Profiler) saveHeapSnapshot(filename string) error {
	if p.heapSnapshot == nil {
		return errors.New("no heap snapshot available")
	}

	var data []byte
	var err error

	// Check if snapshot is a string (raw data) or parsed object
	if str, ok := p.heapSnapshot.Snapshot.(string); ok {
		data = []byte(str)
	} else {
		data, err = json.MarshalIndent(p.heapSnapshot.Snapshot, "", "  ")
		if err != nil {
			return err
		}
	}

	return ioutil.WriteFile(filename, data, 0644)
}

// printHeapSnapshotSummary prints a summary of the heap snapshot
func (p *Profiler) printHeapSnapshotSummary() {
	if p.heapSnapshot == nil {
		return
	}

	fmt.Println("\nHeap Snapshot Summary:")
	fmt.Printf("Timestamp: %s\n", p.heapSnapshot.Timestamp.Format(time.RFC3339))
	fmt.Printf("Snapshot size: %.2f MB\n", float64(p.heapSnapshot.TotalBytes)/1024/1024)

	// If we have parsed snapshot data, show more details
	if snapshotMap, ok := p.heapSnapshot.Snapshot.(map[string]interface{}); ok {
		if nodes, ok := snapshotMap["nodes"]; ok {
			if nodeArr, ok := nodes.([]interface{}); ok {
				fmt.Printf("Heap nodes: %d\n", len(nodeArr))
			}
		}
	}
}

// getMemoryInfo gets current memory usage information
func (p *Profiler) getMemoryInfo(ctx context.Context) (*MemoryInfo, error) {
	var result interface{}

	err := chromedp.Run(p.session.ChromeCtx,
		chromedp.Evaluate(`performance.memory`, &result),
	)

	if err != nil {
		return nil, err
	}

	if memMap, ok := result.(map[string]interface{}); ok {
		info := &MemoryInfo{}

		if val, ok := memMap["jsHeapSizeLimit"].(float64); ok {
			info.JSHeapSizeLimit = int64(val)
		}
		if val, ok := memMap["totalJSHeapSize"].(float64); ok {
			info.TotalJSHeapSize = int64(val)
		}
		if val, ok := memMap["usedJSHeapSize"].(float64); ok {
			info.UsedJSHeapSize = int64(val)
		}

		return info, nil
	}

	return nil, errors.New("failed to parse memory info")
}

// ensureSession ensures there's an active session
func (p *Profiler) ensureSession(ctx context.Context, targetID string) error {
	if p.session != nil {
		return nil
	}

	// Find target
	targets, err := p.manager.ListTargets(ctx)
	if err != nil {
		return fmt.Errorf("failed to list targets: %w", err)
	}

	if len(targets) == 0 {
		return errors.New("no debug targets found")
	}

	var target *DebugTarget
	if targetID != "" {
		// Find specific target
		for _, t := range targets {
			if t.ID == targetID {
				target = &t
				break
			}
		}
		if target == nil {
			return fmt.Errorf("target %s not found", targetID)
		}
	} else {
		// Use first available target
		target = &targets[0]
	}

	// Create session
	session, err := p.manager.CreateSession(ctx, *target)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	p.session = session

	// Enable runtime
	if target.Type == SessionTypeNode {
		if _, err := p.executeRaw(ctx, "Runtime.enable", nil); err != nil {
			// Log but ignore, might not be critical or already enabled
			if p.verbose {
				log.Printf("Warning: failed to enable runtime (Node.js raw): %v", err)
			}
		}
	} else {
		err = chromedp.Run(p.session.ChromeCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return runtime.Enable().Do(ctx)
			}),
		)

		if err != nil {
			return fmt.Errorf("failed to enable runtime: %w", err)
		}
	}

	fmt.Printf("Connected to target: %s\n", target.Title)

	return nil
}

// StartSampling starts sampling heap profiler
func (p *Profiler) StartSampling(ctx context.Context, interval int) error {
	if err := p.ensureSession(ctx, ""); err != nil {
		return err
	}

	return chromedp.Run(p.session.ChromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Enable heap profiler
			if err := heapprofiler.Enable().Do(ctx); err != nil {
				return err
			}

			// Start sampling
			params := heapprofiler.StartSampling()
			if interval > 0 {
				params = params.WithSamplingInterval(float64(interval))
			}

			if err := params.Do(ctx); err != nil {
				return err
			}

			fmt.Printf("Heap sampling started (interval: %d bytes)\n", interval)

			return nil
		}),
	)
}

// StopSampling stops sampling and returns the profile
func (p *Profiler) StopSampling(ctx context.Context) error {
	if p.session == nil {
		return errors.New("no active session")
	}

	var profile *heapprofiler.SamplingHeapProfile

	err := chromedp.Run(p.session.ChromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Stop sampling
			var err error
			profile, err = heapprofiler.StopSampling().Do(ctx)
			if err != nil {
				return err
			}

			// Disable heap profiler
			if err := heapprofiler.Disable().Do(ctx); err != nil {
				if p.verbose {
					log.Printf("Warning: failed to disable heap profiler: %v", err)
				}
			}

			return nil
		}),
	)

	if err != nil {
		return err
	}

	if profile != nil {
		fmt.Printf("Heap sampling stopped. Profile head: %v\n", profile.Head)
	}

	return nil
}

// Close closes the profiler session
func (p *Profiler) Close() error {
	if p.session != nil {
		return p.manager.CloseSession(p.session.ID)
	}
	return nil
}
