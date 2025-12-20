package ast

import "fmt"

// Command is the interface for all CDP script commands.
type Command interface {
	String() string
	commandNode()
}

// NavigationCommand represents navigation commands (goto, back, forward, reload).
type NavigationCommand struct {
	Type string // goto, back, forward, reload
	URL  string // for goto
}

func (c *NavigationCommand) commandNode() {}
func (c *NavigationCommand) String() string {
	if c.Type == "goto" {
		return fmt.Sprintf("%s %s", c.Type, c.URL)
	}
	return c.Type
}

// WaitCommand represents wait commands.
type WaitCommand struct {
	Type      string // "for", "until", "duration"
	Selector  string // for "for" type
	Condition string // for "until" type (e.g., "network idle", "dom stable")
	Duration  string // for "duration" type (e.g., "2s", "500ms")
}

func (c *WaitCommand) commandNode() {}
func (c *WaitCommand) String() string {
	switch c.Type {
	case "for":
		return fmt.Sprintf("wait for %s", c.Selector)
	case "until":
		return fmt.Sprintf("wait until %s", c.Condition)
	case "duration":
		return fmt.Sprintf("wait %s", c.Duration)
	}
	return "wait"
}

// InteractionCommand represents user interaction commands.
type InteractionCommand struct {
	Type     string // click, fill, type, select, hover, press, scroll
	Selector string
	Value    string // for fill, type, select
	Target   string // for scroll (e.g., "to #element")
}

func (c *InteractionCommand) commandNode() {}
func (c *InteractionCommand) String() string {
	if c.Value != "" {
		return fmt.Sprintf("%s %s %s", c.Type, c.Selector, c.Value)
	}
	if c.Target != "" {
		return fmt.Sprintf("%s to %s", c.Type, c.Target)
	}
	return fmt.Sprintf("%s %s", c.Type, c.Selector)
}

// ExtractionCommand represents data extraction commands.
type ExtractionCommand struct {
	Selector  string
	Attribute string // optional, e.g., "src", "href"
	Variable  string // variable name to store result
}

func (c *ExtractionCommand) commandNode() {}
func (c *ExtractionCommand) String() string {
	if c.Attribute != "" {
		return fmt.Sprintf("extract %s attr %s as %s", c.Selector, c.Attribute, c.Variable)
	}
	return fmt.Sprintf("extract %s as %s", c.Selector, c.Variable)
}

// SaveCommand represents saving data to a file.
type SaveCommand struct {
	Variable string
	Filename string
}

func (c *SaveCommand) commandNode() {}
func (c *SaveCommand) String() string {
	return fmt.Sprintf("save %s to %s", c.Variable, c.Filename)
}

// AssertionCommand represents assertion commands.
type AssertionCommand struct {
	Type      string // selector, status, url, errors
	Selector  string
	Condition string // exists, contains, text
	Value     string
	Status    int
}

func (c *AssertionCommand) commandNode() {}
func (c *AssertionCommand) String() string {
	switch c.Type {
	case "selector":
		if c.Value != "" {
			return fmt.Sprintf("assert selector %s %s %s", c.Selector, c.Condition, c.Value)
		}
		return fmt.Sprintf("assert selector %s %s", c.Selector, c.Condition)
	case "status":
		return fmt.Sprintf("assert status %d", c.Status)
	case "no-errors":
		return "assert no errors"
	case "url":
		return fmt.Sprintf("assert url %s %s", c.Condition, c.Value)
	}
	return "assert"
}

// NetworkCommand represents network-related commands.
type NetworkCommand struct {
	Type     string // capture, mock, block, throttle
	Pattern  string
	Target   string // for capture (filename), mock (json file)
	Resource string // for mock (api endpoint)
}

func (c *NetworkCommand) commandNode() {}
func (c *NetworkCommand) String() string {
	switch c.Type {
	case "capture":
		return fmt.Sprintf("capture network to %s", c.Target)
	case "mock":
		return fmt.Sprintf("mock api %s with %s", c.Resource, c.Target)
	case "block":
		return fmt.Sprintf("block %s", c.Pattern)
	case "throttle":
		return fmt.Sprintf("throttle %s", c.Pattern)
	}
	return "network"
}

// OutputCommand represents output commands (screenshot, pdf, har).
type OutputCommand struct {
	Type     string // screenshot, pdf, har
	Filename string
	Selector string // optional, for element screenshots
}

func (c *OutputCommand) commandNode() {}
func (c *OutputCommand) String() string {
	if c.Selector != "" {
		return fmt.Sprintf("%s %s %s", c.Type, c.Selector, c.Filename)
	}
	return fmt.Sprintf("%s %s", c.Type, c.Filename)
}

// JavaScriptCommand represents JavaScript execution.
type JavaScriptCommand struct {
	Code     string // inline code
	Filename string // or filename
	Variable string // optional variable to store result
}

func (c *JavaScriptCommand) commandNode() {}
func (c *JavaScriptCommand) String() string {
	if c.Filename != "" {
		if c.Variable != "" {
			return fmt.Sprintf("js %s as %s", c.Filename, c.Variable)
		}
		return fmt.Sprintf("js %s", c.Filename)
	}
	if c.Variable != "" {
		return fmt.Sprintf("js { ... } as %s", c.Variable)
	}
	return "js { ... }"
}

// ControlFlowCommand represents control flow (if, for, include).
type ControlFlowCommand struct {
	Type      string   // if, for, include
	Condition string   // for if
	Variable  string   // for for
	List      []string // for for
	Filename  string   // for include
	Body      []Command
}

func (c *ControlFlowCommand) commandNode() {}
func (c *ControlFlowCommand) String() string {
	switch c.Type {
	case "if":
		return fmt.Sprintf("if %s { ... }", c.Condition)
	case "for":
		return fmt.Sprintf("for %s in %v { ... }", c.Variable, c.List)
	case "include":
		return fmt.Sprintf("include %s", c.Filename)
	}
	return "control"
}

// DebugCommand represents debugging commands.
type DebugCommand struct {
	Type    string // devtools, breakpoint, log, debug
	Message string // for log, debug
}

func (c *DebugCommand) commandNode() {}
func (c *DebugCommand) String() string {
	if c.Message != "" {
		return fmt.Sprintf("%s %s", c.Type, c.Message)
	}
	return c.Type
}

// CompareCommand represents visual comparison.
type CompareCommand struct {
	Current  string
	Baseline string
}

func (c *CompareCommand) commandNode() {}
func (c *CompareCommand) String() string {
	return fmt.Sprintf("compare %s with %s", c.Current, c.Baseline)
}
