package types

// Assertion represents a test assertion in a CDP script.
type Assertion struct {
	Type     string                 `yaml:"type"` // selector, response, performance, console, variable, visual
	Selector string                 `yaml:"selector,omitempty"`
	Text     string                 `yaml:"text,omitempty"`
	Contains string                 `yaml:"contains,omitempty"`
	Exists   *bool                  `yaml:"exists,omitempty"`
	URL      string                 `yaml:"url,omitempty"`
	Status   int                    `yaml:"status,omitempty"`
	JSON     map[string]interface{} `yaml:"json,omitempty"`
	Metric   string                 `yaml:"metric,omitempty"`
	Max      string                 `yaml:"max,omitempty"`
	Min      string                 `yaml:"min,omitempty"`
	Level    string                 `yaml:"level,omitempty"`
	Count    *int                   `yaml:"count,omitempty"`
	Name     string                 `yaml:"name,omitempty"`
	Length   *int                   `yaml:"length,omitempty"`
	MinLength *int                  `yaml:"minLength,omitempty"`
	MaxLength *int                  `yaml:"maxLength,omitempty"`
	File     string                 `yaml:"file,omitempty"`
	Baseline string                 `yaml:"baseline,omitempty"`
	MaxDiff  float64                `yaml:"maxDiff,omitempty"`
}
