package lexer

import (
	"testing"
)

func TestNextToken_OperatorsAndSymbols(t *testing.T) {
	input := `= | . ? : , ( ) { } [ ] * -
`
	tests := []struct {
		expectedType    TokenType
		expectedLiteral string
	}{
		{TokenAssign, "="},
		{TokenPipe, "|"},
		{TokenDot, "."},
		{TokenQuestion, "?"},
		{TokenColon, ":"},
		{TokenComma, ","},
		{TokenLparen, "("},
		{TokenRparen, ")"},
		{TokenLbrace, "{"},
		{TokenRbrace, "}"},
		{TokenLbracket, "["},
		{TokenRbracket, "]"},
		{TokenStar, "*"},
		{TokenMinus, "-"},
		{TokenNewline, "\n"},
		{TokenEOF, ""},
	}

	l := New(input)
	for i, tt := range tests {
		tok := l.NextToken()
		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q", i, tt.expectedType, tok.Type)
		}
		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q", i, tt.expectedLiteral, tok.Literal)
		}
	}
}

func TestNextToken_KeywordsAndIdents(t *testing.T) {
	input := `import flow handler return true false x http_server _val_1`
	tests := []struct {
		expectedType    TokenType
		expectedLiteral string
	}{
		{TokenImport, "import"},
		{TokenFlow, "flow"},
		{TokenHandler, "handler"},
		{TokenReturn, "return"},
		{TokenBool, "true"},
		{TokenBool, "false"},
		{TokenIdent, "x"},
		{TokenIdent, "http_server"},
		{TokenIdent, "_val_1"},
		{TokenEOF, ""},
	}

	l := New(input)
	for i, tt := range tests {
		tok := l.NextToken()
		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q", i, tt.expectedType, tok.Type)
		}
		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q", i, tt.expectedLiteral, tok.Literal)
		}
	}
}

func TestNextToken_NumbersAndComments(t *testing.T) {
	input := `10 12.34 // ignore this comment
	8080`
	tests := []struct {
		expectedType    TokenType
		expectedLiteral string
	}{
		{TokenInt, "10"},
		{TokenFloat, "12.34"},
		{TokenNewline, "\n"},
		{TokenInt, "8080"},
		{TokenEOF, ""},
	}

	l := New(input)
	for i, tt := range tests {
		tok := l.NextToken()
		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q", i, tt.expectedType, tok.Type)
		}
		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q", i, tt.expectedLiteral, tok.Literal)
		}
	}
}

func TestNextToken_Strings(t *testing.T) {
	input := `"hello" "world\nwith\tescape" "unterminated`
	tests := []struct {
		expectedType    TokenType
		expectedLiteral string
	}{
		{TokenString, "hello"},
		{TokenString, "world\nwith\tescape"},
		{TokenError, "unterminated string literal"},
		{TokenEOF, ""},
	}

	l := New(input)
	for i, tt := range tests {
		tok := l.NextToken()
		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q", i, tt.expectedType, tok.Type)
		}
		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q", i, tt.expectedLiteral, tok.Literal)
		}
	}
}

func TestNextToken_Coordinates(t *testing.T) {
	input := `x = 10
y = "hello"`
	tests := []struct {
		expectedType    TokenType
		expectedLiteral string
		expectedLine    int
		expectedCol     int
	}{
		{TokenIdent, "x", 1, 1},
		{TokenAssign, "=", 1, 3},
		{TokenInt, "10", 1, 5},
		{TokenNewline, "\n", 2, 0},
		{TokenIdent, "y", 2, 1},
		{TokenAssign, "=", 2, 3},
		{TokenString, "hello", 2, 5},
		{TokenEOF, "", 2, 12},
	}

	l := New(input)
	for i, tt := range tests {
		tok := l.NextToken()
		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q", i, tt.expectedType, tok.Type)
		}
		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q", i, tt.expectedLiteral, tok.Literal)
		}
		if tok.Line != tt.expectedLine {
			t.Fatalf("tests[%d] - line wrong. expected=%d, got=%d", i, tt.expectedLine, tok.Line)
		}
		if tok.Col != tt.expectedCol {
			t.Fatalf("tests[%d] - col wrong. expected=%d, got=%d", i, tt.expectedCol, tok.Col)
		}
	}
}

func TestNextToken_ComplexUnicode(t *testing.T) {
	input := `variável_com_acento = "café"`
	tests := []struct {
		expectedType    TokenType
		expectedLiteral string
	}{
		{TokenIdent, "variável_com_acento"},
		{TokenAssign, "="},
		{TokenString, "café"},
		{TokenEOF, ""},
	}

	l := New(input)
	for i, tt := range tests {
		tok := l.NextToken()
		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q", i, tt.expectedType, tok.Type)
		}
		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q", i, tt.expectedLiteral, tok.Literal)
		}
	}
}

func TestTokenString(t *testing.T) {
	tok := Token{Type: TokenIdent, Literal: "foo", Line: 1, Col: 2}
	expected := `Token{Type: IDENT, Literal: "foo", Line: 1, Col: 2}`
	if tok.String() != expected {
		t.Errorf("expected %q, got %q", expected, tok.String())
	}
}

func TestNextToken_EdgeCases(t *testing.T) {
	tests := []struct {
		input           string
		expectedType    TokenType
		expectedLiteral string
	}{
		// Raw newline in string (should return TokenError)
		{"\"hello\nworld\"", TokenError, "unterminated string literal"},
		{"\"hello\rworld\"", TokenError, "unterminated string literal"},
		// Invalid/unhandled escape sequence (should append \ and character)
		{"\"hello\\x\"", TokenString, "hello\\x"},
		// Unterminated string ending with EOF in escape
		{"\"hello\\", TokenError, "unterminated string literal"},
		// Peek past EOF with single slash
		{"/", TokenError, "/"},
		// Peek past EOF with number dot
		{"10.", TokenInt, "10"},
		// Invalid isolated character
		{"@", TokenError, "@"},
		// Escape sequences inside string
		{"\"\\r\"", TokenString, "\r"},
		{"\"\\\"\"", TokenString, "\""},
		{"\"\\\\\"", TokenString, "\\"},
	}

	for _, tt := range tests {
		l := New(tt.input)
		tok := l.NextToken()
		if tok.Type != tt.expectedType {
			t.Errorf("input %q: expected Type %q, got %q", tt.input, tt.expectedType, tok.Type)
		}
		if tok.Literal != tt.expectedLiteral {
			t.Errorf("input %q: expected Literal %q, got %q", tt.input, tt.expectedLiteral, tok.Literal)
		}
	}
}
