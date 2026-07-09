package lexer

import "fmt"

type TokenType string

const (
	TokenEOF   TokenType = "EOF"
	TokenError TokenType = "ERROR"

	// Identifiers & Literals
	TokenIdent  TokenType = "IDENT"
	TokenString TokenType = "STRING"
	TokenInt    TokenType = "INT"
	TokenFloat  TokenType = "FLOAT"
	TokenBool   TokenType = "BOOL"

	// Keywords
	TokenImport  TokenType = "IMPORT"
	TokenFlow    TokenType = "FLOW"
	TokenHandler TokenType = "HANDLER"
	TokenReturn  TokenType = "RETURN"

	// Operators & Punctuation
	TokenAssign   TokenType = "="
	TokenPipe     TokenType = "|"
	TokenDot      TokenType = "."
	TokenQuestion TokenType = "?"
	TokenNewline  TokenType = "NEWLINE"
	TokenColon    TokenType = ":"
	TokenComma    TokenType = ","
	TokenLparen   TokenType = "("
	TokenRparen   TokenType = ")"
	TokenLbrace   TokenType = "{"
	TokenRbrace   TokenType = "}"
	TokenLbracket TokenType = "["
	TokenRbracket TokenType = "]"
	TokenStar     TokenType = "*"
	TokenMinus    TokenType = "-"
)

type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Col     int
}

func (t Token) String() string {
	return fmt.Sprintf("Token{Type: %s, Literal: %q, Line: %d, Col: %d}", t.Type, t.Literal, t.Line, t.Col)
}

var keywords = map[string]TokenType{
	"import":  TokenImport,
	"flow":    TokenFlow,
	"handler": TokenHandler,
	"return":  TokenReturn,
	"true":    TokenBool,
	"false":   TokenBool,
}

// LookupIdent verifica se um identificador é uma palavra-chave reservada
func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return TokenIdent
}
