package main

import "fmt"

// V8Profiler provides CPU and memory profiling capabilities
type V8Profiler struct {
	client *V8InspectorClient
}

// NewV8Profiler creates a new V8 profiler instance
func NewV8Profiler(client *V8InspectorClient) *V8Profiler {
	return &V8Profiler{client: client}
}

// EnableProfiler enables the Profiler domain
func (p *V8Profiler) EnableProfiler() error {
	_, err := p.client.SendCommand("Profiler.enable", nil)
	if err != nil {
		return err
	}

	p.client.profilerEnabled = true

	if p.client.verbose {
		fmt.Println("Profiler domain enabled")
	}

	return nil
}

// DisableProfiler disables the Profiler domain
func (p *V8Profiler) DisableProfiler() error {
	_, err := p.client.SendCommand("Profiler.disable", nil)
	if err != nil {
		return err
	}

	p.client.profilerEnabled = false
	return nil
}

// StartCPUProfiling starts CPU profiling
func (p *V8Profiler) StartCPUProfiling(title string, samplingInterval int) error {
	params := map[string]interface{}{}

	if title != "" {
		params["title"] = title
	}

	if samplingInterval > 0 {
		params["samplingInterval"] = samplingInterval
	}

	_, err := p.client.SendCommand("Profiler.start", params)
	if err != nil {
		return err
	}

	if p.client.verbose {
		fmt.Printf("CPU profiling started: %s (interval: %d μs)\n", title, samplingInterval)
	}

	return nil
}

// StopCPUProfiling stops CPU profiling and returns the profile
func (p *V8Profiler) StopCPUProfiling() (*V8CPUProfile, error) {
	result, err := p.client.SendCommand("Profiler.stop", nil)
	if err != nil {
		return nil, err
	}

	profile := p.parseCPUProfile(result)

	if p.client.verbose {
		fmt.Printf("CPU profiling stopped. Profile contains %d nodes\n", len(profile.Nodes))
	}

	return profile, nil
}

// V8CPUProfile represents a CPU profiling result
type V8CPUProfile struct {
	Nodes      []V8CPUProfileNode `json:"nodes"`
	StartTime  float64            `json:"startTime"`
	EndTime    float64            `json:"endTime"`
	Samples    []int              `json:"samples,omitempty"`
	TimeDeltas []int              `json:"timeDeltas,omitempty"`
}

// V8CPUProfileNode represents a node in the CPU profile call tree
type V8CPUProfileNode struct {
	ID            int                 `json:"id"`
	CallFrame     V8ProfilerCallFrame `json:"callFrame"`
	HitCount      int                 `json:"hitCount,omitempty"`
	Children      []int               `json:"children,omitempty"`
	DeoptReason   string              `json:"deoptReason,omitempty"`
	PositionTicks []PositionTickInfo  `json:"positionTicks,omitempty"`
}

// V8ProfilerCallFrame represents a JavaScript call frame in profiler context
type V8ProfilerCallFrame struct {
	FunctionName string `json:"functionName"`
	ScriptID     string `json:"scriptId"`
	URL          string `json:"url"`
	LineNumber   int    `json:"lineNumber"`
	ColumnNumber int    `json:"columnNumber"`
}

// PositionTickInfo provides position tick information
type PositionTickInfo struct {
	Line  int `json:"line"`
	Ticks int `json:"ticks"`
}

// SetSamplingInterval sets the CPU profiling sampling interval
func (p *V8Profiler) SetSamplingInterval(interval int) error {
	params := map[string]interface{}{
		"interval": interval,
	}

	_, err := p.client.SendCommand("Profiler.setSamplingInterval", params)
	if err != nil {
		return err
	}

	if p.client.verbose {
		fmt.Printf("CPU profiling sampling interval set to %d μs\n", interval)
	}

	return nil
}

// StartPreciseCoverage starts precise code coverage collection
func (p *V8Profiler) StartPreciseCoverage(callCount, detailed bool) error {
	params := map[string]interface{}{
		"callCount": callCount,
		"detailed":  detailed,
	}

	_, err := p.client.SendCommand("Profiler.startPreciseCoverage", params)
	if err != nil {
		return err
	}

	if p.client.verbose {
		fmt.Printf("Precise coverage started (callCount: %t, detailed: %t)\n", callCount, detailed)
	}

	return nil
}

// StopPreciseCoverage stops precise code coverage collection
func (p *V8Profiler) StopPreciseCoverage() error {
	_, err := p.client.SendCommand("Profiler.stopPreciseCoverage", nil)
	if err != nil {
		return err
	}

	if p.client.verbose {
		fmt.Println("Precise coverage stopped")
	}

	return nil
}

// TakePreciseCoverage retrieves precise coverage data
func (p *V8Profiler) TakePreciseCoverage() ([]ScriptCoverage, error) {
	result, err := p.client.SendCommand("Profiler.takePreciseCoverage", nil)
	if err != nil {
		return nil, err
	}

	var coverage []ScriptCoverage
	if resultArray, ok := result["result"].([]interface{}); ok {
		for _, item := range resultArray {
			if itemMap, ok := item.(map[string]interface{}); ok {
				coverage = append(coverage, p.parseScriptCoverage(itemMap))
			}
		}
	}

	if p.client.verbose {
		fmt.Printf("Retrieved coverage for %d scripts\n", len(coverage))
	}

	return coverage, nil
}

// ScriptCoverage represents coverage data for a script
type ScriptCoverage struct {
	ScriptID  string             `json:"scriptId"`
	URL       string             `json:"url"`
	Functions []FunctionCoverage `json:"functions"`
}

// FunctionCoverage represents coverage data for a function
type FunctionCoverage struct {
	FunctionName    string          `json:"functionName"`
	Ranges          []CoverageRange `json:"ranges"`
	IsBlockCoverage bool            `json:"isBlockCoverage"`
}

// CoverageRange represents a coverage range
type CoverageRange struct {
	StartOffset int `json:"startOffset"`
	EndOffset   int `json:"endOffset"`
	Count       int `json:"count"`
}

// GetBestEffortCoverage retrieves best-effort coverage data
func (p *V8Profiler) GetBestEffortCoverage() ([]ScriptCoverage, error) {
	result, err := p.client.SendCommand("Profiler.getBestEffortCoverage", nil)
	if err != nil {
		return nil, err
	}

	var coverage []ScriptCoverage
	if resultArray, ok := result["result"].([]interface{}); ok {
		for _, item := range resultArray {
			if itemMap, ok := item.(map[string]interface{}); ok {
				coverage = append(coverage, p.parseScriptCoverage(itemMap))
			}
		}
	}

	return coverage, nil
}

// StartTypeProfile starts type profiling
func (p *V8Profiler) StartTypeProfile() error {
	_, err := p.client.SendCommand("Profiler.startTypeProfile", nil)
	if err != nil {
		return err
	}

	if p.client.verbose {
		fmt.Println("Type profiling started")
	}

	return nil
}

// StopTypeProfile stops type profiling
func (p *V8Profiler) StopTypeProfile() error {
	_, err := p.client.SendCommand("Profiler.stopTypeProfile", nil)
	if err != nil {
		return err
	}

	if p.client.verbose {
		fmt.Println("Type profiling stopped")
	}

	return nil
}

// TakeTypeProfile retrieves type profile data
func (p *V8Profiler) TakeTypeProfile() ([]ScriptTypeProfile, error) {
	result, err := p.client.SendCommand("Profiler.takeTypeProfile", nil)
	if err != nil {
		return nil, err
	}

	var profiles []ScriptTypeProfile
	if resultArray, ok := result["result"].([]interface{}); ok {
		for _, item := range resultArray {
			if itemMap, ok := item.(map[string]interface{}); ok {
				profiles = append(profiles, p.parseScriptTypeProfile(itemMap))
			}
		}
	}

	return profiles, nil
}

// ScriptTypeProfile represents type profiling data for a script
type ScriptTypeProfile struct {
	ScriptID string             `json:"scriptId"`
	URL      string             `json:"url"`
	Entries  []TypeProfileEntry `json:"entries"`
}

// TypeProfileEntry represents a type profile entry
type TypeProfileEntry struct {
	Offset int          `json:"offset"`
	Types  []TypeObject `json:"types"`
}

// TypeObject represents a type in the type profile
type TypeObject struct {
	Name string `json:"name"`
}

// Heap Profiler Methods (using HeapProfiler domain)

// EnableHeapProfiler enables the HeapProfiler domain
func (p *V8Profiler) EnableHeapProfiler() error {
	_, err := p.client.SendCommand("HeapProfiler.enable", nil)
	if err != nil {
		return err
	}

	if p.client.verbose {
		fmt.Println("HeapProfiler domain enabled")
	}

	return nil
}

// DisableHeapProfiler disables the HeapProfiler domain
func (p *V8Profiler) DisableHeapProfiler() error {
	_, err := p.client.SendCommand("HeapProfiler.disable", nil)
	if err != nil {
		return err
	}

	return nil
}

// TakeHeapSnapshot takes a heap snapshot
func (p *V8Profiler) TakeHeapSnapshot(reportProgress bool) error {
	params := map[string]interface{}{}
	if reportProgress {
		params["reportProgress"] = reportProgress
	}

	_, err := p.client.SendCommand("HeapProfiler.takeHeapSnapshot", params)
	if err != nil {
		return err
	}

	if p.client.verbose {
		fmt.Println("Heap snapshot taken")
	}

	return nil
}

// StartHeapSampling starts heap sampling
func (p *V8Profiler) StartHeapSampling(samplingInterval float64) error {
	params := map[string]interface{}{}
	if samplingInterval > 0 {
		params["samplingInterval"] = samplingInterval
	}

	_, err := p.client.SendCommand("HeapProfiler.startSampling", params)
	if err != nil {
		return err
	}

	if p.client.verbose {
		fmt.Printf("Heap sampling started (interval: %.0f bytes)\n", samplingInterval)
	}

	return nil
}

// StopHeapSampling stops heap sampling and returns the profile
func (p *V8Profiler) StopHeapSampling() (*SamplingHeapProfile, error) {
	result, err := p.client.SendCommand("HeapProfiler.stopSampling", nil)
	if err != nil {
		return nil, err
	}

	profile := p.parseSamplingHeapProfile(result)

	if p.client.verbose {
		fmt.Printf("Heap sampling stopped. Profile contains %d nodes\n", len(profile.Head.Children))
	}

	return profile, nil
}

// SamplingHeapProfile represents a sampling heap profile
type SamplingHeapProfile struct {
	Head SamplingHeapProfileNode `json:"head"`
}

// SamplingHeapProfileNode represents a node in the sampling heap profile
type SamplingHeapProfileNode struct {
	CallFrame V8ProfilerCallFrame       `json:"callFrame"`
	SelfSize  int                       `json:"selfSize"`
	Children  []SamplingHeapProfileNode `json:"children"`
}

// GetObjectByHeapObjectID retrieves an object by heap object ID
func (p *V8Profiler) GetObjectByHeapObjectID(objectID string, objectGroup string) (*EvaluationResult, error) {
	params := map[string]interface{}{
		"objectId": objectID,
	}

	if objectGroup != "" {
		params["objectGroup"] = objectGroup
	}

	result, err := p.client.SendCommand("HeapProfiler.getObjectByHeapObjectId", params)
	if err != nil {
		return nil, err
	}

	// Parse the result similar to Runtime.evaluate
	runtime := NewV8Runtime(p.client)
	return runtime.parseEvaluationResult(result), nil
}

// Utility methods for parsing profiler responses

func (p *V8Profiler) parseCPUProfile(result map[string]interface{}) *V8CPUProfile {
	profile := &V8CPUProfile{}

	if profileData, ok := result["profile"].(map[string]interface{}); ok {
		if nodes, ok := profileData["nodes"].([]interface{}); ok {
			for _, node := range nodes {
				if nodeMap, ok := node.(map[string]interface{}); ok {
					profile.Nodes = append(profile.Nodes, p.parseV8CPUProfileNode(nodeMap))
				}
			}
		}

		if startTime, ok := profileData["startTime"].(float64); ok {
			profile.StartTime = startTime
		}

		if endTime, ok := profileData["endTime"].(float64); ok {
			profile.EndTime = endTime
		}

		if samples, ok := profileData["samples"].([]interface{}); ok {
			for _, sample := range samples {
				if sampleInt, ok := sample.(float64); ok {
					profile.Samples = append(profile.Samples, int(sampleInt))
				}
			}
		}

		if timeDeltas, ok := profileData["timeDeltas"].([]interface{}); ok {
			for _, delta := range timeDeltas {
				if deltaInt, ok := delta.(float64); ok {
					profile.TimeDeltas = append(profile.TimeDeltas, int(deltaInt))
				}
			}
		}
	}

	return profile
}

func (p *V8Profiler) parseV8CPUProfileNode(node map[string]interface{}) V8CPUProfileNode {
	profileNode := V8CPUProfileNode{}

	if id, ok := node["id"].(float64); ok {
		profileNode.ID = int(id)
	}

	if callFrame, ok := node["callFrame"].(map[string]interface{}); ok {
		profileNode.CallFrame = p.parseV8CallFrame(callFrame)
	}

	if hitCount, ok := node["hitCount"].(float64); ok {
		profileNode.HitCount = int(hitCount)
	}

	if children, ok := node["children"].([]interface{}); ok {
		for _, child := range children {
			if childInt, ok := child.(float64); ok {
				profileNode.Children = append(profileNode.Children, int(childInt))
			}
		}
	}

	return profileNode
}

func (p *V8Profiler) parseV8CallFrame(frame map[string]interface{}) V8ProfilerCallFrame {
	callFrame := V8ProfilerCallFrame{}

	if functionName, ok := frame["functionName"].(string); ok {
		callFrame.FunctionName = functionName
	}
	if scriptID, ok := frame["scriptId"].(string); ok {
		callFrame.ScriptID = scriptID
	}
	if url, ok := frame["url"].(string); ok {
		callFrame.URL = url
	}
	if lineNumber, ok := frame["lineNumber"].(float64); ok {
		callFrame.LineNumber = int(lineNumber)
	}
	if columnNumber, ok := frame["columnNumber"].(float64); ok {
		callFrame.ColumnNumber = int(columnNumber)
	}

	return callFrame
}

func (p *V8Profiler) parseScriptCoverage(coverage map[string]interface{}) ScriptCoverage {
	scriptCoverage := ScriptCoverage{}

	if scriptID, ok := coverage["scriptId"].(string); ok {
		scriptCoverage.ScriptID = scriptID
	}
	if url, ok := coverage["url"].(string); ok {
		scriptCoverage.URL = url
	}

	if functions, ok := coverage["functions"].([]interface{}); ok {
		for _, function := range functions {
			if functionMap, ok := function.(map[string]interface{}); ok {
				scriptCoverage.Functions = append(scriptCoverage.Functions, p.parseFunctionCoverage(functionMap))
			}
		}
	}

	return scriptCoverage
}

func (p *V8Profiler) parseFunctionCoverage(function map[string]interface{}) FunctionCoverage {
	functionCoverage := FunctionCoverage{}

	if functionName, ok := function["functionName"].(string); ok {
		functionCoverage.FunctionName = functionName
	}
	if isBlockCoverage, ok := function["isBlockCoverage"].(bool); ok {
		functionCoverage.IsBlockCoverage = isBlockCoverage
	}

	if ranges, ok := function["ranges"].([]interface{}); ok {
		for _, rangeItem := range ranges {
			if rangeMap, ok := rangeItem.(map[string]interface{}); ok {
				functionCoverage.Ranges = append(functionCoverage.Ranges, p.parseCoverageRange(rangeMap))
			}
		}
	}

	return functionCoverage
}

func (p *V8Profiler) parseCoverageRange(rangeItem map[string]interface{}) CoverageRange {
	coverageRange := CoverageRange{}

	if startOffset, ok := rangeItem["startOffset"].(float64); ok {
		coverageRange.StartOffset = int(startOffset)
	}
	if endOffset, ok := rangeItem["endOffset"].(float64); ok {
		coverageRange.EndOffset = int(endOffset)
	}
	if count, ok := rangeItem["count"].(float64); ok {
		coverageRange.Count = int(count)
	}

	return coverageRange
}

func (p *V8Profiler) parseScriptTypeProfile(profile map[string]interface{}) ScriptTypeProfile {
	scriptTypeProfile := ScriptTypeProfile{}

	if scriptID, ok := profile["scriptId"].(string); ok {
		scriptTypeProfile.ScriptID = scriptID
	}
	if url, ok := profile["url"].(string); ok {
		scriptTypeProfile.URL = url
	}

	if entries, ok := profile["entries"].([]interface{}); ok {
		for _, entry := range entries {
			if entryMap, ok := entry.(map[string]interface{}); ok {
				scriptTypeProfile.Entries = append(scriptTypeProfile.Entries, p.parseTypeProfileEntry(entryMap))
			}
		}
	}

	return scriptTypeProfile
}

func (p *V8Profiler) parseTypeProfileEntry(entry map[string]interface{}) TypeProfileEntry {
	typeProfileEntry := TypeProfileEntry{}

	if offset, ok := entry["offset"].(float64); ok {
		typeProfileEntry.Offset = int(offset)
	}

	if types, ok := entry["types"].([]interface{}); ok {
		for _, typeItem := range types {
			if typeMap, ok := typeItem.(map[string]interface{}); ok {
				if name, ok := typeMap["name"].(string); ok {
					typeProfileEntry.Types = append(typeProfileEntry.Types, TypeObject{Name: name})
				}
			}
		}
	}

	return typeProfileEntry
}

func (p *V8Profiler) parseSamplingHeapProfile(result map[string]interface{}) *SamplingHeapProfile {
	profile := &SamplingHeapProfile{}

	if profileData, ok := result["profile"].(map[string]interface{}); ok {
		if head, ok := profileData["head"].(map[string]interface{}); ok {
			profile.Head = p.parseSamplingHeapProfileNode(head)
		}
	}

	return profile
}

func (p *V8Profiler) parseSamplingHeapProfileNode(node map[string]interface{}) SamplingHeapProfileNode {
	profileNode := SamplingHeapProfileNode{}

	if callFrame, ok := node["callFrame"].(map[string]interface{}); ok {
		profileNode.CallFrame = p.parseV8CallFrame(callFrame)
	}

	if selfSize, ok := node["selfSize"].(float64); ok {
		profileNode.SelfSize = int(selfSize)
	}

	if children, ok := node["children"].([]interface{}); ok {
		for _, child := range children {
			if childMap, ok := child.(map[string]interface{}); ok {
				profileNode.Children = append(profileNode.Children, p.parseSamplingHeapProfileNode(childMap))
			}
		}
	}

	return profileNode
}
