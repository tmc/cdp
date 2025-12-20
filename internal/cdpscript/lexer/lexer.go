package lexer

import (
	"strings"
	"unicode"
)

// Lexer tokenizes CDP script commands.
type Lexer struct {
	input        string
	position     int  // current position in input
	readPosition int  // current reading position in input
	ch           byte // current char under examination
	line         int
	column       int
	atLineStart  bool // true if we're at the start of a line (after whitespace)
}

// New creates a new Lexer for the given input.
func New(input string) *Lexer {
	l := &Lexer{
		input:       input,
		line:        1,
		column:      0,
		atLineStart: true,
	}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
	l.column++
	if l.ch == '\n' {
		l.line++
		l.column = 0
	}
}

func (l *Lexer) peekChar() byte {
	if l.readPosition >= len(l.input) {
		return 0
	}
	return l.input[l.readPosition]
}

// NextToken returns the next token from the input.
func (l *Lexer) NextToken() Token {
	var tok Token

	l.skipWhitespace()

	tok.Line = l.line
	tok.Column = l.column

	switch l.ch {
	case '\n':
		tok = Token{Type: NEWLINE, Literal: "\n", Line: l.line, Column: l.column}
		l.atLineStart = true // Reset for next line
	case '{':
		tok = Token{Type: LBRACE, Literal: "{", Line: l.line, Column: l.column}
	case '}':
		tok = Token{Type: RBRACE, Literal: "}", Line: l.line, Column: l.column}
	case '(':
		tok = Token{Type: LPAREN, Literal: "(", Line: l.line, Column: l.column}
	case ')':
		tok = Token{Type: RPAREN, Literal: ")", Line: l.line, Column: l.column}
	case ',':
		tok = Token{Type: COMMA, Literal: ",", Line: l.line, Column: l.column}
	case '$':
		if l.peekChar() == '{' {
			tok.Type = VAR
			tok.Literal = l.readVariable()
		} else {
			tok.Type = IDENT
			tok.Literal = l.readIdentifier()
		}
	case '"', '\'':
		tok.Type = STRING
		tok.Literal = l.readString(l.ch)
	case 0:
		tok.Literal = ""
		tok.Type = EOF
	default:
		if isLetter(l.ch) {
			// Check if this is a keyword or identifier
			literal := l.readIdentifier()
			tokType := LookupIdent(literal)
			if tokType != IDENT {
				// It's a keyword
				tok.Type = tokType
				tok.Literal = literal
				return tok
			}
			// It's an identifier, but could be part of a larger string (like a URL)
			// Check if next char continues the string
			if l.ch == ':' || l.ch == '/' || l.ch == '.' || l.ch == '@' {
				// This is likely part of a URL or email, read as unquoted string
				// Back up and read the whole thing
				l.position -= len(literal)
				l.readPosition = l.position + 1
				l.ch = l.input[l.position]
				tok.Type = STRING
				tok.Literal = l.readUnquotedString()
				return tok
			}
			tok.Type = IDENT
			tok.Literal = literal
			return tok
		} else if isDigit(l.ch) {
			tok.Type = NUMBER
			tok.Literal = l.readNumber()
			return tok
		} else {
			// Read as unquoted string token until whitespace or special char
			tok.Type = STRING
			tok.Literal = l.readUnquotedString()
			return tok
		}
	}

	// Mark that we've seen non-whitespace on this line
	if tok.Type != NEWLINE && tok.Type != COMMENT {
		l.atLineStart = false
	}

	l.readChar()
	return tok
}

func (l *Lexer) skipWhitespace() {
	// Only skip spaces and tabs, not newlines (they're significant)
	// But track if we've seen any non-whitespace on this line yet
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\r' {
		l.readChar()
	}
}

func (l *Lexer) readComment() string {
	position := l.position + 1 // skip '#'
	for l.ch != '\n' && l.ch != 0 {
		l.readChar()
	}
	return strings.TrimSpace(l.input[position:l.position])
}

func (l *Lexer) readIdentifier() string {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' || l.ch == '-' {
		l.readChar()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readNumber() string {
	position := l.position
	for isDigit(l.ch) || l.ch == '.' {
		l.readChar()
	}
	// Handle duration suffixes (e.g., "2s", "500ms")
	if l.ch == 's' || l.ch == 'm' || l.ch == 'h' {
		start := l.ch
		l.readChar()
		if start == 'm' && l.ch == 's' {
			l.readChar()
		}
	}
	return l.input[position:l.position]
}

func (l *Lexer) readString(quote byte) string {
	position := l.position + 1 // skip opening quote
	for {
		l.readChar()
		if l.ch == quote || l.ch == 0 {
			break
		}
		if l.ch == '\\' {
			l.readChar() // skip escaped character
		}
	}
	return l.input[position:l.position]
}

func (l *Lexer) readUnquotedString() string {
	position := l.position
	// Read until we hit whitespace or special syntax chars
	// Allow colons, @, dots, slashes, etc. for URLs and emails
	for l.ch != ' ' && l.ch != '\t' && l.ch != '\n' && l.ch != '\r' &&
		l.ch != '{' && l.ch != '}' && l.ch != '(' && l.ch != ')' &&
		l.ch != ',' && l.ch != 0 && l.ch != '#' {
		l.readChar()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readVariable() string {
	position := l.position // includes $
	l.readChar()           // skip $
	l.readChar()           // skip {
	for l.ch != '}' && l.ch != 0 {
		l.readChar()
	}
	if l.ch == '}' {
		l.readChar()
	}
	return l.input[position:l.position]
}

func isLetter(ch byte) bool {
	return unicode.IsLetter(rune(ch)) || ch == '_'
}

func isDigit(ch byte) bool {
	return unicode.IsDigit(rune(ch))
}
