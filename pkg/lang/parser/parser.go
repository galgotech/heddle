package parser

import (
	"fmt"
	"strconv"

	"heddle/pkg/lang/ast"
	"heddle/pkg/lang/lexer"
)

// Precedência de operadores
const (
	_ int = iota
	LOWEST
	ASSIGN  // =
	PIPE    // |
	HANDLER // ?
	PREFIX  // -
	CALL    // (
	MEMBER  // .
	LBRACE  // {
)

var precedences = map[lexer.TokenType]int{
	lexer.TokenAssign:   ASSIGN,
	lexer.TokenPipe:     PIPE,
	lexer.TokenQuestion: HANDLER,
	lexer.TokenLparen:   CALL,
	lexer.TokenDot:      MEMBER,
	lexer.TokenLbrace:   LBRACE,
}

type (
	prefixParseFn func() ast.Expression
	infixParseFn  func(ast.Expression) ast.Expression
)

type Parser struct {
	l      *lexer.Lexer
	errors []string

	buffer []lexer.Token
	pos    int

	curToken  lexer.Token
	peekToken lexer.Token

	prefixParseFns map[lexer.TokenType]prefixParseFn
	infixParseFns  map[lexer.TokenType]infixParseFn

	recursionDepth int
}

func New(l *lexer.Lexer) *Parser {
	p := &Parser{
		l:              l,
		errors:         []string{},
		buffer:         []lexer.Token{},
		pos:            -1,
		recursionDepth: 0,
	}

	p.prefixParseFns = make(map[lexer.TokenType]prefixParseFn)
	p.infixParseFns = make(map[lexer.TokenType]infixParseFn)

	// Registrar funções de prefixo
	p.registerPrefix(lexer.TokenIdent, p.parseIdentifier)
	p.registerPrefix(lexer.TokenString, p.parseStringLiteral)
	p.registerPrefix(lexer.TokenInt, p.parseIntegerLiteral)
	p.registerPrefix(lexer.TokenFloat, p.parseFloatLiteral)
	p.registerPrefix(lexer.TokenBool, p.parseBooleanLiteral)
	p.registerPrefix(lexer.TokenDot, p.parseRelativePath)
	p.registerPrefix(lexer.TokenLbrace, p.parseMapLiteral)
	p.registerPrefix(lexer.TokenLbracket, p.parseArrayLiteral)
	p.registerPrefix(lexer.TokenLparen, p.parseTupleExpression)
	p.registerPrefix(lexer.TokenMinus, p.parsePrefixExpression)

	// Registrar funções de infixo
	p.registerInfix(lexer.TokenAssign, p.parseAssignExpression)
	p.registerInfix(lexer.TokenPipe, p.parsePipeExpression)
	p.registerInfix(lexer.TokenQuestion, p.parseFunctionHandlerExpression)
	p.registerInfix(lexer.TokenLparen, p.parseCallExpression)
	p.registerInfix(lexer.TokenDot, p.parseDotExpression)
	p.registerInfix(lexer.TokenLbrace, p.parseLbraceExpression)

	// Inicializar curToken e peekToken
	p.nextToken()

	return p
}

func (p *Parser) nextToken() {
	p.pos++
	p.curToken = p.getToken(p.pos)
	p.peekToken = p.getToken(p.pos + 1)
}

func (p *Parser) getToken(index int) lexer.Token {
	for len(p.buffer) <= index {
		p.buffer = append(p.buffer, p.l.NextToken())
	}
	return p.buffer[index]
}

func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) registerPrefix(tokenType lexer.TokenType, fn prefixParseFn) {
	p.prefixParseFns[tokenType] = fn
}

func (p *Parser) registerInfix(tokenType lexer.TokenType, fn infixParseFn) {
	p.infixParseFns[tokenType] = fn
}

func (p *Parser) curTokenIs(t lexer.TokenType) bool {
	return p.curToken.Type == t
}

func (p *Parser) peekTokenIs(t lexer.TokenType) bool {
	return p.peekToken.Type == t
}

func (p *Parser) lookaheadTokenIs(offset int, t lexer.TokenType) bool {
	return p.getToken(p.pos+offset).Type == t
}

func (p *Parser) expectPeek(t lexer.TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.peekError(t)
	return false
}

func (p *Parser) peekError(t lexer.TokenType) {
	msg := fmt.Sprintf("line %d, col %d: expected next token to be %s, got %s instead",
		p.peekToken.Line, p.peekToken.Col, t, p.peekToken.Type)
	p.errors = append(p.errors, msg)
}

func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) peekPrecedence() int {
	if p, ok := precedences[p.peekToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) skipNewlines() {
	for p.curTokenIs(lexer.TokenNewline) {
		p.nextToken()
	}
}

func (p *Parser) ParseProgram() *ast.Program {
	program := &ast.Program{Statements: []ast.Statement{}}

	for !p.curTokenIs(lexer.TokenEOF) {
		p.skipNewlines()
		if p.curTokenIs(lexer.TokenEOF) {
			break
		}
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
		p.nextToken() // Advance past the last token of the statement
		p.skipNewlines()
	}

	return program
}

func (p *Parser) parseStatement() ast.Statement {
	switch p.curToken.Type {
	case lexer.TokenImport:
		return p.parseImportStatement()
	case lexer.TokenFlow:
		return p.parseFlowStatement()
	case lexer.TokenHandler:
		return p.parseHandlerStatement()
	case lexer.TokenReturn:
		return p.parseReturnStatement()
	default:
		return p.parseExpressionStatement()
	}
}

func (p *Parser) parseImportStatement() ast.Statement {
	stmt := &ast.ImportStatement{Token: p.curToken}

	if !p.expectPeek(lexer.TokenString) {
		return nil
	}
	stmt.Path = p.curToken.Literal

	if p.peekTokenIs(lexer.TokenIdent) {
		p.nextToken()
		stmt.Alias = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

	return stmt
}

func (p *Parser) parseFlowStatement() ast.Statement {
	stmt := &ast.FlowStatement{Token: p.curToken}

	if !p.expectPeek(lexer.TokenIdent) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if p.peekTokenIs(lexer.TokenLparen) {
		p.nextToken() // consume '('
		if !p.expectPeek(lexer.TokenIdent) {
			return nil
		}
		stmt.Param = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		if !p.expectPeek(lexer.TokenRparen) {
			return nil
		}
	}

	if p.peekTokenIs(lexer.TokenQuestion) {
		p.nextToken() // consume '?'
		if !p.expectPeek(lexer.TokenIdent) {
			return nil
		}
		stmt.HandlerName = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		if p.peekTokenIs(lexer.TokenLparen) {
			p.nextToken() // consume '('
			if !p.expectPeek(lexer.TokenRparen) {
				return nil
			}
		}
	}

	if !p.expectPeek(lexer.TokenLbrace) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()

	return stmt
}

func (p *Parser) parseHandlerStatement() ast.Statement {
	stmt := &ast.HandlerStatement{Token: p.curToken}

	if !p.expectPeek(lexer.TokenIdent) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if p.peekTokenIs(lexer.TokenLparen) {
		p.nextToken() // consume '('
		if !p.expectPeek(lexer.TokenIdent) {
			return nil
		}
		stmt.Param = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		if !p.expectPeek(lexer.TokenRparen) {
			return nil
		}
	}

	if !p.expectPeek(lexer.TokenLbrace) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()

	return stmt
}

func (p *Parser) parseBlockStatement() *ast.BlockStatement {
	block := &ast.BlockStatement{Token: p.curToken}
	block.Statements = []ast.Statement{}

	p.nextToken() // consume '{'
	p.skipNewlines()

	for !p.curTokenIs(lexer.TokenRbrace) && !p.curTokenIs(lexer.TokenEOF) {
		p.skipNewlines()
		if p.curTokenIs(lexer.TokenRbrace) || p.curTokenIs(lexer.TokenEOF) {
			break
		}
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.nextToken() // Advance past the last token of the statement
		p.skipNewlines()
	}

	if !p.curTokenIs(lexer.TokenRbrace) {
		p.peekError(lexer.TokenRbrace)
		return nil
	}

	return block
}

func (p *Parser) parseReturnStatement() ast.Statement {
	stmt := &ast.ReturnStatement{Token: p.curToken}

	if p.peekTokenIs(lexer.TokenRbrace) || p.peekTokenIs(lexer.TokenEOF) ||
		p.peekTokenIs(lexer.TokenNewline) || p.peekTokenIs(lexer.TokenImport) ||
		p.peekTokenIs(lexer.TokenFlow) || p.peekTokenIs(lexer.TokenHandler) ||
		p.peekTokenIs(lexer.TokenReturn) {
		return stmt
	}

	p.nextToken()
	stmt.ReturnValue = p.parseExpression(LOWEST)

	return stmt
}

func (p *Parser) parseExpressionStatement() ast.Statement {
	stmt := &ast.ExpressionStatement{Token: p.curToken}
	expr := p.parseExpression(LOWEST)

	if me, ok := expr.(*ast.MatchExpression); ok {
		return &ast.TriggerBlockStatement{
			Token:   stmt.Token,
			Trigger: me.Target,
			Cases:   me.Cases,
		}
	}

	stmt.Expression = expr
	return stmt
}

func (p *Parser) parseExpression(precedence int) ast.Expression {
	p.recursionDepth++
	defer func() { p.recursionDepth-- }()

	if p.recursionDepth > 1000 {
		msg := fmt.Sprintf("line %d, col %d: max recursion depth exceeded (1000)", p.curToken.Line, p.curToken.Col)
		p.errors = append(p.errors, msg)
		return nil
	}

	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		p.noPrefixParseFnError(p.curToken.Type)
		return nil
	}
	leftExp := prefix()

	for !p.peekTokenIs(lexer.TokenEOF) {
		if p.peekTokenIs(lexer.TokenNewline) {
			if p.lookaheadTokenIs(2, lexer.TokenPipe) || p.lookaheadTokenIs(2, lexer.TokenAssign) {
				p.nextToken() // consume Newline, positioning curToken at Newline
			} else {
				break
			}
		}

		if precedence >= p.peekPrecedence() {
			break
		}

		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			return leftExp
		}

		p.nextToken()
		leftExp = infix(leftExp)
	}

	return leftExp
}

func (p *Parser) noPrefixParseFnError(t lexer.TokenType) {
	msg := fmt.Sprintf("line %d, col %d: no prefix parse function for %s found",
		p.curToken.Line, p.curToken.Col, t)
	p.errors = append(p.errors, msg)
}

// --- PREFIX PARSERS ---

func (p *Parser) parseIdentifier() ast.Expression {
	return &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseStringLiteral() ast.Expression {
	return &ast.StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseIntegerLiteral() ast.Expression {
	lit := &ast.IntegerLiteral{Token: p.curToken}
	val, err := strconv.ParseInt(p.curToken.Literal, 0, 64)
	if err != nil {
		msg := fmt.Sprintf("line %d, col %d: could not parse %q as integer",
			p.curToken.Line, p.curToken.Col, p.curToken.Literal)
		p.errors = append(p.errors, msg)
		return nil
	}
	lit.Value = val
	return lit
}

func (p *Parser) parseFloatLiteral() ast.Expression {
	lit := &ast.FloatLiteral{Token: p.curToken}
	val, err := strconv.ParseFloat(p.curToken.Literal, 64)
	if err != nil {
		msg := fmt.Sprintf("line %d, col %d: could not parse %q as float",
			p.curToken.Line, p.curToken.Col, p.curToken.Literal)
		p.errors = append(p.errors, msg)
		return nil
	}
	lit.Value = val
	return lit
}

func (p *Parser) parseBooleanLiteral() ast.Expression {
	return &ast.BooleanLiteral{
		Token: p.curToken,
		Value: p.curToken.Literal == "true",
	}
}

func (p *Parser) parseRelativePath() ast.Expression {
	pe := &ast.PathExpression{Token: p.curToken, Root: nil, Elements: []string{}}

	if !p.expectPeek(lexer.TokenIdent) {
		return nil
	}
	pe.Elements = append(pe.Elements, p.curToken.Literal)

	return pe
}

func (p *Parser) parseTupleExpression() ast.Expression {
	te := &ast.TupleExpression{Token: p.curToken, Expressions: []ast.Expression{}}

	p.nextToken() // consume '('
	p.skipNewlines()

	if p.curTokenIs(lexer.TokenRparen) {
		return te
	}

	te.Expressions = append(te.Expressions, p.parseExpression(LOWEST))
	p.skipNewlines()

	for p.peekTokenIs(lexer.TokenComma) {
		p.nextToken() // consume cur expression
		p.nextToken() // consume ','
		p.skipNewlines()
		if p.curTokenIs(lexer.TokenRparen) {
			break
		}
		te.Expressions = append(te.Expressions, p.parseExpression(LOWEST))
		p.skipNewlines()
	}

	if p.peekTokenIs(lexer.TokenRparen) {
		p.nextToken()
		return te
	}

	if p.curTokenIs(lexer.TokenRparen) {
		return te
	}

	return nil
}

func (p *Parser) parseArrayLiteral() ast.Expression {
	al := &ast.ArrayLiteral{Token: p.curToken, Elements: []ast.Expression{}}

	p.nextToken() // consume '['
	p.skipNewlines()

	if p.curTokenIs(lexer.TokenRbracket) {
		return al
	}

	al.Elements = append(al.Elements, p.parseExpression(LOWEST))
	p.skipNewlines()

	for p.peekTokenIs(lexer.TokenComma) {
		p.nextToken() // consume cur expression
		p.nextToken() // consume ','
		p.skipNewlines()
		if p.curTokenIs(lexer.TokenRbracket) {
			break
		}
		al.Elements = append(al.Elements, p.parseExpression(LOWEST))
		p.skipNewlines()
	}

	if p.curTokenIs(lexer.TokenRbracket) {
		return al
	}
	if !p.expectPeek(lexer.TokenRbracket) {
		return nil
	}

	return al
}

func (p *Parser) parseMapLiteral() ast.Expression {
	ml := &ast.MapLiteral{Token: p.curToken, Pairs: []ast.MapPair{}}

	p.nextToken() // consume '{'
	p.skipNewlines()

	if p.curTokenIs(lexer.TokenRbrace) {
		return ml
	}

	for !p.curTokenIs(lexer.TokenRbrace) && !p.curTokenIs(lexer.TokenEOF) {
		p.skipNewlines()
		if p.curTokenIs(lexer.TokenRbrace) || p.curTokenIs(lexer.TokenEOF) {
			break
		}

		var key ast.Expression
		if p.curTokenIs(lexer.TokenIdent) {
			key = p.parseIdentifier()
		} else if p.curTokenIs(lexer.TokenString) {
			key = p.parseStringLiteral()
		} else {
			msg := fmt.Sprintf("line %d, col %d: expected map key to be identifier or string, got %s",
				p.curToken.Line, p.curToken.Col, p.curToken.Type)
			p.errors = append(p.errors, msg)
			return nil
		}

		if !p.expectPeek(lexer.TokenColon) {
			return nil
		}

		p.nextToken() // consume ':'
		val := p.parseExpression(LOWEST)

		ml.Pairs = append(ml.Pairs, ast.MapPair{Key: key, Value: val})

		if p.peekTokenIs(lexer.TokenComma) {
			p.nextToken() // consume ','
		}
		p.nextToken() // move to next pair or '}'
		p.skipNewlines()
	}

	return ml
}

func (p *Parser) parsePrefixExpression() ast.Expression {
	pe := &ast.PrefixExpression{Token: p.curToken, Operator: p.curToken.Literal}

	p.nextToken()
	pe.Right = p.parseExpression(PREFIX)

	return pe
}

// --- INFIX PARSERS ---

func (p *Parser) parseAssignExpression(left ast.Expression) ast.Expression {
	ae := &ast.AssignExpression{Token: p.curToken}

	ident, ok := left.(*ast.Identifier)
	if !ok {
		msg := fmt.Sprintf("line %d, col %d: left side of assignment must be an identifier, got %T",
			p.curToken.Line, p.curToken.Col, left)
		p.errors = append(p.errors, msg)
		return nil
	}
	ae.Name = ident

	p.nextToken()
	ae.Value = p.parseExpression(LOWEST)

	return ae
}

func (p *Parser) parsePipeExpression(left ast.Expression) ast.Expression {
	var pipe *ast.PipeExpression
	if pe, ok := left.(*ast.PipeExpression); ok {
		pipe = pe
	} else {
		pipe = &ast.PipeExpression{
			Token:       p.curToken,
			Expressions: []ast.Expression{left},
		}
	}

	precedence := p.curPrecedence()
	p.nextToken()
	right := p.parseExpression(precedence)
	pipe.Expressions = append(pipe.Expressions, right)

	return pipe
}

func (p *Parser) parseFunctionHandlerExpression(left ast.Expression) ast.Expression {
	she := &ast.FunctionHandlerExpression{Token: p.curToken, Expr: left}
	p.nextToken() // consume '?'
	she.Handler = p.parseExpression(HANDLER)
	return she
}

func (p *Parser) parseCallExpression(left ast.Expression) ast.Expression {
	if p.isMapperExpressionLookahead(left) {
		return p.parseMapperExpression(left)
	}

	ce := &ast.CallExpression{Token: p.curToken, Function: left, Arguments: []ast.CallArgument{}}

	p.nextToken() // consume '('
	p.skipNewlines()

	if p.curTokenIs(lexer.TokenRparen) {
		return ce
	}

	ce.Arguments = append(ce.Arguments, p.parseCallArgument())
	p.skipNewlines()

	for p.peekTokenIs(lexer.TokenComma) {
		p.nextToken() // consume argument
		p.nextToken() // consume ','
		p.skipNewlines()
		ce.Arguments = append(ce.Arguments, p.parseCallArgument())
		p.skipNewlines()
	}

	if !p.expectPeek(lexer.TokenRparen) {
		return nil
	}

	return ce
}

func (p *Parser) parseCallArgument() ast.CallArgument {
	arg := ast.CallArgument{}

	if p.curTokenIs(lexer.TokenIdent) && p.peekTokenIs(lexer.TokenColon) {
		arg.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		p.nextToken() // consume ident
		p.nextToken() // consume ':'
	}

	arg.Value = p.parseExpression(LOWEST)
	return arg
}

func (p *Parser) parseMapperExpression(left ast.Expression) ast.Expression {
	me := &ast.MapperExpression{Token: p.curToken, Path: left}

	p.nextToken() // consume '('
	if !p.curTokenIs(lexer.TokenIdent) {
		msg := fmt.Sprintf("line %d, col %d: expected mapper alias to be identifier, got %s",
			p.curToken.Line, p.curToken.Col, p.curToken.Type)
		p.errors = append(p.errors, msg)
		return nil
	}

	me.Alias = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(lexer.TokenRparen) {
		return nil
	}

	if p.peekTokenIs(lexer.TokenNewline) {
		p.nextToken()
	}

	if !p.expectPeek(lexer.TokenLbrace) {
		return nil
	}

	me.Body = p.parseBlockStatement()

	return me
}

func (p *Parser) isMapperExpressionLookahead(left ast.Expression) bool {
	_, isPath := left.(*ast.PathExpression)
	_, isIdent := left.(*ast.Identifier)
	if !isPath && !isIdent {
		return false
	}
	if p.getToken(p.pos+1).Type != lexer.TokenIdent ||
		p.getToken(p.pos+2).Type != lexer.TokenRparen {
		return false
	}
	offset := 3
	if p.getToken(p.pos+offset).Type == lexer.TokenNewline {
		offset++
	}
	return p.getToken(p.pos+offset).Type == lexer.TokenLbrace
}

func (p *Parser) parseDotExpression(left ast.Expression) ast.Expression {
	if !p.expectPeek(lexer.TokenIdent) {
		return nil
	}
	identStr := p.curToken.Literal

	if pe, ok := left.(*ast.PathExpression); ok {
		pe.Elements = append(pe.Elements, identStr)
		return pe
	}

	ident, ok := left.(*ast.Identifier)
	if !ok {
		msg := fmt.Sprintf("line %d, col %d: dot property access target must be identifier or path expression, got %T",
			p.curToken.Line, p.curToken.Col, left)
		p.errors = append(p.errors, msg)
		return nil
	}

	return &ast.PathExpression{
		Token:    ident.Token,
		Root:     ident,
		Elements: []string{identStr},
	}
}

func (p *Parser) parseLbraceExpression(left ast.Expression) ast.Expression {
	if p.isMatchExpressionLookahead() {
		me := &ast.MatchExpression{Token: p.curToken, Target: left, Cases: []*ast.MatchCase{}}

		p.nextToken() // consume '{'
		p.skipNewlines()

		for !p.curTokenIs(lexer.TokenRbrace) && !p.curTokenIs(lexer.TokenEOF) {
			p.skipNewlines()
			if p.curTokenIs(lexer.TokenRbrace) || p.curTokenIs(lexer.TokenEOF) {
				break
			}
			mc := p.parseMatchCase()
			if mc != nil {
				me.Cases = append(me.Cases, mc)
			}
			p.nextToken() // advance past the closing '}' of the case block
			p.skipNewlines()
		}

		if !p.curTokenIs(lexer.TokenRbrace) {
			p.peekError(lexer.TokenRbrace)
			return nil
		}

		return me
	}

	_, isIdent := left.(*ast.Identifier)
	_, isPath := left.(*ast.PathExpression)
	if !isIdent && !isPath {
		msg := fmt.Sprintf("line %d, col %d: struct initializer type must be identifier or path expression, got %T",
			p.curToken.Line, p.curToken.Col, left)
		p.errors = append(p.errors, msg)
		return nil
	}

	si := &ast.StructInitializer{Token: p.curToken, Type: left, Fields: []ast.StructField{}}

	p.nextToken() // consume '{'
	p.skipNewlines()

	if p.curTokenIs(lexer.TokenRbrace) {
		return si
	}

	for !p.curTokenIs(lexer.TokenRbrace) && !p.curTokenIs(lexer.TokenEOF) {
		p.skipNewlines()
		if p.curTokenIs(lexer.TokenRbrace) || p.curTokenIs(lexer.TokenEOF) {
			break
		}

		if !p.curTokenIs(lexer.TokenIdent) {
			msg := fmt.Sprintf("line %d, col %d: expected struct field name to be identifier, got %s",
				p.curToken.Line, p.curToken.Col, p.curToken.Type)
			p.errors = append(p.errors, msg)
			return nil
		}

		fieldName := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

		if !p.expectPeek(lexer.TokenColon) {
			return nil
		}

		p.nextToken() // consume ':'
		val := p.parseExpression(LOWEST)

		si.Fields = append(si.Fields, ast.StructField{Name: fieldName, Value: val})

		if p.peekTokenIs(lexer.TokenComma) {
			p.nextToken() // consume ','
		}
		p.nextToken() // move to next field or '}'
		p.skipNewlines()
	}

	return si
}

func (p *Parser) isMatchExpressionLookahead() bool {
	offset := 1
	if p.getToken(p.pos+offset).Type == lexer.TokenNewline {
		offset++
	}
	return p.getToken(p.pos+offset).Type == lexer.TokenIdent &&
		p.getToken(p.pos+offset+1).Type == lexer.TokenLparen
}

func (p *Parser) parseMatchCase() *ast.MatchCase {
	if !p.curTokenIs(lexer.TokenIdent) {
		msg := fmt.Sprintf("line %d, col %d: expected match case tag to be identifier, got %s",
			p.curToken.Line, p.curToken.Col, p.curToken.Type)
		p.errors = append(p.errors, msg)
		return nil
	}

	mc := &ast.MatchCase{Token: p.curToken, Tag: p.curToken.Literal}

	if !p.expectPeek(lexer.TokenLparen) {
		return nil
	}

	if !p.expectPeek(lexer.TokenIdent) {
		return nil
	}

	mc.Param = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(lexer.TokenRparen) {
		return nil
	}

	if !p.expectPeek(lexer.TokenLbrace) {
		return nil
	}

	mc.Body = p.parseBlockStatement()

	return mc
}
