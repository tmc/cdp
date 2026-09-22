package browser

import (
	"testing"
	"time"
)

func TestStdDev(t *testing.T) {
	tests := []struct {
		name string
		ds   []time.Duration
		mean time.Duration
		want time.Duration
	}{
		{"empty", nil, 0, 0},
		{"constant", []time.Duration{5 * time.Millisecond, 5 * time.Millisecond}, 5 * time.Millisecond, 0},
		{"two", []time.Duration{2 * time.Millisecond, 4 * time.Millisecond}, 3 * time.Millisecond, time.Millisecond},
		{"large", []time.Duration{0, 20 * time.Second}, 10 * time.Second, 10 * time.Second},
	}
	for _, tt := range tests {
		if got := stdDev(tt.ds, tt.mean); got != tt.want {
			t.Errorf("%s: stdDev = %v, want %v", tt.name, got, tt.want)
		}
	}
}
