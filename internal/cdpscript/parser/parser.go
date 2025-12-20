package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tmc/misc/chrome-to-har/internal/cdpscript/ast"
	"github.com/tmc/misc/chrome-to-har/internal/cdpscript/lexer"
)

// Parser parses CDP script commands.
type Parser struct {
	l      *lexer.Lexer
	errors []string

	curToken  lexer.Token
	peekToken lexer.Token
}

// New creates a new Parser.
func NewParser(l *lexer.Lexer) *Parser {
	p := &Parser{
		l:      l,
		errors: []string{},
	}

	// Read two tokens to initialize curToken and peekToken
	p.nextToken()
	p.nextToken()

	return p
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

// Errors returns the parsing errors.
func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) error(msg string) {
	p.errors = append(p.errors, fmt.Sprintf("[%d:%d] %s", p.curToken.Line, p.curToken.Column, msg))
}

// ParseCommands parses a list of commands from the script.
func (p *Parser) ParseCommands() []ast.Command {
	commands := []ast.Command{}

	for p.curToken.Type != lexer.EOF {
		// Skip empty lines
		if p.curToken.Type == lexer.NEWLINE {
			p.nextToken()
			continue
		}

		// Check for comment lines (lines starting with #)
		if p.curToken.Type == lexer.STRING && len(p.curToken.Literal) > 0 && p.curToken.Literal[0] == '#' {
			// Skip until end of line
			for p.curToken.Type != lexer.NEWLINE && p.curToken.Type != lexer.EOF {
				p.nextToken()
			}
			if p.curToken.Type == lexer.NEWLINE {
				p.nextToken()
			}
			continue
		}

		// Handle old-style COMMENT tokens if any
		if p.curToken.Type == lexer.COMMENT {
			p.nextToken()
			continue
		}

		cmd := p.parseCommand()
		if cmd != nil {
			commands = append(commands, cmd)
		}

		// Expect newline or EOF after command
		if p.curToken.Type != lexer.NEWLINE && p.curToken.Type != lexer.EOF {
			p.error(fmt.Sprintf("expected newline or EOF, got %s", p.curToken.Type))
		}
		if p.curToken.Type == lexer.NEWLINE {
			p.nextToken()
		}
	}

	return commands
}

func (p *Parser) parseCommand() ast.Command {
	switch p.curToken.Type {
	case lexer.GOTO:
		return p.parseGoto()
	case lexer.BACK, lexer.FORWARD, lexer.RELOAD:
		return p.parseSimpleNavigation()
	case lexer.WAIT:
		return p.parseWait()
	case lexer.CLICK, lexer.HOVER:
		return p.parseSimpleInteraction()
	case lexer.FILL, lexer.TYPE, lexer.SELECT:
		return p.parseValueInteraction()
	case lexer.PRESS:
		return p.parsePress()
	case lexer.SCROLL:
		return p.parseScroll()
	case lexer.EXTRACT:
		return p.parseExtract()
	case lexer.SAVE:
		return p.parseSave()
	case lexer.ASSERT:
		return p.parseAssert()
	case lexer.CAPTURE:
		return p.parseCapture()
	case lexer.MOCK:
		return p.parseMock()
	case lexer.BLOCK:
		return p.parseBlock()
	case lexer.THROTTLE:
		return p.parseThrottle()
	case lexer.SCREENSHOT, lexer.PDF, lexer.HAR:
		return p.parseOutput()
	case lexer.JS:
		return p.parseJavaScript()
	case lexer.IF:
		return p.parseIf()
	case lexer.INCLUDE:
		return p.parseInclude()
	case lexer.DEVTOOLS, lexer.BREAKPOINT:
		return p.parseDebugSimple()
	case lexer.LOG, lexer.DEBUG:
		return p.parseDebugWithMessage()
	case lexer.COMPARE:
		return p.parseCompare()
	default:
		p.error(fmt.Sprintf("unknown command: %s", p.curToken.Literal))
		p.nextToken()
		return nil
	}
}

func (p *Parser) parseGoto() ast.Command {
	p.nextToken() // consume 'goto'
	url := p.readRestOfLine()
	return &ast.NavigationCommand{Type: "goto", URL: url}
}

func (p *Parser) parseSimpleNavigation() ast.Command {
	cmdType := p.curToken.Literal
	p.nextToken()
	return &ast.NavigationCommand{Type: cmdType}
}

func (p *Parser) parseWait() ast.Command {
	p.nextToken() // consume 'wait'

	cmd := &ast.WaitCommand{}

	switch p.curToken.Type {
	case lexer.FOR:
		cmd.Type = "for"
		p.nextToken()
		cmd.Selector = p.readRestOfLine()
	case lexer.UNTIL:
		cmd.Type = "until"
		p.nextToken()
		cmd.Condition = p.readRestOfLine()
	case lexer.NUMBER, lexer.STRING:
		cmd.Type = "duration"
		cmd.Duration = p.curToken.Literal
		p.nextToken()
	default:
		p.error("expected 'for', 'until', or duration after 'wait'")
	}

	return cmd
}

func (p *Parser) parseSimpleInteraction() ast.Command {
	cmdType := p.curToken.Literal
	p.nextToken()
	selector := p.readRestOfLine()
	return &ast.InteractionCommand{Type: cmdType, Selector: selector}
}

func (p *Parser) parseValueInteraction() ast.Command {
	cmdType := p.curToken.Literal
	p.nextToken()

	// Read selector (first token)
	selector := p.curToken.Literal
	p.nextToken()

	// Read value (rest of line)
	value := p.readRestOfLine()

	if value == "" {
		p.error(fmt.Sprintf("%s command requires selector and value", cmdType))
		return nil
	}

	return &ast.InteractionCommand{
		Type:     cmdType,
		Selector: selector,
		Value:    value,
	}
}

func (p *Parser) parsePress() ast.Command {
	p.nextToken()
	key := p.readRestOfLine()
	return &ast.InteractionCommand{Type: "press", Value: key}
}

func (p *Parser) parseScroll() ast.Command {
	p.nextToken() // consume 'scroll'
	if p.curToken.Type == lexer.TO {
		p.nextToken()
		target := p.readRestOfLine()
		return &ast.InteractionCommand{Type: "scroll", Target: target}
	}
	p.error("expected 'to' after 'scroll'")
	return nil
}

func (p *Parser) parseExtract() ast.Command {
	p.nextToken() // consume 'extract'

	cmd := &ast.ExtractionCommand{}
	cmd.Selector = p.curToken.Literal
	p.nextToken()

	// Check for 'attr'
	if p.curToken.Type == lexer.ATTR {
		p.nextToken()
		cmd.Attribute = p.curToken.Literal
		p.nextToken()
	}

	// Expect 'as'
	if p.curToken.Type != lexer.AS {
		p.error("expected 'as' in extract command")
		return nil
	}
	p.nextToken()

	cmd.Variable = p.curToken.Literal
	p.nextToken()

	return cmd
}

func (p *Parser) parseSave() ast.Command {
	p.nextToken() // consume 'save'

	variable := p.curToken.Literal
	p.nextToken()

	if p.curToken.Type != lexer.TO {
		p.error("expected 'to' in save command")
		return nil
	}
	p.nextToken()

	filename := p.curToken.Literal
	p.nextToken()

	return &ast.SaveCommand{Variable: variable, Filename: filename}
}

func (p *Parser) parseAssert() ast.Command {
	p.nextToken() // consume 'assert'

	cmd := &ast.AssertionCommand{}

	switch p.curToken.Type {
	case lexer.SELECTOR:
		cmd.Type = "selector"
		p.nextToken()
		cmd.Selector = p.curToken.Literal
		p.nextToken()
		cmd.Condition = p.curToken.Literal
		p.nextToken()
		if p.curToken.Type != lexer.NEWLINE && p.curToken.Type != lexer.EOF {
			cmd.Value = p.readRestOfLine()
		}
	case lexer.STATUS:
		cmd.Type = "status"
		p.nextToken()
		status, err := strconv.Atoi(p.curToken.Literal)
		if err != nil {
			p.error("invalid status code")
			return nil
		}
		cmd.Status = status
		p.nextToken()
	case lexer.NO:
		p.nextToken()
		if p.curToken.Type == lexer.ERRORS {
			cmd.Type = "no-errors"
			p.nextToken()
		}
	case lexer.URL:
		cmd.Type = "url"
		p.nextToken()
		cmd.Condition = p.curToken.Literal
		p.nextToken()
		cmd.Value = p.readRestOfLine()
	}

	return cmd
}

func (p *Parser) parseCapture() ast.Command {
	p.nextToken() // consume 'capture'

	if p.curToken.Type != lexer.NETWORK {
		p.error("expected 'network' after 'capture'")
		return nil
	}
	p.nextToken()

	if p.curToken.Type != lexer.TO {
		p.error("expected 'to' after 'capture network'")
		return nil
	}
	p.nextToken()

	filename := p.curToken.Literal
	p.nextToken()

	return &ast.NetworkCommand{Type: "capture", Target: filename}
}

func (p *Parser) parseMock() ast.Command {
	p.nextToken() // consume 'mock'

	if p.curToken.Type != lexer.API {
		p.error("expected 'api' after 'mock'")
		return nil
	}
	p.nextToken()

	resource := p.curToken.Literal
	p.nextToken()

	if p.curToken.Type != lexer.WITH {
		p.error("expected 'with' in mock command")
		return nil
	}
	p.nextToken()

	filename := p.curToken.Literal
	p.nextToken()

	return &ast.NetworkCommand{Type: "mock", Resource: resource, Target: filename}
}

func (p *Parser) parseBlock() ast.Command {
	p.nextToken()
	pattern := p.readRestOfLine()
	return &ast.NetworkCommand{Type: "block", Pattern: pattern}
}

func (p *Parser) parseThrottle() ast.Command {
	p.nextToken()
	pattern := p.readRestOfLine()
	return &ast.NetworkCommand{Type: "throttle", Pattern: pattern}
}

func (p *Parser) parseOutput() ast.Command {
	cmdType := p.curToken.Literal
	p.nextToken()

	// Check if next token is a selector (for element screenshots)
	var selector, filename string
	first := p.curToken.Literal

	// Simple heuristic: if it starts with # or . it's likely a selector
	if strings.HasPrefix(first, "#") || strings.HasPrefix(first, ".") {
		selector = first
		p.nextToken()
		filename = p.curToken.Literal
		p.nextToken()
	} else {
		filename = first
		p.nextToken()
	}

	return &ast.OutputCommand{Type: cmdType, Filename: filename, Selector: selector}
}

func (p *Parser) parseJavaScript() ast.Command {
	p.nextToken() // consume 'js'

	cmd := &ast.JavaScriptCommand{}

	if p.curToken.Type == lexer.LBRACE {
		// Inline JavaScript block
		p.nextToken()
		code := p.readUntilToken(lexer.RBRACE)
		cmd.Code = strings.TrimSpace(code)
		p.nextToken() // consume '}'
	} else {
		// JavaScript filename
		cmd.Filename = p.curToken.Literal
		p.nextToken()
	}

	// Check for 'as variable'
	if p.curToken.Type == lexer.AS {
		p.nextToken()
		cmd.Variable = p.curToken.Literal
		p.nextToken()
	}

	return cmd
}

func (p *Parser) parseIf() ast.Command {
	p.nextToken() // consume 'if'

	condition := p.readUntilToken(lexer.LBRACE)

	if p.curToken.Type != lexer.LBRACE {
		p.error("expected '{' after if condition")
		return nil
	}
	p.nextToken()

	// Parse body commands until '}'
	body := []ast.Command{}
	for p.curToken.Type != lexer.RBRACE && p.curToken.Type != lexer.EOF {
		if p.curToken.Type == lexer.NEWLINE || p.curToken.Type == lexer.COMMENT {
			p.nextToken()
			continue
		}
		cmd := p.parseCommand()
		if cmd != nil {
			body = append(body, cmd)
		}
	}

	if p.curToken.Type == lexer.RBRACE {
		p.nextToken()
	}

	return &ast.ControlFlowCommand{
		Type:      "if",
		Condition: strings.TrimSpace(condition),
		Body:      body,
	}
}

func (p *Parser) parseInclude() ast.Command {
	p.nextToken()
	filename := p.readRestOfLine()
	return &ast.ControlFlowCommand{Type: "include", Filename: filename}
}

func (p *Parser) parseDebugSimple() ast.Command {
	cmdType := p.curToken.Literal
	p.nextToken()
	return &ast.DebugCommand{Type: cmdType}
}

func (p *Parser) parseDebugWithMessage() ast.Command {
	cmdType := p.curToken.Literal
	p.nextToken()
	message := p.readRestOfLine()
	return &ast.DebugCommand{Type: cmdType, Message: message}
}

func (p *Parser) parseCompare() ast.Command {
	p.nextToken() // consume 'compare'

	current := p.curToken.Literal
	p.nextToken()

	if p.curToken.Type != lexer.WITH {
		p.error("expected 'with' in compare command")
		return nil
	}
	p.nextToken()

	baseline := p.curToken.Literal
	p.nextToken()

	return &ast.CompareCommand{Current: current, Baseline: baseline}
}

// Helper functions

func (p *Parser) readRestOfLine() string {
	return strings.TrimSpace(p.readUntilToken(lexer.NEWLINE, lexer.EOF))
}

func (p *Parser) readUntilToken(types ...lexer.TokenType) string {
	var parts []string
	for !p.isTokenType(p.curToken.Type, types...) {
		parts = append(parts, p.curToken.Literal)
		p.nextToken()
	}
	return strings.Join(parts, " ")
}

func (p *Parser) isTokenType(t lexer.TokenType, types ...lexer.TokenType) bool {
	for _, typ := range types {
		if t == typ {
			return true
		}
	}
	return false
}
