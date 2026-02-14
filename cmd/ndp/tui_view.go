package main

import (
	"fmt"
)

func (m TUIModel) View() string {
	if !m.ready {
		// Initializing...
		return "Initializing TUI..."
	}

	return fmt.Sprintf("%s\n%s",
		m.viewport.View(),
		m.textInput.View(),
	)
}
