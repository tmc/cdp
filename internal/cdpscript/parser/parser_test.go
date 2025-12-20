package parser

import (
	"testing"

	"github.com/tmc/misc/chrome-to-har/internal/cdpscript/ast"
	"github.com/tmc/misc/chrome-to-har/internal/cdpscript/lexer"
)

func TestLexer(t *testing.T) {
	input := "fill #email test@example.com"
	l := lexer.New(input)

	tokens := []lexer.Token{}
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		t.Logf("Token: %s", tok)
		if tok.Type == lexer.EOF {
			break
		}
	}
}

func TestParseGoto(t *testing.T) {
	input := "goto https://example.com"
	l := lexer.New(input)
	p := NewParser(l)
	commands := p.ParseCommands()

	if len(p.Errors()) != 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}

	cmd, ok := commands[0].(*ast.NavigationCommand)
	if !ok {
		t.Fatalf("expected NavigationCommand, got %T", commands[0])
	}

	if cmd.Type != "goto" {
		t.Errorf("expected type 'goto', got '%s'", cmd.Type)
	}

	if cmd.URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got '%s'", cmd.URL)
	}
}

func TestParseWait(t *testing.T) {
	tests := []struct {
		input    string
		waitType string
		value    string
	}{
		{"wait for #content", "for", "#content"},
		{"wait until network idle", "until", "network idle"},
		{"wait 2s", "duration", "2s"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			l := lexer.New(tt.input)
			p := NewParser(l)
			commands := p.ParseCommands()

			if len(p.Errors()) != 0 {
				t.Fatalf("parser errors: %v", p.Errors())
			}

			if len(commands) != 1 {
				t.Fatalf("expected 1 command, got %d", len(commands))
			}

			cmd, ok := commands[0].(*ast.WaitCommand)
			if !ok {
				t.Fatalf("expected WaitCommand, got %T", commands[0])
			}

			if cmd.Type != tt.waitType {
				t.Errorf("expected type '%s', got '%s'", tt.waitType, cmd.Type)
			}
		})
	}
}

func TestParseInteraction(t *testing.T) {
	tests := []struct {
		input string
		typ   string
	}{
		{"click button", "click"},
		{"hover .menu", "hover"},
		{"fill #email test@example.com", "fill"},
		{"type #search hello world", "type"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			l := lexer.New(tt.input)
			p := NewParser(l)
			commands := p.ParseCommands()

			if len(p.Errors()) != 0 {
				t.Fatalf("parser errors: %v", p.Errors())
			}

			if len(commands) != 1 {
				t.Fatalf("expected 1 command, got %d", len(commands))
			}

			cmd, ok := commands[0].(*ast.InteractionCommand)
			if !ok {
				t.Fatalf("expected InteractionCommand, got %T", commands[0])
			}

			if cmd.Type != tt.typ {
				t.Errorf("expected type '%s', got '%s'", tt.typ, cmd.Type)
			}
		})
	}
}

func TestParseMultipleCommands(t *testing.T) {
	input := `goto https://example.com
wait for #content
click button
screenshot page.png`

	l := lexer.New(input)
	p := NewParser(l)
	commands := p.ParseCommands()

	if len(p.Errors()) != 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	if len(commands) != 4 {
		t.Fatalf("expected 4 commands, got %d", len(commands))
		for i, cmd := range commands {
			t.Logf("Command %d: %T - %s", i, cmd, cmd)
		}
	}

	// Verify command types
	if _, ok := commands[0].(*ast.NavigationCommand); !ok {
		t.Errorf("command 0: expected NavigationCommand, got %T", commands[0])
	}
	if _, ok := commands[1].(*ast.WaitCommand); !ok {
		t.Errorf("command 1: expected WaitCommand, got %T", commands[1])
	}
	if _, ok := commands[2].(*ast.InteractionCommand); !ok {
		t.Errorf("command 2: expected InteractionCommand, got %T", commands[2])
	}
	if _, ok := commands[3].(*ast.OutputCommand); !ok {
		t.Errorf("command 3: expected OutputCommand, got %T", commands[3])
	}
}

func TestParseWithComments(t *testing.T) {
	input := `# This is a test script
goto https://example.com
# Wait for content
wait for #content`

	l := lexer.New(input)
	p := NewParser(l)
	commands := p.ParseCommands()

	if len(p.Errors()) != 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	// Comments should be skipped
	if len(commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(commands))
	}
}
