package lsp

import (
	"heddle/pkg/lang/lexer"

	"go.lsp.dev/protocol"
)

// LSPPositionToLexer converte uma posição 0-based do LSP para coordenadas 1-based do Lexer.
func LSPPositionToLexer(pos protocol.Position) (line int, col int) {
	return int(pos.Line) + 1, int(pos.Character) + 1
}

// LexerToLSPPosition converte coordenadas 1-based do Lexer para uma posição 0-based do LSP.
func LexerToLSPPosition(line int, col int) protocol.Position {
	l := max(line-1, 0)
	c := max(col-1, 0)
	return protocol.Position{
		Line:      uint32(l),
		Character: uint32(c),
	}
}

// LexerTokenToLSPRange converte um token 1-based do Lexer para um Range 0-based do LSP.
func LexerTokenToLSPRange(tok lexer.Token) protocol.Range {
	start := LexerToLSPPosition(tok.Line, tok.Col)
	end := start
	length := len(tok.Literal)
	if length <= 0 {
		length = 1
	}
	end.Character += uint32(length)
	return protocol.Range{
		Start: start,
		End:   end,
	}
}

// LSPPositionToOffset converte uma Position 0-based para o correspondente offset de bytes no texto.
func LSPPositionToOffset(text string, pos protocol.Position) int {
	line := uint32(0)
	col := uint32(0)
	for byteOffset, r := range text {
		if line == pos.Line && col == pos.Character {
			return byteOffset
		}
		if r == '\n' {
			line++
			col = 0
		} else {
			if r > 0xffff {
				col += 2 // Surrogate pair no UTF-16
			} else {
				col++
			}
		}
	}
	return len(text)
}

// ApplyIncrementalChange aplica uma alteração parcial incremental no conteúdo do documento.
func ApplyIncrementalChange(content string, change protocol.TextDocumentContentChangePartial) string {
	startByte := LSPPositionToOffset(content, change.Range.Start)
	endByte := LSPPositionToOffset(content, change.Range.End)
	if startByte > len(content) {
		startByte = len(content)
	}
	if endByte > len(content) {
		endByte = len(content)
	}
	if startByte > endByte {
		startByte, endByte = endByte, startByte
	}

	return content[:startByte] + change.Text + content[endByte:]
}
