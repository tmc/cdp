package main

import (
	"math"
	"strings"
	"testing"
)

func TestAnalyzeTraceCoreWebVitals(t *testing.T) {
	trace := `{
		"traceEvents": [
			{"name":"navigationStart","ts":1000000,"args":{"data":{}}},
			{"name":"LargestContentfulPaint::Candidate","ts":1250000,"args":{"data":{"size":1234}}},
			{"name":"EventTiming","ts":1300000,"dur":40000,"args":{"data":{"interactionId":1,"duration":40}}},
			{"name":"EventTiming","ts":1400000,"dur":75000,"args":{"data":{"interactionId":2,"duration":75}}},
			{"name":"LayoutShift","ts":1500000,"args":{"data":{"score":0.10,"had_recent_input":false}}},
			{"name":"LayoutShift","ts":1600000,"args":{"data":{"score":0.20,"had_recent_input":true}}},
			{"name":"LayoutShift","ts":1700000,"args":{"data":{"score":0.05,"had_recent_input":false}}}
		]
	}`

	got, err := analyzeTrace(strings.NewReader(trace))
	if err != nil {
		t.Fatal(err)
	}
	if got.LCPMS != 250 {
		t.Fatalf("LCPMS = %v, want 250", got.LCPMS)
	}
	if got.INPMS != 75 {
		t.Fatalf("INPMS = %v, want 75", got.INPMS)
	}
	if math.Abs(got.CLS-0.15) > 1e-9 {
		t.Fatalf("CLS = %v, want 0.15", got.CLS)
	}
}
