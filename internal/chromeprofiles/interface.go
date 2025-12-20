package chromeprofiles

// ProfileManager handles Chrome profile operations
type ProfileManager interface {
	ListProfiles() ([]string, error)
	SetupWorkdir() error
	Cleanup() error
	CopyProfile(name string, cookieDomains []string) error
	CopyProfileFromDir(srcDir string, cookieDomains []string) error
	WorkDir() string
	// BraveSessionIsolation creates a unique isolated profile for Brave
	BraveSessionIsolation(name string, cookieDomains []string) error
}
