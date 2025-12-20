// interface.go - Computer interface for different backends (browser, VM, etc.)
package main

// Computer defines the interface for computer control backends.
// Both BrowserComputer and VZComputer implement this interface.
type Computer interface {
	// ScreenSize returns the viewport dimensions
	ScreenSize() (width, height int)

	// ClickAt clicks at normalized coordinates (0-1000 scale)
	ClickAt(x, y int) (*EnvState, error)

	// HoverAt hovers at normalized coordinates (0-1000 scale)
	HoverAt(x, y int) (*EnvState, error)

	// TypeTextAt types text at normalized coordinates
	TypeTextAt(x, y int, text string, pressEnter, clearBefore bool) (*EnvState, error)

	// DragAndDrop performs a drag and drop operation
	DragAndDrop(x, y, destX, destY int) (*EnvState, error)

	// Navigate navigates to a URL
	Navigate(url string) (*EnvState, error)

	// GoBack navigates back in history
	GoBack() (*EnvState, error)

	// GoForward navigates forward in history
	GoForward() (*EnvState, error)

	// Search navigates to search (Google)
	Search() (*EnvState, error)

	// ScrollDocument scrolls the entire page
	ScrollDocument(direction string) (*EnvState, error)

	// ScrollAt scrolls at specific coordinates
	ScrollAt(x, y int, direction string, magnitude float64) (*EnvState, error)

	// KeyCombination presses a key combination
	KeyCombination(keys []string) (*EnvState, error)

	// Wait5Seconds waits for 5 seconds
	Wait5Seconds() (*EnvState, error)

	// CurrentState returns the current state with screenshot
	CurrentState() (*EnvState, error)

	// OpenWebBrowser opens/navigates to URL
	OpenWebBrowser(url string) (*EnvState, error)

	// Close closes the computer/browser
	Close() error
}

// Compile-time interface checks
var _ Computer = (*BrowserComputer)(nil)
var _ Computer = (*VZComputer)(nil)
