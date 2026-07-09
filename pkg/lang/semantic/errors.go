package semantic

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"heddle/pkg/lang/ast"
	"heddle/pkg/lang/lexer"
)

type AnalysisError struct {
	Msg  string
	Node ast.Node
}

func (e AnalysisError) Format(filename string, content string) string {
	if e.Node == nil {
		return fmt.Sprintf("error: %s", e.Msg)
	}

	var tok lexer.Token = e.Node.GetToken()
	if tok.Line <= 0 {
		return fmt.Sprintf("error: %s", e.Msg)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("error: %s\n", e.Msg))
	sb.WriteString(fmt.Sprintf("  --> %s:%d:%d\n", filename, tok.Line, tok.Col))
	sb.WriteString("   |\n")

	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if tok.Line <= len(lines) {
		lineContent := lines[tok.Line-1]
		lineStr := fmt.Sprintf("%d", tok.Line)
		padding := strings.Repeat(" ", len(lineStr))

		sb.WriteString(fmt.Sprintf("%s | %s\n", lineStr, lineContent))

		// Build visual padding matching spaces/tabs up to the target column
		prefixRunes := []rune(lineContent)
		colIdx := tok.Col - 1
		if colIdx < 0 {
			colIdx = 0
		}
		if colIdx > len(prefixRunes) {
			colIdx = len(prefixRunes)
		}

		var markerPrefix strings.Builder
		for _, r := range prefixRunes[:colIdx] {
			if r == '\t' {
				markerPrefix.WriteRune('\t')
			} else {
				markerPrefix.WriteRune(' ')
			}
		}

		length := utf8.RuneCountInString(tok.Literal)
		if length <= 0 {
			length = 1
		}
		carats := strings.Repeat("^", length)

		sb.WriteString(fmt.Sprintf("%s | %s%s", padding, markerPrefix.String(), carats))
	} else {
		sb.WriteString("     | <source line unavailable>")
	}

	return sb.String()
}
