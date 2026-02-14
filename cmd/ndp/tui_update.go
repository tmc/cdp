package main

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			input := m.textInput.Value()
			if input != "" {
				m.logs = append(m.logs, "> "+input)
				m.textInput.SetValue("")

				// Return a Cmd that executes the function
				cmd := func() tea.Msg {
					// We ignore error here as it should be printed to repl.out -> LogMsg
					_ = m.repl.processCommand(context.Background(), input)
					return nil // Or a specific CommandFinishedMsg
				}

				return m, cmd
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 3 // input + border
		m.textInput.Width = msg.Width
		m.ready = true

	case LogMsg:
		// Clean up newline since we append to list
		line := strings.TrimRight(string(msg), "\n")
		m.logs = append(m.logs, line)
		m.viewport.SetContent(strings.Join(m.logs, "\n"))
		m.viewport.GotoBottom()
	}

	m.textInput, tiCmd = m.textInput.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)

	return m, tea.Batch(tiCmd, vpCmd)
}
