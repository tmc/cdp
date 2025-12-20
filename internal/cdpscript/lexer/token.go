package lexer

import "fmt"

// TokenType represents the type of a token.
type TokenType int

const (
	// Special tokens
	EOF TokenType = iota
	ILLEGAL
	COMMENT

	// Literals
	IDENT  // goto, wait, click, etc.
	STRING // "quoted string" or unquoted strings
	NUMBER // 123, 1.5
	VAR    // ${VARIABLE}

	// Keywords
	GOTO
	WAIT
	FOR
	UNTIL
	CLICK
	FILL
	TYPE
	SELECT
	HOVER
	PRESS
	SCROLL
	TO
	EXTRACT
	AS
	ATTR
	SAVE
	ASSERT
	SELECTOR
	EXISTS
	CONTAINS
	TEXT
	STATUS
	NO
	ERRORS
	URL
	CAPTURE
	NETWORK
	MOCK
	API
	WITH
	BLOCK
	THROTTLE
	SCREENSHOT
	PDF
	HAR
	JS
	IF
	INCLUDE
	DEVTOOLS
	BREAKPOINT
	LOG
	DEBUG
	BACK
	FORWARD
	RELOAD
	IN
	COUNT
	COMPARE

	// Punctuation
	LBRACE // {
	RBRACE // }
	LPAREN // (
	RPAREN // )
	COMMA  // ,
	NEWLINE
)

var keywords = map[string]TokenType{
	"goto":       GOTO,
	"wait":       WAIT,
	"for":        FOR,
	"until":      UNTIL,
	"click":      CLICK,
	"fill":       FILL,
	"type":       TYPE,
	"select":     SELECT,
	"hover":      HOVER,
	"press":      PRESS,
	"scroll":     SCROLL,
	"to":         TO,
	"extract":    EXTRACT,
	"as":         AS,
	"attr":       ATTR,
	"save":       SAVE,
	"assert":     ASSERT,
	"selector":   SELECTOR,
	"exists":     EXISTS,
	"contains":   CONTAINS,
	"text":       TEXT,
	"status":     STATUS,
	"no":         NO,
	"errors":     ERRORS,
	"url":        URL,
	"capture":    CAPTURE,
	"network":    NETWORK,
	"mock":       MOCK,
	"api":        API,
	"with":       WITH,
	"block":      BLOCK,
	"throttle":   THROTTLE,
	"screenshot": SCREENSHOT,
	"pdf":        PDF,
	"har":        HAR,
	"js":         JS,
	"if":         IF,
	"include":    INCLUDE,
	"devtools":   DEVTOOLS,
	"breakpoint": BREAKPOINT,
	"log":        LOG,
	"debug":      DEBUG,
	"back":       BACK,
	"forward":    FORWARD,
	"reload":     RELOAD,
	"in":         IN,
	"count":      COUNT,
	"compare":    COMPARE,
}

// Token represents a lexical token.
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

func (t Token) String() string {
	return fmt.Sprintf("[%d:%d] %s %q", t.Line, t.Column, t.Type, t.Literal)
}

func (tt TokenType) String() string {
	switch tt {
	case EOF:
		return "EOF"
	case ILLEGAL:
		return "ILLEGAL"
	case COMMENT:
		return "COMMENT"
	case IDENT:
		return "IDENT"
	case STRING:
		return "STRING"
	case NUMBER:
		return "NUMBER"
	case VAR:
		return "VAR"
	case LBRACE:
		return "LBRACE"
	case RBRACE:
		return "RBRACE"
	case LPAREN:
		return "LPAREN"
	case RPAREN:
		return "RPAREN"
	case COMMA:
		return "COMMA"
	case NEWLINE:
		return "NEWLINE"
	default:
		if name, ok := keywordName(tt); ok {
			return name
		}
		return fmt.Sprintf("TokenType(%d)", tt)
	}
}

func keywordName(tt TokenType) (string, bool) {
	for k, v := range keywords {
		if v == tt {
			return k, true
		}
	}
	return "", false
}

// LookupIdent returns the TokenType for a given identifier.
func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return IDENT
}
