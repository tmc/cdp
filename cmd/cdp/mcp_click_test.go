package main

import (
	"reflect"
	"testing"
)

func TestParseCoordSelector(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		want     viewportPoint
		wantOK   bool
		wantErr  bool
	}{
		{name: "css selector", selector: "button", wantOK: false},
		{name: "ref selector", selector: "@1", wantOK: false},
		{name: "integer coords", selector: "coord:100,200", want: viewportPoint{X: 100, Y: 200}, wantOK: true},
		{name: "decimal coords", selector: "coord:12.5,0.75", want: viewportPoint{X: 12.5, Y: 0.75}, wantOK: true},
		{name: "whitespace", selector: " coord: 12 , 34 ", want: viewportPoint{X: 12, Y: 34}, wantOK: true},
		{name: "missing comma", selector: "coord:12", wantOK: true, wantErr: true},
		{name: "extra comma", selector: "coord:12,34,56", wantOK: true, wantErr: true},
		{name: "non number", selector: "coord:x,34", wantOK: true, wantErr: true},
		{name: "negative x", selector: "coord:-1,34", wantOK: true, wantErr: true},
		{name: "negative y", selector: "coord:1,-34", wantOK: true, wantErr: true},
		{name: "nan", selector: "coord:NaN,34", wantOK: true, wantErr: true},
		{name: "inf", selector: "coord:+Inf,34", wantOK: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := parseCoordSelector(tt.selector)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("point = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestValidateRawCDPInput(t *testing.T) {
	tests := []struct {
		name       string
		input      RawCDPInput
		wantMethod string
		wantTarget string
		wantErr    bool
	}{
		{name: "target default", input: RawCDPInput{Method: "Runtime.evaluate"}, wantMethod: "Runtime.evaluate", wantTarget: "target"},
		{name: "browser target", input: RawCDPInput{Method: "Browser.getVersion", Target: "browser"}, wantMethod: "Browser.getVersion", wantTarget: "browser"},
		{name: "trim method", input: RawCDPInput{Method: " Runtime.evaluate "}, wantMethod: "Runtime.evaluate", wantTarget: "target"},
		{name: "missing method", input: RawCDPInput{}, wantErr: true},
		{name: "no domain separator", input: RawCDPInput{Method: "Runtime"}, wantErr: true},
		{name: "too many separators", input: RawCDPInput{Method: "Runtime.evaluate.now"}, wantErr: true},
		{name: "whitespace in method", input: RawCDPInput{Method: "Runtime. evaluate"}, wantErr: true},
		{name: "invalid target", input: RawCDPInput{Method: "Runtime.evaluate", Target: "page"}, wantErr: true},
		{name: "block browser close", input: RawCDPInput{Method: "Browser.close", Target: "browser"}, wantErr: true},
		{name: "block target close", input: RawCDPInput{Method: "Target.closeTarget", Target: "browser"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method, target, err := validateRawCDPInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if method != tt.wantMethod || target != tt.wantTarget {
				t.Fatalf("method,target = %q,%q; want %q,%q", method, target, tt.wantMethod, tt.wantTarget)
			}
		})
	}
}

func TestParseRawCDPCommand(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		wantMethod string
		wantParams map[string]any
		wantErr    bool
	}{
		{
			name:       "empty params",
			command:    "Page.reload",
			wantMethod: "Page.reload",
			wantParams: map[string]any{},
		},
		{
			name:       "json params",
			command:    `Runtime.evaluate {"expression":"document.title","returnByValue":true}`,
			wantMethod: "Runtime.evaluate",
			wantParams: map[string]any{"expression": "document.title", "returnByValue": true},
		},
		{
			name:    "invalid method",
			command: "Runtime.evaluate.now {}",
			wantErr: true,
		},
		{
			name:    "invalid json",
			command: `Runtime.evaluate {"expression":}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method, params, err := parseRawCDPCommand(tt.command)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseRawCDPCommand(%q) succeeded, want error", tt.command)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRawCDPCommand(%q): %v", tt.command, err)
			}
			if method != tt.wantMethod {
				t.Fatalf("method = %q, want %q", method, tt.wantMethod)
			}
			if !reflect.DeepEqual(params, tt.wantParams) {
				t.Fatalf("params = %#v, want %#v", params, tt.wantParams)
			}
		})
	}
}
