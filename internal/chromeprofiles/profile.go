package chromeprofiles

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/tmc/misc/chrome-to-har/internal/secureio"
	"github.com/tmc/misc/chrome-to-har/internal/validation"
	_ "modernc.org/sqlite"
)

type profileManager struct {
	baseDir string
	workDir string
	verbose bool
}

// NewProfileManager creates a new profile manager with the given options
func NewProfileManager(opts ...Option) (*profileManager, error) {
	baseDir, err := getChromeProfileDir()
	if err != nil {
		return nil, err
	}
	pm := &profileManager{
		baseDir: baseDir,
	}
	for _, opt := range opts {
		opt(pm)
	}
	return pm, nil
}

func (pm *profileManager) logf(format string, args ...interface{}) {
	if pm.verbose {
		log.Printf(format, args...)
	}
}

func (pm *profileManager) SetupWorkdir() error {
	dir, err := secureio.CreateSecureTempDir("chrome-to-har-")
	if err != nil {
		return fmt.Errorf("failed to create secure temporary directory: %w", err)
	}
	pm.workDir = dir
	pm.logf("Created secure temporary working directory: %s", dir)
	return nil
}

func (pm *profileManager) Cleanup() error {
	if pm.workDir != "" {
		pm.logf("Cleaning up working directory: %s", pm.workDir)
		return secureio.SecureRemoveAll(pm.workDir)
	}
	return nil
}

// WorkDir returns the current working directory
func (pm *profileManager) WorkDir() string {
	return pm.workDir
}

func (pm *profileManager) ListProfiles() ([]string, error) {
	pm.logf("Listing profiles from base directory: %s", pm.baseDir)

	// Detect browser type from baseDir path
	browserType := detectBrowserType(pm.baseDir)
	pm.logf("Detected browser type: %s", browserType)

	entries, err := os.ReadDir(pm.baseDir)
	if err != nil {
		return nil, withField(fileOpError("read", pm.baseDir, err), "operation", "list_profiles")
	}

	var profiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			profilePath := filepath.Join(pm.baseDir, entry.Name())
			if isValidProfile(profilePath) {
				profiles = append(profiles, entry.Name())
				pm.logf("Found valid %s profile: %s", browserType, entry.Name())
			} else {
				pm.logf("Skipped invalid %s profile: %s (no readable indicator files)", browserType, entry.Name())
			}
		}
	}
	pm.logf("Total valid %s profiles found: %d", browserType, len(profiles))
	return profiles, nil
}

func (pm *profileManager) CopyProfile(name string, cookieDomains []string) error {
	if pm.workDir == "" {
		return profileSetup("working directory not set up")
	}

	// Validate profile name for security
	if err := validation.ValidateProfileName(name); err != nil {
		return withField(wrapProfileNotFound(err, "invalid profile name"), "profile", name)
	}

	srcDir := filepath.Join(pm.baseDir, name)
	return pm.copyProfileImpl(srcDir, name, cookieDomains)
}

// CopyProfileFromDir copies a profile from a custom directory path
// This method supports custom/non-standard profile locations
// Useful for testing, non-standard installations, and multiple profile locations
func (pm *profileManager) CopyProfileFromDir(srcDir string, cookieDomains []string) error {
	if pm.workDir == "" {
		return profileSetup("working directory not set up")
	}

	pm.logf("Copying profile from custom directory: %s", srcDir)

	// Validate the custom path
	if srcDir == "" {
		return profileNotFound("profile directory path cannot be empty")
	}

	// Check if path is absolute
	if !filepath.IsAbs(srcDir) {
		pm.logf("Converting relative path to absolute: %s", srcDir)
		absPath, err := filepath.Abs(srcDir)
		if err != nil {
			return withField(wrapProfileSetup(err, "failed to resolve profile directory path"), "path", srcDir)
		}
		srcDir = absPath
	}

	// Use the directory name as the profile identifier for logging
	profileName := filepath.Base(srcDir)
	return pm.copyProfileImpl(srcDir, profileName, cookieDomains)
}

// copyProfileImpl is the implementation for copying profiles from any source directory
func (pm *profileManager) copyProfileImpl(srcDir, profileName string, cookieDomains []string) error {
	pm.logf("Validating profile at: %s", srcDir)
	if !isValidProfile(srcDir) {
		pm.logf("Profile validation failed for: %s", profileName)
		// Check if directory exists to provide better error message
		if info, err := os.Stat(srcDir); err != nil {
			browserType := detectBrowserType(pm.baseDir)
			errorMsg := fmt.Sprintf("%s profile directory '%s' not found", browserType, profileName)
			return withField(profileNotFound(errorMsg), "profile", profileName)
		} else if !info.IsDir() {
			return withField(profileNotFound("profile path is not a directory"), "profile", profileName)
		}
		// Profile directory exists but lacks required files
		browserType := detectBrowserType(pm.baseDir)
		errorMsg := fmt.Sprintf("%s profile '%s' does not contain readable profile data files (Preferences, History, or Cookies). This may indicate a corrupted profile. Location: %s", browserType, profileName, srcDir)
		return withField(profileNotFound(errorMsg), "profile", profileName)
	}
	browserType := detectBrowserType(pm.baseDir)
	pm.logf("Profile validation successful for %s profile: %s", browserType, profileName)

	dstDir := filepath.Join(pm.workDir, "Default")
	if err := os.MkdirAll(dstDir, secureio.SecureDirPerms); err != nil {
		return withField(fileOpError("create", dstDir, err), "profile", profileName)
	}

	pm.logf("Copying profile from %s to %s", srcDir, dstDir)

	// Handle cookies with domain filtering
	if len(cookieDomains) > 0 {
		pm.logf("Filtering cookies for domains: %v", cookieDomains)
		if err := pm.CopyCookiesWithDomains(srcDir, dstDir, cookieDomains); err != nil {
			return withField(profileCopy("failed to copy cookies with domain filtering", err), "profile", profileName)
		}
	} else {
		if err := copyFile(filepath.Join(srcDir, "Cookies"), filepath.Join(dstDir, "Cookies")); err != nil {
			if !os.IsNotExist(err) {
				return withField(fileOpError("copy", filepath.Join(srcDir, "Cookies"), err), "profile", profileName)
			}
		}
	}

	// Essential profile components
	essentials := map[string]bool{
		"Cookies":                  false, // Session cookies for authentication
		"Login Data":               false,
		"Web Data":                 false,
		"Preferences":              false,
		"Bookmarks":                false,
		"History":                  false,
		"Favicons":                 false,
		"Network Action Predictor": false,
		"Network Persistent State": false,
		"Extension Cookies":        false,
		"Local Storage":            true,
		"IndexedDB":                true,
		"Session Storage":          true,
	}

	for name, isDir := range essentials {
		src := filepath.Join(srcDir, name)
		dst := filepath.Join(dstDir, name)

		if isDir {
			if err := copyDir(src, dst); err != nil {
				if !os.IsNotExist(err) {
					pm.logf("Warning: error copying directory %s: %v", name, err)
				}
			} else {
				pm.logf("Copied directory: %s", name)
			}
		} else {
			if err := copyFile(src, dst); err != nil {
				if !os.IsNotExist(err) {
					pm.logf("Warning: error copying file %s: %v", name, err)
				}
			} else {
				pm.logf("Copied file: %s", name)
			}
		}
	}

	// Copy Local State file (contains encryption keys for cookies)
	srcLocalState := filepath.Join(pm.baseDir, "Local State")
	dstLocalState := filepath.Join(pm.workDir, "Local State")
	if err := copyFile(srcLocalState, dstLocalState); err != nil {
		if os.IsNotExist(err) {
			// If Local State doesn't exist, create a minimal one
			pm.logf("Local State not found, creating minimal version")
			localState := `{"os_crypt":{"encrypted_key":""}}`
			if err := os.WriteFile(dstLocalState, []byte(localState), 0644); err != nil {
				return withField(fileOpError("write", dstLocalState, err), "profile", profileName)
			}
			pm.logf("Created minimal Local State file for profile: %s", profileName)
		} else {
			return withField(fileOpError("copy", srcLocalState, err), "profile", profileName)
		}
	} else {
		pm.logf("Copied Local State file (with encryption keys) for profile: %s", profileName)
	}

	return nil
}

func (pm *profileManager) CopyCookiesWithDomains(srcDir, dstDir string, domains []string) error {
	srcDB := filepath.Join(srcDir, "Cookies")
	dstDB := filepath.Join(dstDir, "Cookies")

	// Open source database
	src, err := sql.Open("sqlite", srcDB+"?mode=ro")
	if err != nil {
		return withField(profileCopy("failed to open source cookies database", err), "database", srcDB)
	}
	defer src.Close()

	// Create destination database
	if err := copyFile(srcDB, dstDB); err != nil {
		return withField(fileOpError("copy", srcDB, err), "operation", "copy_cookies_database")
	}

	dst, err := sql.Open("sqlite", dstDB)
	if err != nil {
		return withField(profileCopy("failed to open destination cookies database", err), "database", dstDB)
	}
	defer dst.Close()

	// Begin transaction
	tx, err := dst.Begin()
	if err != nil {
		return profileCopy("failed to begin database transaction", err)
	}
	defer tx.Rollback()

	// Delete cookies that don't match domains
	var whereClause strings.Builder
	whereClause.WriteString("host_key NOT LIKE '%")
	whereClause.WriteString(strings.Join(domains, "%' AND host_key NOT LIKE '%"))
	whereClause.WriteString("%'")

	_, err = tx.Exec("DELETE FROM cookies WHERE " + whereClause.String())
	if err != nil {
		return withField(profileCopy("failed to filter cookies by domain", err), "domains", domains)
	}

	if err := tx.Commit(); err != nil {
		return profileCopy("failed to commit database changes", err)
	}

	pm.logf("Copied and filtered cookies for domains: %v", domains)
	return nil
}

// getChromeProfileDir detects and returns the base directory for browser profiles
// This function searches for Chrome, Brave, Chromium, and Edge profiles
// Brave profiles are searched FIRST on all platforms for priority detection
func getChromeProfileDir() (string, error) {
	var candidates []string

	switch runtime.GOOS {
	case "windows":
		// Brave first, then Chrome (for priority Brave detection)
		candidates = []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "BraveSoftware", "Brave-Browser"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "User Data"),
		}
	case "darwin":
		// Brave first, then Chrome (for priority Brave detection)
		// This ensures Brave is preferred if both are installed
		candidates = []string{
			filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "BraveSoftware", "Brave-Browser"),
			filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "Google", "Chrome"),
		}
	case "linux":
		// Brave first, then Chrome (for priority Brave detection)
		candidates = []string{
			filepath.Join(os.Getenv("HOME"), ".config", "BraveSoftware", "Brave-Browser"),
			filepath.Join(os.Getenv("HOME"), ".config", "google-chrome"),
		}
	default:
		return "", withField(configurationError("unsupported operating system"), "os", runtime.GOOS)
	}

	// Return first existing directory with valid profiles
	// Only check directories that match known profile name patterns
	profilePatterns := []string{"Default", "Profile ", "System Profile", "Guest Profile"}

	for _, dir := range candidates {
		if _, err := os.Stat(dir); err == nil {
			// Check if it has at least one valid profile
			entries, err := os.ReadDir(dir)
			if err != nil {
				// Directory exists but cannot be read - continue to next candidate
				continue
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				// Only check directories with known profile name patterns
				name := entry.Name()
				isProfileDir := false
				for _, pattern := range profilePatterns {
					if name == pattern || strings.HasPrefix(name, pattern) {
						isProfileDir = true
						break
					}
				}
				if !isProfileDir {
					// Skip system directories, caches, etc.
					continue
				}

				profilePath := filepath.Join(dir, name)
				if isValidProfile(profilePath) {
					// Found valid profile directory, return this base directory
					// This works for Brave, Chrome, Chromium, Edge, and other Chromium-based browsers
					return dir, nil
				}
			}
		}
	}

	// If no valid profiles found, return first existing directory
	// This allows profile manager to work with directories that have profiles
	// but may have permission issues or different structures
	for _, dir := range candidates {
		if _, err := os.Stat(dir); err == nil {
			return dir, nil
		}
	}

	// Return first candidate as fallback
	if len(candidates) > 0 {
		return candidates[0], nil
	}

	return "", configurationError("no browser profile directories found")
}

// detectBrowserType identifies the browser type from the profile directory path
func detectBrowserType(profileDir string) string {
	if strings.Contains(profileDir, "BraveSoftware") {
		return "Brave"
	}
	if strings.Contains(profileDir, "Google/Chrome") || strings.Contains(profileDir, "google-chrome") {
		return "Chrome"
	}
	if strings.Contains(profileDir, "chromium") || strings.Contains(profileDir, "Chromium") {
		return "Chromium"
	}
	if strings.Contains(profileDir, "Edge") || strings.Contains(profileDir, "microsoft-edge") {
		return "Edge"
	}
	return "Chromium-based"
}

// isValidProfile checks if a directory contains valid profile indicator files
// This function supports Chrome, Brave, Chromium, and Edge profile structures
func isValidProfile(dir string) bool {
	// Check if directory exists and is readable
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}

	// Check for at least one indicator file that exists and is readable
	// Support both Chrome and Brave profile structures (identical indicator files)
	// Brave profiles contain the same files as Chrome, just in different locations
	indicators := []string{"Preferences", "History", "Cookies"}
	foundCount := 0
	var foundFiles []string

	for _, indicator := range indicators {
		indicatorPath := filepath.Join(dir, indicator)
		if file, err := os.Open(indicatorPath); err == nil {
			file.Close()
			foundCount++
			foundFiles = append(foundFiles, indicator)
		}
	}

	// Consider valid if at least one indicator file is found and readable
	// This works for Chrome, Brave, Chromium, Edge, and other Chromium-based browsers
	return foundCount > 0
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	if err != nil {
		return err
	}

	info, err := source.Stat()
	if err != nil {
		return err
	}

	return os.Chmod(dst, info.Mode())
}

func copyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, info.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// BraveSessionIsolation creates a unique isolated profile for Brave to avoid session reuse issues.
// This method is specifically designed to handle Brave's session reuse behavior by creating
// a temporary profile directory with a unique timestamp suffix, preventing Brave from
// attempting to reuse an existing browser session.
func (pm *profileManager) BraveSessionIsolation(name string, cookieDomains []string) error {
	if pm.workDir == "" {
		return profileSetup("working directory not set up")
	}

	// Validate profile name for security
	if err := validation.ValidateProfileName(name); err != nil {
		return withField(wrapProfileNotFound(err, "invalid profile name"), "profile", name)
	}

	srcDir := filepath.Join(pm.baseDir, name)
	pm.logf("Validating profile at: %s for Brave session isolation", srcDir)
	if !isValidProfile(srcDir) {
		pm.logf("Profile validation failed for: %s", name)
		return withField(profileNotFound("profile directory does not contain readable indicator files"), "profile", name)
	}

	// Create unique isolated profile directory with timestamp
	// This prevents Brave's session reuse by making each launch use a different path
	isolatedDir := filepath.Join(pm.workDir, fmt.Sprintf("Profile-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(isolatedDir, secureio.SecureDirPerms); err != nil {
		return withField(fileOpError("create", isolatedDir, err), "profile", name)
	}

	pm.logf("Created isolated profile directory: %s", isolatedDir)

	// Use isolated directory as the target
	originalWorkDir := pm.workDir
	pm.workDir = isolatedDir

	// Copy profile to isolated directory
	if err := pm.copyProfileToDir(srcDir, isolatedDir, name, cookieDomains); err != nil {
		pm.workDir = originalWorkDir
		return err
	}

	pm.logf("Successfully created Brave isolated profile session")
	return nil
}

// copyProfileToDir is a helper method that copies profile to a specific directory.
func (pm *profileManager) copyProfileToDir(srcDir, dstDir, profileName string, cookieDomains []string) error {
	dstProfileDir := filepath.Join(dstDir, "Default")
	if err := os.MkdirAll(dstProfileDir, secureio.SecureDirPerms); err != nil {
		return withField(fileOpError("create", dstProfileDir, err), "profile", profileName)
	}

	pm.logf("Copying profile from %s to %s", srcDir, dstProfileDir)

	// Handle cookies with domain filtering
	if len(cookieDomains) > 0 {
		pm.logf("Filtering cookies for domains: %v", cookieDomains)
		if err := pm.CopyCookiesWithDomains(srcDir, dstProfileDir, cookieDomains); err != nil {
			return withField(profileCopy("failed to copy cookies with domain filtering", err), "profile", profileName)
		}
	} else {
		if err := copyFile(filepath.Join(srcDir, "Cookies"), filepath.Join(dstProfileDir, "Cookies")); err != nil {
			if !os.IsNotExist(err) {
				return withField(fileOpError("copy", filepath.Join(srcDir, "Cookies"), err), "profile", profileName)
			}
		}
	}

	// Essential profile components
	essentials := map[string]bool{
		"Cookies":                  false, // Session cookies for authentication
		"Login Data":               false,
		"Web Data":                 false,
		"Preferences":              false,
		"Bookmarks":                false,
		"History":                  false,
		"Favicons":                 false,
		"Network Action Predictor": false,
		"Network Persistent State": false,
		"Extension Cookies":        false,
		"Local Storage":            true,
		"IndexedDB":                true,
		"Session Storage":          true,
	}

	for name, isDir := range essentials {
		src := filepath.Join(srcDir, name)
		dst := filepath.Join(dstProfileDir, name)

		if isDir {
			if err := copyDir(src, dst); err != nil {
				if !os.IsNotExist(err) {
					pm.logf("Warning: error copying directory %s: %v", name, err)
				}
			} else {
				pm.logf("Copied directory: %s", name)
			}
		} else {
			if err := copyFile(src, dst); err != nil {
				if !os.IsNotExist(err) {
					pm.logf("Warning: error copying file %s: %v", name, err)
				}
			} else {
				pm.logf("Copied file: %s", name)
			}
		}
	}

	// Copy Local State file (contains encryption keys for cookies)
	srcLocalState := filepath.Join(pm.baseDir, "Local State")
	dstLocalState := filepath.Join(dstDir, "Local State")
	if err := copyFile(srcLocalState, dstLocalState); err != nil {
		if os.IsNotExist(err) {
			// If Local State doesn't exist, create a minimal one
			pm.logf("Local State not found, creating minimal version")
			localState := `{"os_crypt":{"encrypted_key":""}}`
			if err := os.WriteFile(dstLocalState, []byte(localState), 0644); err != nil {
				return withField(fileOpError("write", dstLocalState, err), "profile", profileName)
			}
			pm.logf("Created minimal Local State file in isolated directory")
		} else {
			return withField(fileOpError("copy", srcLocalState, err), "profile", profileName)
		}
	} else {
		pm.logf("Copied Local State file (with encryption keys) to isolated directory")
	}

	return nil
}
