package cdpscripttest

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/chromedp/chromedp"
	"rsc.io/script"
	"rsc.io/script/scripttest"
)

// DefaultConds returns the full condition set for use inside go test.
//
// It includes scripttest's defaults (GOOS, GOARCH, compiler, root, exec:...,
// short, verbose) plus:
//
//   - headless            — running Chrome in headless mode
//   - element:<selector>  — querySelector returns non-null (instant, no waiting)
//   - title:<glob>        — current page title matches a glob pattern
//   - stdout:<pattern>    — last command's stdout matches a regexp pattern
//
// Do NOT call this outside of a test binary — scripttest.DefaultConds calls
// testing.Short() which panics if called before testing.Init.
func DefaultConds() map[string]script.Cond {
	conds := scripttest.DefaultConds()
	addCDPConds(conds)
	return conds
}

// CLIConds returns a condition set safe for use outside of go test (e.g. the
// cdpscripttest CLI binary). It includes script.DefaultConds (GOOS, GOARCH,
// compiler, root), exec: prefix, and the CDP conditions, but omits
// testing.Short / testing.Verbose which panic outside a test binary.
func CLIConds() map[string]script.Cond {
	conds := script.DefaultConds()
	// Add exec: prefix condition — same logic as scripttest.CachedExec.
	conds["exec"] = script.CachedCondition(
		"<suffix> names an executable in PATH",
		func(name string) (bool, error) {
			_, err := exec.LookPath(name)
			return err == nil, nil
		},
	)
	addCDPConds(conds)
	return conds
}

// addCDPConds registers the CDP-aware conditions into conds in place.
func addCDPConds(conds map[string]script.Cond) {
	conds["headless"] = script.Condition(
		"running Chrome in headless mode",
		func(s *script.State) (bool, error) {
			cs, err := cdpState(s)
			if err != nil {
				return false, nil
			}
			return cs.headless, nil
		},
	)

	// element:<selector> — instant non-waiting querySelector check.
	conds["element"] = script.PrefixCondition(
		"<selector> is present in the DOM (non-waiting querySelector)",
		func(s *script.State, sel string) (bool, error) {
			cs, err := cdpState(s)
			if err != nil {
				return false, nil
			}
			var found bool
			err = chromedp.Run(cs.cdpCtx,
				chromedp.Evaluate(fmt.Sprintf(`!!document.querySelector(%q)`, sel), &found),
			)
			if err != nil {
				return false, nil
			}
			return found, nil
		},
	)

	// title:<glob> — matches current page title against a glob pattern.
	conds["title"] = script.PrefixCondition(
		"current page title matches <glob>",
		func(s *script.State, glob string) (bool, error) {
			cs, err := cdpState(s)
			if err != nil {
				return false, nil
			}
			var title string
			if err := chromedp.Run(cs.cdpCtx, chromedp.Title(&title)); err != nil {
				return false, nil
			}
			matched, err := filepath.Match(glob, title)
			if err != nil {
				return false, fmt.Errorf("title condition: bad glob %q: %w", glob, err)
			}
			return matched, nil
		},
	)

	// stdout:<pattern> — true if the last command's stdout matches the regexp.
	// Useful for branching on command output without a separate assertion step:
	//   eval '...'
	//   [stdout skip] skip 'skipping'
	conds["stdout"] = script.PrefixCondition(
		"last command stdout matches <regexp>",
		func(s *script.State, pattern string) (bool, error) {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return false, fmt.Errorf("stdout condition: bad regexp %q: %w", pattern, err)
			}
			return re.MatchString(s.Stdout()), nil
		},
	)

	// WebRTC conditions.
	for k, v := range WebRTCConds() {
		conds[k] = v
	}
}
