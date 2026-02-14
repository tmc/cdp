package main

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// LogMsg is a message carrying a log line
type LogMsg string

// TUIModel holds the state for the bubbletea UI
type TUIModel struct {
	repl  *REPL
	ready bool

	// Components
	viewport  viewport.Model
	textInput textinput.Model

	// State
	logs   []string
	err    error
	width  int
	height int

	// Program reference for writing? No, Writer handles that.
}

func (m TUIModel) Init() tea.Cmd {
	return textinput.Blink
}

func NewTUIModel(repl *REPL) TUIModel {
	ti := textinput.New()
	ti.Placeholder = "Enter command (e.g. ls, vars, bt)..."
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20

	vp := viewport.New(30, 5)
	vp.SetContent("Welcome to NDP TUI\nConnected to target.\n")

	return TUIModel{
		repl:      repl,
		textInput: ti,
		viewport:  vp,
		logs:      []string{"Welcome to NDP TUI"},
	}
}
