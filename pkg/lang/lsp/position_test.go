package lsp

import (
	"testing"

	"go.lsp.dev/protocol"
	"heddle/pkg/lang/lexer"
)

func TestPositionConversions(t *testing.T) {
	// Test LSPPositionToLexer
	pos := protocol.Position{Line: 5, Character: 10}
	line, col := LSPPositionToLexer(pos)
	if line != 6 || col != 11 {
		t.Errorf("LSPPositionToLexer: expected (6, 11), got (%d, %d)", line, col)
	}

	// Test LexerToLSPPosition
	pos2 := LexerToLSPPosition(6, 11)
	if pos2.Line != 5 || pos2.Character != 10 {
		t.Errorf("LexerToLSPPosition: expected {Line: 5, Character: 10}, got {Line: %d, Character: %d}", pos2.Line, pos2.Character)
	}

	// Test bounds safety for LexerToLSPPosition
	posZero := LexerToLSPPosition(0, 0)
	if posZero.Line != 0 || posZero.Character != 0 {
		t.Errorf("LexerToLSPPosition(0, 0) bounds safety failed")
	}

	// Test LexerTokenToLSPRange
	tok := lexer.Token{
		Line:    12,
		Col:     4,
		Literal: "my_variable",
	}
	r := LexerTokenToLSPRange(tok)
	if r.Start.Line != 11 || r.Start.Character != 3 {
		t.Errorf("LexerTokenToLSPRange start mismatch")
	}
	if r.End.Line != 11 || r.End.Character != 14 {
		t.Errorf("LexerTokenToLSPRange end mismatch: expected Character 14, got %d", r.End.Character)
	}
}

func TestIncrementalChanges(t *testing.T) {
	orig := "hello\nworld\n!"
	// Substituir "world" por "there"
	change := protocol.TextDocumentContentChangePartial{
		Range: protocol.Range{
			Start: protocol.Position{Line: 1, Character: 0},
			End:   protocol.Position{Line: 1, Character: 5},
		},
		Text: "there",
	}

	result := ApplyIncrementalChange(orig, change)
	expected := "hello\nthere\n!"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}

	// Inserir " beautiful"
	change2 := protocol.TextDocumentContentChangePartial{
		Range: protocol.Range{
			Start: protocol.Position{Line: 1, Character: 5},
			End:   protocol.Position{Line: 1, Character: 5},
		},
		Text: " beautiful",
	}
	result2 := ApplyIncrementalChange(result, change2)
	expected2 := "hello\nthere beautiful\n!"
	if result2 != expected2 {
		t.Errorf("expected %q, got %q", expected2, result2)
	}

	// Teste com emojis (UTF-16 surrogate pairs e UTF-8 bytes offsets)
	unicodeText := "🚀 hello\n🔥 world"
	// "🚀" = 1 caractere no editor (2 unidades no UTF-16). Espaço = 1 unidade. total = 3.
	// Substituir "hello" por "world"
	change3 := protocol.TextDocumentContentChangePartial{
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 3},
			End:   protocol.Position{Line: 0, Character: 8},
		},
		Text: "world",
	}
	result3 := ApplyIncrementalChange(unicodeText, change3)
	expected3 := "🚀 world\n🔥 world"
	if result3 != expected3 {
		t.Errorf("expected %q, got %q", expected3, result3)
	}
}


