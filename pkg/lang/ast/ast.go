package ast

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"

	"heddle/pkg/lang/lexer"
)

// Node representa qualquer nó na Árvore de Sintaxe Abstrata
type Node interface {
	TokenLiteral() string
	String() string
	GetToken() lexer.Token
}

// Statement representa uma instrução (declaração)
type Statement interface {
	Node
	statementNode()
}

// Expression representa uma expressão que produz um valor
type Expression interface {
	Node
	expressionNode()
}

func safeString(n interface{ String() string }) string {
	if n == nil || (reflect.ValueOf(n).Kind() == reflect.Pointer && reflect.ValueOf(n).IsNil()) {
		return "<nil>"
	}
	return n.String()
}

// Program é o nó raiz da AST contendo uma lista de declarações
type Program struct {
	Statements []Statement
}

func (p *Program) TokenLiteral() string {
	if len(p.Statements) > 0 {
		return p.Statements[0].TokenLiteral()
	}
	return ""
}

func (p *Program) String() string {
	var out bytes.Buffer
	for _, s := range p.Statements {
		out.WriteString(safeString(s))
		out.WriteString("\n")
	}
	return out.String()
}

// --- STATEMENTS ---

// ImportStatement representa uma instrução de importação: import "path" [alias]
type ImportStatement struct {
	Token lexer.Token // O token TokenImport ("import")
	Path  string      // Caminho do pacote (literal de string sem aspas)
	Alias *Identifier // Alias opcional para o pacote importado
}

func (is *ImportStatement) statementNode()       {}
func (is *ImportStatement) TokenLiteral() string { return is.Token.Literal }
func (is *ImportStatement) String() string {
	var out bytes.Buffer
	out.WriteString("import ")
	out.WriteString(fmt.Sprintf("%q", is.Path))
	if is.Alias != nil {
		out.WriteString(" ")
		out.WriteString(safeString(is.Alias))
	}
	return out.String()
}

// HandlerStatement representa um tratador de erro: handler name(err) { body }
type HandlerStatement struct {
	Token lexer.Token     // O token TokenHandler ("handler")
	Name  *Identifier     // Nome do handler
	Param *Identifier     // Parâmetro do erro opcional (ex: err)
	Body  *BlockStatement // Bloco de comandos do handler
}

func (hs *HandlerStatement) statementNode()       {}
func (hs *HandlerStatement) TokenLiteral() string { return hs.Token.Literal }
func (hs *HandlerStatement) String() string {
	var out bytes.Buffer
	out.WriteString("handler ")
	out.WriteString(safeString(hs.Name))
	if hs.Param != nil {
		out.WriteString("(")
		out.WriteString(safeString(hs.Param))
		out.WriteString(") ")
	} else {
		out.WriteString(" ")
	}
	out.WriteString(safeString(hs.Body))
	return out.String()
}

// FlowStatement representa um fluxo de processamento: flow name(in) ? err_handler { body }
type FlowStatement struct {
	Token       lexer.Token     // O token TokenFlow ("flow")
	Name        *Identifier     // Nome do fluxo
	Param       *Identifier     // Parâmetro de entrada opcional (ex: in)
	HandlerName *Identifier     // Nome do handler de erro associado opcional
	Body        *BlockStatement // Bloco de comandos do fluxo
}

func (fs *FlowStatement) statementNode()       {}
func (fs *FlowStatement) TokenLiteral() string { return fs.Token.Literal }
func (fs *FlowStatement) String() string {
	var out bytes.Buffer
	out.WriteString("flow ")
	out.WriteString(safeString(fs.Name))
	if fs.Param != nil {
		out.WriteString("(")
		out.WriteString(safeString(fs.Param))
		out.WriteString(")")
	}
	if fs.HandlerName != nil {
		out.WriteString(" ? ")
		out.WriteString(safeString(fs.HandlerName))
	}
	out.WriteString(" ")
	out.WriteString(safeString(fs.Body))
	return out.String()
}

// BlockStatement representa um bloco delimitado por chaves { statements }
type BlockStatement struct {
	Token      lexer.Token // O token TokenLbrace ("{")
	Statements []Statement
}

func (bs *BlockStatement) statementNode()       {}
func (bs *BlockStatement) TokenLiteral() string { return bs.Token.Literal }
func (bs *BlockStatement) String() string {
	var out bytes.Buffer
	out.WriteString("{\n")
	for _, s := range bs.Statements {
		out.WriteString("  ")
		out.WriteString(safeString(s))
		out.WriteString("\n")
	}
	out.WriteString("}")
	return out.String()
}

// TriggerBlockStatement representa um bloco reativo/trigger com callbacks: trigger_expr { match_cases }
type TriggerBlockStatement struct {
	Token   lexer.Token // Primeiro token da expressão do trigger
	Trigger Expression  // Expressão do trigger (ex: time.tick("0.5s"))
	Cases   []*MatchCase
}

func (tbs *TriggerBlockStatement) statementNode()       {}
func (tbs *TriggerBlockStatement) TokenLiteral() string { return tbs.Token.Literal }
func (tbs *TriggerBlockStatement) String() string {
	var out bytes.Buffer
	out.WriteString(safeString(tbs.Trigger))
	out.WriteString(" {\n")
	for _, c := range tbs.Cases {
		out.WriteString("  ")
		out.WriteString(safeString(c))
		out.WriteString("\n")
	}
	out.WriteString("}")
	return out.String()
}

// ReturnStatement representa a instrução de retorno: return [expr]
type ReturnStatement struct {
	Token       lexer.Token // O token TokenReturn ("return")
	ReturnValue Expression  // Expressão opcional a ser retornada
}

func (rs *ReturnStatement) statementNode()       {}
func (rs *ReturnStatement) TokenLiteral() string { return rs.Token.Literal }
func (rs *ReturnStatement) String() string {
	var out bytes.Buffer
	out.WriteString("return")
	if rs.ReturnValue != nil {
		out.WriteString(" ")
		out.WriteString(safeString(rs.ReturnValue))
	}
	return out.String()
}

// ExpressionStatement envelopa uma expressão para ser tratada como instrução standalone
type ExpressionStatement struct {
	Token      lexer.Token // Primeiro token da expressão
	Expression Expression
}

func (es *ExpressionStatement) statementNode()       {}
func (es *ExpressionStatement) TokenLiteral() string { return es.Token.Literal }
func (es *ExpressionStatement) String() string {
	if es.Expression != nil {
		return es.Expression.String()
	}
	return ""
}

// --- EXPRESSIONS ---

// AssignExpression representa uma atribuição: variable = value
type AssignExpression struct {
	Token        lexer.Token // O token TokenAssign ("=")
	Name         *Identifier // Identificador de destino
	Value        Expression  // Expressão atribuída
}

func (ae *AssignExpression) expressionNode()      {}
func (ae *AssignExpression) TokenLiteral() string { return ae.Token.Literal }
func (ae *AssignExpression) String() string {
	return fmt.Sprintf("%s = %s", safeString(ae.Name), safeString(ae.Value))
}

// PipeExpression representa o encadeamento de pipelines: expr | expr | expr
type PipeExpression struct {
	Token        lexer.Token // O primeiro token '|' ou do primeiro nó
	Expressions  []Expression
}

func (pe *PipeExpression) expressionNode()      {}
func (pe *PipeExpression) TokenLiteral() string { return pe.Token.Literal }
func (pe *PipeExpression) String() string {
	var exprs []string
	for _, e := range pe.Expressions {
		exprs = append(exprs, safeString(e))
	}
	return strings.Join(exprs, " | ")
}

// PrefixExpression representa operadores prefixos (atualmente apenas "-")
type PrefixExpression struct {
	Token        lexer.Token // O token operador (ex: "-")
	Operator     string      // O literal operador
	Right        Expression  // Expressão da direita
}

func (pe *PrefixExpression) expressionNode()      {}
func (pe *PrefixExpression) TokenLiteral() string { return pe.Token.Literal }
func (pe *PrefixExpression) String() string {
	return "(" + pe.Operator + safeString(pe.Right) + ")"
}

// PathExpression representa caminhos de acesso de propriedade relativos ou absolutos (ex: .cart.items, clean_payload.cart)
type PathExpression struct {
	Token        lexer.Token // O primeiro token do caminho ('.' ou Identificador)
	Root         *Identifier // Identificador raiz (pode ser nil para caminhos relativos tipo `.cart.items`)
	Elements     []string    // Partes do caminho (ex: ["cart", "items"])
}

func (pe *PathExpression) expressionNode()      {}
func (pe *PathExpression) TokenLiteral() string { return pe.Token.Literal }
func (pe *PathExpression) String() string {
	var out bytes.Buffer
	if pe.Root != nil {
		out.WriteString(pe.Root.String())
	}
	for _, elem := range pe.Elements {
		out.WriteString(".")
		out.WriteString(elem)
	}
	return out.String()
}

// CallExpression representa chamadas de funções: fn_expr(arg1, name: arg2)
type CallExpression struct {
	Token        lexer.Token // O token TokenLparen ("(") ou primeiro token da função
	Function     Expression  // Expressão da função chamada (geralmente PathExpression ou Identifier)
	Arguments    []CallArgument
}

func (ce *CallExpression) expressionNode()      {}
func (ce *CallExpression) TokenLiteral() string { return ce.Token.Literal }
func (ce *CallExpression) String() string {
	var out bytes.Buffer
	out.WriteString(safeString(ce.Function))
	out.WriteString("(")
	var args []string
	for _, a := range ce.Arguments {
		args = append(args, a.String())
	}
	out.WriteString(strings.Join(args, ", "))
	out.WriteString(")")
	return out.String()
}

// CallArgument representa um argumento em uma chamada de função, que pode ser nomeado ou posicional
type CallArgument struct {
	Name  *Identifier // Nome do argumento opcional (ex: scalar em scalar: tax_rate)
	Value Expression  // Expressão de valor do argumento
}

func (ca *CallArgument) String() string {
	if ca.Name != nil {
		return safeString(ca.Name) + ": " + safeString(ca.Value)
	}
	return safeString(ca.Value)
}

// Identifier representa um identificador/variável simples (ex: x, main)
type Identifier struct {
	Token        lexer.Token // O token TokenIdent
	Value        string      // O nome do identificador
}

func (i *Identifier) expressionNode()      {}
func (i *Identifier) TokenLiteral() string { return i.Token.Literal }
func (i *Identifier) String() string       { return i.Value }

// MapperExpression representa um mapeador de transformação estrutural: path (alias) { body }
type MapperExpression struct {
	Token        lexer.Token     // Primeiro token do PathExpression
	Path         Expression      // Caminho sob transformação (geralmente PathExpression)
	Alias        *Identifier     // Nome do alias local temporário entre parênteses
	Body         *BlockStatement // Bloco de transformações do pipeline do mapeador
}

func (me *MapperExpression) expressionNode()      {}
func (me *MapperExpression) TokenLiteral() string { return me.Token.Literal }
func (me *MapperExpression) String() string {
	var out bytes.Buffer
	out.WriteString(safeString(me.Path))
	out.WriteString(" (")
	out.WriteString(safeString(me.Alias))
	out.WriteString(") ")
	out.WriteString(safeString(me.Body))
	return out.String()
}

// MatchExpression representa uma expressão de casamento de padrões estrutural: target { match_cases }
type MatchExpression struct {
	Token        lexer.Token // Primeiro token da expressão alvo
	Target       Expression  // Expressão alvo (Identifier, PathExpression ou CallExpression)
	Cases        []*MatchCase
}

func (me *MatchExpression) expressionNode()      {}
func (me *MatchExpression) TokenLiteral() string { return me.Token.Literal }
func (me *MatchExpression) String() string {
	var out bytes.Buffer
	out.WriteString(safeString(me.Target))
	out.WriteString(" {\n")
	for _, c := range me.Cases {
		out.WriteString("  ")
		out.WriteString(safeString(c))
		out.WriteString("\n")
	}
	out.WriteString("}")
	return out.String()
}

// MatchCase representa uma ramificação em um bloco de match ou trigger: tag(param) { body }
type MatchCase struct {
	Token lexer.Token     // O identificador da tag do caso (ex: less, ok, each)
	Tag   string          // Nome do caso
	Param *Identifier     // Nome do parâmetro associado ao payload recebido no caso
	Body  *BlockStatement // Bloco de comandos associado ao caso
}

func (mc *MatchCase) String() string {
	var out bytes.Buffer
	out.WriteString(mc.Tag)
	out.WriteString("(")
	out.WriteString(safeString(mc.Param))
	out.WriteString(") ")
	out.WriteString(safeString(mc.Body))
	return out.String()
}

// StructInitializer representa inicializadores de structs e servidores: TypeName { fields }
type StructInitializer struct {
	Token        lexer.Token   // O primeiro token do tipo
	Type         Expression    // Nome do tipo instanciado (Identifier ou PathExpression)
	Fields       []StructField // Campos inicializadores
}

func (si *StructInitializer) expressionNode()      {}
func (si *StructInitializer) TokenLiteral() string { return si.Token.Literal }
func (si *StructInitializer) String() string {
	var out bytes.Buffer
	out.WriteString(safeString(si.Type))
	out.WriteString(" {\n")
	for _, f := range si.Fields {
		out.WriteString("  " + f.String() + ",\n")
	}
	out.WriteString("}")
	return out.String()
}

// StructField representa a inicialização de um campo de struct: name: value
type StructField struct {
	Name  *Identifier
	Value Expression
}

func (sf *StructField) String() string {
	return safeString(sf.Name) + ": " + safeString(sf.Value)
}

// TupleExpression representa uma tupla de expressões paralelas: (expr1, expr2)
type TupleExpression struct {
	Token        lexer.Token // O token TokenLparen ("(")
	Expressions  []Expression
}

func (te *TupleExpression) expressionNode()      {}
func (te *TupleExpression) TokenLiteral() string { return te.Token.Literal }
func (te *TupleExpression) String() string {
	var exprs []string
	for _, e := range te.Expressions {
		exprs = append(exprs, safeString(e))
	}
	return "(" + strings.Join(exprs, ", ") + ")"
}

// MapLiteral representa dicionários e mapeamentos de dados: { key1: val1, key2: val2 }
type MapLiteral struct {
	Token        lexer.Token // O token TokenLbrace ("{")
	Pairs        []MapPair
}

func (ml *MapLiteral) expressionNode()      {}
func (ml *MapLiteral) TokenLiteral() string { return ml.Token.Literal }
func (ml *MapLiteral) String() string {
	var out bytes.Buffer
	out.WriteString("{\n")
	for _, p := range ml.Pairs {
		out.WriteString("  " + p.String() + ",\n")
	}
	out.WriteString("}")
	return out.String()
}

// MapPair representa um par chave-valor em um MapLiteral
type MapPair struct {
	Key   Expression // Identifier ou StringLiteral
	Value Expression
}

func (mp *MapPair) String() string {
	return safeString(mp.Key) + ": " + safeString(mp.Value)
}

// ArrayLiteral representa literais de listas estruturadas: [expr1, expr2]
type ArrayLiteral struct {
	Token        lexer.Token // O token TokenLbracket ("[")
	Elements     []Expression
}

func (al *ArrayLiteral) expressionNode()      {}
func (al *ArrayLiteral) TokenLiteral() string { return al.Token.Literal }
func (al *ArrayLiteral) String() string {
	var elems []string
	for _, e := range al.Elements {
		elems = append(elems, safeString(e))
	}
	return "[" + strings.Join(elems, ", ") + "]"
}

// --- LITERALS ---

type StringLiteral struct {
	Token        lexer.Token
	Value        string
}

func (sl *StringLiteral) expressionNode()      {}
func (sl *StringLiteral) TokenLiteral() string { return sl.Token.Literal }
func (sl *StringLiteral) String() string       { return fmt.Sprintf("%q", sl.Value) }

type IntegerLiteral struct {
	Token        lexer.Token
	Value        int64
}

func (il *IntegerLiteral) expressionNode()      {}
func (il *IntegerLiteral) TokenLiteral() string { return il.Token.Literal }
func (il *IntegerLiteral) String() string       { return il.Token.Literal }

type FloatLiteral struct {
	Token        lexer.Token
	Value        float64
}

func (fl *FloatLiteral) expressionNode()      {}
func (fl *FloatLiteral) TokenLiteral() string { return fl.Token.Literal }
func (fl *FloatLiteral) String() string       { return fl.Token.Literal }

type BooleanLiteral struct {
	Token        lexer.Token
	Value        bool
}

func (bl *BooleanLiteral) expressionNode()      {}
func (bl *BooleanLiteral) TokenLiteral() string { return bl.Token.Literal }
func (bl *BooleanLiteral) String() string       { return bl.Token.Literal }

// FunctionHandlerExpression representa o tratamento de erro no nível de passo/função: expr ? handler
type FunctionHandlerExpression struct {
	Token        lexer.Token // O token TokenQuestion ("?")
	Expr         Expression  // Expressão da esquerda
	Handler      Expression  // Chamada de handler da direita (CallExpression ou Identifier)
}

func (she *FunctionHandlerExpression) expressionNode()      {}
func (she *FunctionHandlerExpression) TokenLiteral() string { return she.Token.Literal }
func (she *FunctionHandlerExpression) String() string {
	return fmt.Sprintf("%s ? %s", safeString(she.Expr), safeString(she.Handler))
}

// --- GetToken() Implementations ---

func (p *Program) GetToken() lexer.Token {
	if len(p.Statements) > 0 {
		return p.Statements[0].GetToken()
	}
	return lexer.Token{}
}

func (is *ImportStatement) GetToken() lexer.Token        { return is.Token }
func (hs *HandlerStatement) GetToken() lexer.Token       { return hs.Token }
func (fs *FlowStatement) GetToken() lexer.Token          { return fs.Token }
func (bs *BlockStatement) GetToken() lexer.Token         { return bs.Token }
func (tbs *TriggerBlockStatement) GetToken() lexer.Token { return tbs.Token }
func (rs *ReturnStatement) GetToken() lexer.Token        { return rs.Token }
func (es *ExpressionStatement) GetToken() lexer.Token    { return es.Token }
func (ae *AssignExpression) GetToken() lexer.Token       { return ae.Token }
func (pe *PipeExpression) GetToken() lexer.Token         { return pe.Token }
func (pe *PrefixExpression) GetToken() lexer.Token       { return pe.Token }
func (pe *PathExpression) GetToken() lexer.Token         { return pe.Token }
func (ce *CallExpression) GetToken() lexer.Token         { return ce.Token }
func (i *Identifier) GetToken() lexer.Token              { return i.Token }
func (me *MapperExpression) GetToken() lexer.Token         { return me.Token }
func (me *MatchExpression) GetToken() lexer.Token        { return me.Token }
func (si *StructInitializer) GetToken() lexer.Token      { return si.Token }
func (te *TupleExpression) GetToken() lexer.Token        { return te.Token }
func (ml *MapLiteral) GetToken() lexer.Token             { return ml.Token }
func (al *ArrayLiteral) GetToken() lexer.Token           { return al.Token }
func (sl *StringLiteral) GetToken() lexer.Token          { return sl.Token }
func (il *IntegerLiteral) GetToken() lexer.Token         { return il.Token }
func (fl *FloatLiteral) GetToken() lexer.Token           { return fl.Token }
func (bl *BooleanLiteral) GetToken() lexer.Token         { return bl.Token }
func (she *FunctionHandlerExpression) GetToken() lexer.Token { return she.Token }
