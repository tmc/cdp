// Package scrub detects and redacts secrets in text and HAR data.
package scrub

import (
	_ "embed"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

//go:generate sh -c "curl -sL https://raw.githubusercontent.com/gitleaks/gitleaks/master/config/gitleaks.toml > gitleaks.toml"

//go:embed gitleaks.toml
var gitleaksConfig string

// sensitiveHeaders lists header names whose values should be redacted.
var sensitiveHeaders = map[string]bool{
	"authorization":       true,
	"cookie":              true,
	"set-cookie":          true,
	"x-api-key":           true,
	"x-auth-token":        true,
	"x-csrf-token":        true,
	"x-xsrf-token":        true,
	"proxy-authorization": true,
}

// sensitiveParams lists query parameter names whose values should be redacted.
var sensitiveParams = map[string]bool{
	"token":          true,
	"key":            true,
	"api_key":        true,
	"apikey":         true,
	"secret":         true,
	"password":       true,
	"passwd":         true,
	"access_token":   true,
	"refresh_token":  true,
	"client_secret":  true,
	"session_id":     true,
}

type rule struct {
	id       string
	regex    *regexp.Regexp
	keywords []string // lowercase, for pre-filtering
}

// Scrubber detects and redacts secrets in text and HAR data.
type Scrubber struct {
	rules   []rule
	enabled bool
}

type gitleaksFile struct {
	Rules []gitleaksRule `toml:"rules"`
}

type gitleaksRule struct {
	ID       string   `toml:"id"`
	Regex    string   `toml:"regex"`
	Keywords []string `toml:"keywords"`
}

// New creates a scrubber with default patterns (gitleaks + HAR-specific).
func New() *Scrubber {
	s := &Scrubber{enabled: true}
	s.rules = loadGitleaksRules()
	return s
}

func loadGitleaksRules() []rule {
	var cfg gitleaksFile
	if _, err := toml.Decode(gitleaksConfig, &cfg); err != nil {
		log.Printf("scrub: parsing gitleaks.toml: %v; using HAR-specific patterns only", err)
		return nil
	}
	var rules []rule
	var skipped int
	for _, r := range cfg.Rules {
		if r.Regex == "" {
			continue
		}
		re, err := regexp.Compile(r.Regex)
		if err != nil {
			skipped++
			log.Printf("scrub: skipping rule %s: %v", r.ID, err)
			continue
		}
		kw := make([]string, len(r.Keywords))
		for i, k := range r.Keywords {
			kw[i] = strings.ToLower(k)
		}
		rules = append(rules, rule{
			id:       r.ID,
			regex:    re,
			keywords: kw,
		})
	}
	if skipped > 0 {
		log.Printf("scrub: skipped %d rules with unsupported regex features", skipped)
	}
	return rules
}

// Enabled reports whether scrubbing is active.
func (s *Scrubber) Enabled() bool {
	return s.enabled
}

// ScrubText redacts secrets from source text. Returns scrubbed text and count of redactions.
func (s *Scrubber) ScrubText(text string) (string, int) {
	if !s.enabled {
		return text, 0
	}
	count := 0
	lower := strings.ToLower(text)
	for _, r := range s.rules {
		if !keywordMatch(lower, r.keywords) {
			continue
		}
		replacement := fmt.Sprintf("[REDACTED:%s]", r.id)
		result := r.regex.ReplaceAllStringFunc(text, func(match string) string {
			count++
			return replacement
		})
		if count > 0 || result != text {
			text = result
			lower = strings.ToLower(text)
		}
	}
	return text, count
}

// keywordMatch returns true if keywords is empty or at least one keyword
// appears in the lowercased text.
func keywordMatch(lower string, keywords []string) bool {
	if len(keywords) == 0 {
		return true
	}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// ScrubHeaderValue returns "[REDACTED]" if the header name is sensitive,
// otherwise the original value.
func (s *Scrubber) ScrubHeaderValue(name, value string) string {
	if !s.enabled {
		return value
	}
	if sensitiveHeaders[strings.ToLower(name)] {
		return "[REDACTED]"
	}
	return value
}

// ScrubQueryParam returns "[REDACTED]" if the param name is sensitive,
// otherwise the original value.
func (s *Scrubber) ScrubQueryParam(name, value string) string {
	if !s.enabled {
		return value
	}
	if sensitiveParams[strings.ToLower(name)] {
		return "[REDACTED]"
	}
	return value
}
