package types

import "time"

// Script represents a parsed CDP script with all its components.
type Script struct {
	Metadata    Metadata
	Main        *CommandList
	Helpers     map[string]string    // JavaScript helper files
	TestData    map[string][]byte    // Test data files
	Assertions  []Assertion          // Assertions to validate
	Expected    map[string][]byte    // Expected output files
}

// Metadata represents the metadata section of a CDP script.
type Metadata struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Version     string            `yaml:"version"`
	Author      string            `yaml:"author"`
	Browser     string            `yaml:"browser"`      // chrome, brave, chromium
	Profile     string            `yaml:"profile"`      // browser profile name
	Headless    bool              `yaml:"headless"`     // run in headless mode
	Timeout     time.Duration     `yaml:"timeout"`      // default timeout
	Env         map[string]string `yaml:"env"`          // environment variables
	Imports     []string          `yaml:"imports"`      // imported script files
}

// CommandList represents a list of commands in the script.
type CommandList struct {
	Commands interface{} // Can be []ast.Command or other implementations
}

// ExecutionContext holds the runtime state during script execution.
type ExecutionContext struct {
	Variables map[string]interface{}
	Browser   interface{} // Browser instance (will be *chromedp.Context)
	Timeout   time.Duration
}
