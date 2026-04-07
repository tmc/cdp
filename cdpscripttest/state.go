package cdpscripttest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"rsc.io/script"
)

// DefaultWaitTimeout is the default timeout for wait commands
// (waitVisible, waitNotVisible) when no per-command or per-script
// override is specified.
const DefaultWaitTimeout = 10 * time.Second

// artifactDirForTB returns the artifact directory for saving screenshots.
// Go 1.26 added ArtifactDir to testing.TB directly.
func artifactDirForTB(t testing.TB) string {
	return t.ArtifactDir()
}

// stateKey is the context value key used to thread *State through script commands.
type stateKey struct{}

// withState stores cs in ctx so that CDP commands can retrieve it via cdpState.
func withState(ctx context.Context, cs *State) context.Context {
	return context.WithValue(ctx, stateKey{}, cs)
}

// cdpState extracts *State from the script.State's context.
// Every command uses this to reach the CDP context and base URL.
func cdpState(s *script.State) (*State, error) {
	v := s.Context().Value(stateKey{})
	if v == nil {
		return nil, fmt.Errorf("cdpscripttest: no CDP state in context; use cdpscripttest.Run or cdpscripttest.Test")
	}
	return v.(*State), nil
}

// CDPState extracts the *State from a script.State's context.
// Use this in custom commands registered outside the package to access
// the chromedp context, base URL, and other CDP state.
func CDPState(s *script.State) (*State, error) { return cdpState(s) }

// State extends script.State with CDP-specific fields.
//
// The embedded *script.State provides environment variables, working directory,
// and stdout/stderr buffering. The CDP fields layer on top: a chromedp context
// for browser interaction, a base URL prepended to relative paths, and a
// directory for screenshots.
type State struct {
	*script.State

	// cdpCtx is the chromedp browser context for this test.
	cdpCtx context.Context

	// baseURL is prepended to paths passed to navigate.
	baseURL string

	// screenshotDir is where unnamed screenshots are saved.
	screenshotDir string

	// headless records whether Chrome is running headless.
	headless bool

	// artifactDirFn returns the directory to save screenshots and baselines.
	// Set at construction time so the CLI can use its own directory and
	// tests can use t.ArtifactDir() (Go 1.26+) or t.TempDir() as a fallback.
	// screenshot-compare reads and writes baselines from this same directory.
	artifactDirFn func() string

	// updateGolden, when true, causes screenshot-compare to overwrite
	// baselines instead of comparing against them.
	updateGolden bool

	// injectedScripts tracks script identifiers added via inject, so
	// inject-clear can remove them.
	injectedScripts []page.ScriptIdentifier

	// skipBlur disables --blur processing so screenshots show raw content.
	skipBlur bool

	// emitUnblurred saves an additional unblurred copy next to each
	// blurred screenshot for side-by-side comparison.
	emitUnblurred bool

	// waitTimeout is the default timeout for wait commands (waitVisible,
	// waitNotVisible). Set via the "timeout" script command or Go API.
	// Zero means use DefaultWaitTimeout.
	waitTimeout time.Duration

	// rtcInjected is true after the WebRTC monitoring script has been injected.
	rtcInjected bool

	// rtcSelectedPeer is the stable ID of the currently selected peer
	// connection for rtc-* commands. Default 0.
	rtcSelectedPeer int
}

// NewState creates a State for a single script run.
//
// t is the testing.TB for this run; it is used by screenshot commands to
// obtain the artifact directory (ArtifactDir on Go 1.26+, TempDir otherwise).
// cdpCtx must be a chromedp browser context (obtained from chromedp.NewContext).
// workdir is the test's temporary working directory (t.TempDir()).
// baseURL is prepended to path arguments of the navigate command.
// env is the initial environment; nil uses os.Environ().
//
// The returned State embeds a *script.State whose internal context carries the
// *State as a value, so CDP commands can retrieve it via cdpState.
func NewState(t testing.TB, cdpCtx context.Context, workdir, baseURL string, env []string) (*State, error) {
	return newState(cdpCtx, workdir, baseURL, env, func() string { return artifactDirForTB(t) })
}

// NewStateWithArtifactDir is like NewState but accepts an explicit artifact
// directory instead of a testing.TB. Useful for the CLI runner.
func NewStateWithArtifactDir(cdpCtx context.Context, workdir, baseURL, artifactDir string, env []string) (*State, error) {
	fn := func() string { return artifactDir }
	if artifactDir == "" {
		fn = func() string {
			dir := filepath.Join(workdir, "screenshots")
			_ = os.MkdirAll(dir, 0o777)
			return dir
		}
	}
	return newState(cdpCtx, workdir, baseURL, env, fn)
}

func newState(cdpCtx context.Context, workdir, baseURL string, env []string, artifactDirFn func() string) (*State, error) {
	// Allow UPDATE_GOLDEN=1 (or any non-empty value) to enable golden update mode
	// without modifying callers. Useful for: UPDATE_GOLDEN=1 go test ./...
	updateGolden := os.Getenv("UPDATE_GOLDEN") != ""

	screenshotDir := filepath.Join(workdir, "screenshots")
	if err := os.MkdirAll(screenshotDir, 0o777); err != nil {
		return nil, fmt.Errorf("cdpscripttest: mkdir screenshots: %w", err)
	}

	cs := &State{
		cdpCtx:        cdpCtx,
		baseURL:       baseURL,
		screenshotDir: screenshotDir,
		artifactDirFn: artifactDirFn,
		updateGolden:  updateGolden,
	}

	// Inject *State into the context that script.State will use internally.
	// Commands call s.Context() and retrieve the *State from it.
	ctxWithState := withState(cdpCtx, cs)

	ss, err := script.NewState(ctxWithState, workdir, env)
	if err != nil {
		return nil, fmt.Errorf("cdpscripttest: new state: %w", err)
	}
	cs.State = ss

	// Expose base URL and screenshot dir to scripts as environment variables.
	if err := cs.Setenv("BASE_URL", baseURL); err != nil {
		return nil, err
	}
	if err := cs.Setenv("SCREENSHOT_DIR", screenshotDir); err != nil {
		return nil, err
	}

	// Detect headless mode: navigator.webdriver is true in headless Chrome.
	var isHeadless bool
	_ = chromedp.Run(cdpCtx, chromedp.Evaluate(`navigator.webdriver === true`, &isHeadless))
	cs.headless = isHeadless

	return cs, nil
}

// CDPCtx returns the chromedp context attached to this state.
func (s *State) CDPCtx() context.Context { return s.cdpCtx }

// BaseURL returns the base URL for navigate commands.
func (s *State) BaseURL() string { return s.baseURL }

// SetBaseURL changes the base URL for subsequent navigate commands.
func (s *State) SetBaseURL(u string) error {
	s.baseURL = u
	return s.Setenv("BASE_URL", u)
}

// Headless reports whether Chrome is running in headless mode.
func (s *State) Headless() bool { return s.headless }

// ScreenshotDir returns the directory where unnamed screenshots are saved.
func (s *State) ScreenshotDir() string { return s.screenshotDir }

// SetUpdateGolden controls whether screenshot-compare overwrites baselines
// instead of comparing against them. Useful for updating golden files.
func (s *State) SetUpdateGolden(v bool) { s.updateGolden = v }

// UpdateGolden reports whether screenshot-compare should overwrite baselines.
func (s *State) UpdateGolden() bool { return s.updateGolden }

// WaitTimeout returns the current wait timeout. Returns DefaultWaitTimeout
// if no custom value has been set.
func (s *State) WaitTimeout() time.Duration {
	if s.waitTimeout > 0 {
		return s.waitTimeout
	}
	return DefaultWaitTimeout
}

// SetWaitTimeout sets the default timeout for wait commands.
func (s *State) SetWaitTimeout(d time.Duration) { s.waitTimeout = d }

// RTCInjected reports whether the WebRTC monitoring script has been injected.
func (s *State) RTCInjected() bool { return s.rtcInjected }

// SetRTCInjected records whether the WebRTC monitoring script has been injected.
func (s *State) SetRTCInjected(v bool) { s.rtcInjected = v }

// RTCSelectedPeer returns the stable ID of the currently selected peer connection.
func (s *State) RTCSelectedPeer() int { return s.rtcSelectedPeer }

// SetRTCSelectedPeer sets the currently selected peer connection by stable ID.
func (s *State) SetRTCSelectedPeer(id int) { s.rtcSelectedPeer = id }
