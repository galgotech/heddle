package lexer

import (
	"unicode"
)

type Lexer struct {
	input        []rune
	position     int  // Posição atual no input (índice do caractere ch)
	readPosition int  // Posição de leitura atual (após o caractere ch)
	ch           rune // Caractere atual sob análise
	line         int  // Linha atual no arquivo (1-based)
	col          int  // Coluna atual no arquivo (1-based)
}

// New inicializa o analisador léxico com o código-fonte de entrada
func New(input string) *Lexer {
	l := &Lexer{
		input: []rune(input),
		line:  1,
		col:   0,
	}
	l.readChar()
	return l
}

// readChar lê o próximo caractere do input e avança o cursor
func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++

	if l.ch == '\n' {
		l.line++
		l.col = 0
	} else {
		l.col++
	}
}

// peekChar retorna o próximo caractere sem avançar o cursor
func (l *Lexer) peekChar() rune {
	if l.readPosition >= len(l.input) {
		return 0
	}
	return l.input[l.readPosition]
}

// NextToken analisa e retorna o próximo token da entrada
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	var tok Token
	tok.Line = l.line
	tok.Col = l.col

	switch l.ch {
	case '=':
		tok.Type = TokenAssign
		tok.Literal = string(l.ch)
	case '|':
		tok.Type = TokenPipe
		tok.Literal = string(l.ch)
	case '.':
		tok.Type = TokenDot
		tok.Literal = string(l.ch)
	case '?':
		tok.Type = TokenQuestion
		tok.Literal = string(l.ch)
	case '\n', '\r':
		tok.Type = TokenNewline
		tok.Literal = "\n"
		l.readChar()
		for l.ch == '\n' || l.ch == '\r' || l.ch == ' ' || l.ch == '\t' {
			l.readChar()
		}
		return tok
	case ':':
		tok.Type = TokenColon
		tok.Literal = string(l.ch)
	case ',':
		tok.Type = TokenComma
		tok.Literal = string(l.ch)
	case '(':
		tok.Type = TokenLparen
		tok.Literal = string(l.ch)
	case ')':
		tok.Type = TokenRparen
		tok.Literal = string(l.ch)
	case '{':
		tok.Type = TokenLbrace
		tok.Literal = string(l.ch)
	case '}':
		tok.Type = TokenRbrace
		tok.Literal = string(l.ch)
	case '[':
		tok.Type = TokenLbracket
		tok.Literal = string(l.ch)
	case ']':
		tok.Type = TokenRbracket
		tok.Literal = string(l.ch)
	case '*':
		tok.Type = TokenStar
		tok.Literal = string(l.ch)
	case '-':
		tok.Type = TokenMinus
		tok.Literal = string(l.ch)
	case '/':
		if l.peekChar() == '/' {
			l.skipComment()
			return l.NextToken()
		}
		tok.Type = TokenError
		tok.Literal = string(l.ch)
	case '"':
		strVal, tokType := l.readString()
		tok.Type = tokType
		tok.Literal = strVal
		return tok
	case 0:
		tok.Type = TokenEOF
		tok.Literal = ""
	default:
		if isLetter(l.ch) {
			tok.Literal = l.readIdentifier()
			tok.Type = LookupIdent(tok.Literal)
			return tok
		} else if unicode.IsDigit(l.ch) {
			tokType, literal := l.readNumber()
			tok.Type = tokType
			tok.Literal = literal
			return tok
		} else {
			tok.Type = TokenError
			tok.Literal = string(l.ch)
		}
	}

	l.readChar()
	return tok
}

// skipWhitespace avança o cursor ignorando espaços em branco
func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' {
		l.readChar()
	}
}

// skipComment avança o cursor pulando o comentário de linha até a quebra de linha
func (l *Lexer) skipComment() {
	for l.ch != '\n' && l.ch != 0 {
		l.readChar()
	}
}

// readIdentifier consome um identificador contínuo
func (l *Lexer) readIdentifier() string {
	startPos := l.position
	for isLetter(l.ch) || unicode.IsDigit(l.ch) {
		l.readChar()
	}
	return string(l.input[startPos:l.position])
}

// readNumber consome e retorna literais inteiros ou de ponto flutuante
func (l *Lexer) readNumber() (TokenType, string) {
	startPos := l.position

	// Parte inteira
	for unicode.IsDigit(l.ch) {
		l.readChar()
	}

	isFloat := false
	// Se houver um ponto seguido de um dígito, é um float
	if l.ch == '.' && unicode.IsDigit(l.peekChar()) {
		isFloat = true
		l.readChar() // consome o '.'
		for unicode.IsDigit(l.ch) {
			l.readChar()
		}
	}

	literal := string(l.input[startPos:l.position])
	if isFloat {
		return TokenFloat, literal
	}
	return TokenInt, literal
}

// readString consome literais de string resolvendo sequências de escape comuns
func (l *Lexer) readString() (string, TokenType) {
	l.readChar() // consome a aspas de abertura
	var out []rune

	for l.ch != '"' && l.ch != '\n' && l.ch != '\r' && l.ch != 0 {
		if l.ch == '\\' {
			l.readChar() // consome a barra invertida
			switch l.ch {
			case 'n':
				out = append(out, '\n')
			case 't':
				out = append(out, '\t')
			case 'r':
				out = append(out, '\r')
			case '"':
				out = append(out, '"')
			case '\\':
				out = append(out, '\\')
			case 0:
				return "unterminated string literal", TokenError
			default:
				out = append(out, '\\', l.ch)
			}
		} else {
			out = append(out, l.ch)
		}
		l.readChar()
	}

	if l.ch == '\n' || l.ch == '\r' || l.ch == 0 {
		return "unterminated string literal", TokenError
	}

	l.readChar() // consome a aspas de fechamento
	return string(out), TokenString
}

// isLetter verifica se o rune pode iniciar ou compor um identificador
func isLetter(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_'
}
