package compiler

import (
	"strings"

	"heddle/pkg/lang/ast"
	"heddle/pkg/lang/ir"
)

type Compiler struct {
	funcID        int
	importAliases map[string]string
}

func NewCompiler() *Compiler {
	return &Compiler{
		funcID:        0,
		importAliases: make(map[string]string),
	}
}

func (c *Compiler) nextFunctionID() int {
	c.funcID++
	return c.funcID
}

// Compile traduz um ast.Program em um ir.Program pronto para ser executado ou serializado
func (c *Compiler) Compile(program *ast.Program) (*ir.Program, error) {
	c.funcID = 0
	irProg := &ir.Program{
		Globals:  []ir.Instruction{},
		Flows:    make(map[string]*ir.Routine),
		Handlers: make(map[string]*ir.Routine),
	}

	c.importAliases = make(map[string]string)
	// 1. Coletar aliases de imports para filtrar reads de variáveis
	for _, stmt := range program.Statements {
		if imp, ok := stmt.(*ast.ImportStatement); ok {
			alias := ""
			if imp.Alias != nil {
				alias = imp.Alias.Value
			} else {
				parts := strings.Split(imp.Path, "/")
				alias = parts[len(parts)-1]
			}
			c.importAliases[alias] = imp.Path
		}
	}

	// 2. Compilar declarações
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.ImportStatement:
			// Imports são declarativos e resolvidos na análise semântica. Não geram IR.
			continue

		case *ast.FlowStatement:
			fn, err := c.compileFlow(s)
			if err != nil {
				return nil, err
			}
			irProg.Flows[fn.Name] = fn

		case *ast.HandlerStatement:
			fn, err := c.compileHandler(s)
			if err != nil {
				return nil, err
			}
			irProg.Handlers[fn.Name] = fn

		default:
			// Declarações globais arbitrárias (atribuições de servidores, etc.)
			insts := c.compileStatement(s)
			irProg.Globals = append(irProg.Globals, insts...)
		}
	}

	return irProg, nil
}

func (c *Compiler) compileFlow(flow *ast.FlowStatement) (*ir.Routine, error) {
	fn := &ir.Routine{
		Name:      flow.Name.Value,
		Functions: []ir.Function{},
	}
	if flow.Param != nil {
		fn.ParamName = flow.Param.Value
	}
	if flow.HandlerName != nil {
		fn.HandlerName = flow.HandlerName.Value
	}

	// Compilar corpo
	if flow.Body != nil {
		for _, stmt := range flow.Body.Statements {
			function := c.compileToFunction(stmt)
			fn.Functions = append(fn.Functions, function)
		}
	}

	return fn, nil
}

func (c *Compiler) compileHandler(handler *ast.HandlerStatement) (*ir.Routine, error) {
	fn := &ir.Routine{
		Name:      handler.Name.Value,
		Functions: []ir.Function{},
	}
	if handler.Param != nil {
		fn.ParamName = handler.Param.Value
	}

	// Compilar corpo
	if handler.Body != nil {
		for _, stmt := range handler.Body.Statements {
			function := c.compileToFunction(stmt)
			fn.Functions = append(fn.Functions, function)
		}
	}

	return fn, nil
}

func (c *Compiler) compileToFunction(stmt ast.Statement) ir.Function {
	function := ir.Function{
		ID:           c.nextFunctionID(),
		Instructions: c.compileStatement(stmt),
		Reads:        []string{},
		Writes:       []string{},
	}

	// Analisar dependências de variáveis (Reads/Writes)
	c.collectReads(stmt, &function.Reads)
	c.collectWrites(stmt, &function.Writes)

	return function
}

func (c *Compiler) compileStatement(stmt ast.Statement) []ir.Instruction {
	if stmt == nil {
		return nil
	}

	switch s := stmt.(type) {
	case *ast.BlockStatement:
		var insts []ir.Instruction
		for _, subStmt := range s.Statements {
			insts = append(insts, c.compileStatement(subStmt)...)
		}
		return insts

	case *ast.ReturnStatement:
		var insts []ir.Instruction
		if s.ReturnValue != nil {
			insts = c.compileExpression(s.ReturnValue, false)
		}
		insts = append(insts, ir.Return{})
		return insts

	case *ast.ExpressionStatement:
		return c.compileExpression(s.Expression, false)

	case *ast.TriggerBlockStatement:
		// Triggers com callbacks são compilados como uma instrução MATCH em cima da expressão do trigger
		insts := c.compileExpression(s.Trigger, false)
		var cases []ir.MatchCase
		for _, mc := range s.Cases {
			param := ""
			if mc.Param != nil {
				param = mc.Param.Value
			}
			cases = append(cases, ir.MatchCase{
				Tag:          mc.Tag,
				ParamName:    param,
				Instructions: c.compileStatement(mc.Body),
			})
		}
		insts = append(insts, ir.Match{
			Info: ir.MatchInfo{
				Cases: cases,
			},
		})
		return insts
	}

	return nil
}

func (c *Compiler) compileExpression(expr ast.Expression, hasReceiver bool) []ir.Instruction {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case *ast.StringLiteral:
		return []ir.Instruction{ir.LoadConst{Value: e.Value}}

	case *ast.IntegerLiteral:
		return []ir.Instruction{ir.LoadConst{Value: e.Value}}

	case *ast.FloatLiteral:
		return []ir.Instruction{ir.LoadConst{Value: e.Value}}

	case *ast.BooleanLiteral:
		return []ir.Instruction{ir.LoadConst{Value: e.Value}}

	case *ast.Identifier:
		return []ir.Instruction{ir.LoadVar{Name: e.Value}}

	case *ast.PathExpression:
		if e.Root != nil {
			// Se o root for um alias de pacote (ex: strings.to_upper), não geramos LOAD_VAR.
			// Em vez disso, o PathExpression completo será tratado como o identificador da função no CALL.
			if c.isImportAlias(e.Root.Value) {
				return nil // Será resolvido no CALL
			}
			// Caso contrário, é um acesso normal a propriedades de uma variável local (ex: cart.items.price)
			insts := []ir.Instruction{ir.LoadVar{Name: e.Root.Value}}
			if len(e.Elements) > 0 {
				insts = append(insts, ir.PropAccess{Path: e.Elements})
			}
			return insts
		}
		// Acesso relativo de pipe (ex: .cart.items)
		return []ir.Instruction{ir.PropAccess{Path: e.Elements}}

	case *ast.AssignExpression:
		insts := c.compileExpression(e.Value, false)
		if e.Name != nil {
			insts = append(insts, ir.StoreVar{Name: e.Name.Value})
		}
		return insts

	case *ast.CallExpression:
		var insts []ir.Instruction
		// 1. Compilar os argumentos fornecidos (empurrando-os para a pilha)
		var argNames []string
		hasNamed := false
		for _, arg := range e.Arguments {
			name := ""
			if arg.Name != nil {
				name = arg.Name.Value
				hasNamed = true
			}
			argNames = append(argNames, name)
			insts = append(insts, c.compileExpression(arg.Value, false)...)
		}

		if !hasNamed {
			argNames = nil // Simplificar se todos forem posicionais
		}

		// 2. Extrair o nome da função (pode ser PathExpression ou Identifier)
		funcName := ""
		switch fn := e.Function.(type) {
		case *ast.PathExpression:
			funcName = c.resolvePathExpr(fn)
		case *ast.Identifier:
			funcName = fn.Value
		}

		// 3. Gerar CALL
		insts = append(insts, ir.Call{
			Info: ir.CallInfo{
				FuncName:    funcName,
				ArgNames:    argNames,
				NumArgs:     len(e.Arguments),
				HasReceiver: hasReceiver,
			},
		})
		return insts

	case *ast.PipeExpression:
		var insts []ir.Instruction
		// O primeiro elemento do pipe é avaliado e empurrado para a pilha
		insts = append(insts, c.compileExpression(e.Expressions[0], false)...)
		// Os elementos subsequentes consomem a pilha como receiver
		for i := 1; i < len(e.Expressions); i++ {
			insts = append(insts, c.compileExpression(e.Expressions[i], true)...)
		}
		return insts

	case *ast.StructInitializer:
		var insts []ir.Instruction
		var fieldNames []string
		for _, f := range e.Fields {
			fieldNames = append(fieldNames, f.Name.Value)
			insts = append(insts, c.compileExpression(f.Value, false)...)
		}
		typeName := ""
		switch t := e.Type.(type) {
		case *ast.PathExpression:
			typeName = c.resolvePathExpr(t)
		case *ast.Identifier:
			typeName = t.Value
		}
		insts = append(insts, ir.BuildStruct{
			Info: ir.StructInfo{
				TypeName:   typeName,
				FieldNames: fieldNames,
			},
		})
		return insts

	case *ast.MapLiteral:
		var insts []ir.Instruction
		for _, pair := range e.Pairs {
			// Empurrar chave e valor para a pilha
			keyStr := ""
			switch k := pair.Key.(type) {
			case *ast.Identifier:
				keyStr = k.Value
			case *ast.StringLiteral:
				keyStr = k.Value
			}
			insts = append(insts, ir.LoadConst{Value: keyStr})
			insts = append(insts, c.compileExpression(pair.Value, false)...)
		}
		insts = append(insts, ir.BuildMap{Size: len(e.Pairs)})
		return insts

	case *ast.ArrayLiteral:
		var insts []ir.Instruction
		for _, elem := range e.Elements {
			insts = append(insts, c.compileExpression(elem, false)...)
		}
		insts = append(insts, ir.BuildArray{Size: len(e.Elements)})
		return insts

	case *ast.TupleExpression:
		var branches [][]ir.Instruction
		for _, expr := range e.Expressions {
			branches = append(branches, c.compileExpression(expr, false))
		}
		return []ir.Instruction{ir.Spawn{
			Info: ir.SpawnInfo{
				Branches: branches,
			},
		}}

	case *ast.MapperExpression:
		var insts []ir.Instruction
		var pathElems []string
		if pe, ok := e.Path.(*ast.PathExpression); ok {
			pathElems = pe.Elements
			if pe.Root != nil {
				// Para mapeadores com caminho absoluto (ex: clean_payload.cart.items),
				// precisamos carregar a variável raiz correspondente na pilha.
				insts = append(insts, ir.LoadVar{Name: pe.Root.Value})
			}
		}
		alias := ""
		if e.Alias != nil {
			alias = e.Alias.Value
		}
		insts = append(insts, ir.Mapper{
			Info: ir.MapperInfo{
				Path:         pathElems,
				Alias:        alias,
				Instructions: c.compileStatement(e.Body),
			},
		})
		return insts

	case *ast.MatchExpression:
		insts := c.compileExpression(e.Target, hasReceiver)
		var cases []ir.MatchCase
		for _, mc := range e.Cases {
			param := ""
			if mc.Param != nil {
				param = mc.Param.Value
			}
			cases = append(cases, ir.MatchCase{
				Tag:          mc.Tag,
				ParamName:    param,
				Instructions: c.compileStatement(mc.Body),
			})
		}
		insts = append(insts, ir.Match{
			Info: ir.MatchInfo{
				Cases: cases,
			},
		})
		return insts

	case *ast.FunctionHandlerExpression:
		// Se e.Expr for chamada, repassar hasReceiver
		bodyInsts := c.compileExpression(e.Expr, hasReceiver)
		handlerName := ""
		if ident, ok := e.Handler.(*ast.Identifier); ok {
			handlerName = ident.Value
		} else if call, ok := e.Handler.(*ast.CallExpression); ok {
			if ident, ok := call.Function.(*ast.Identifier); ok {
				handlerName = ident.Value
			}
		}
		return []ir.Instruction{ir.FunctionHandler{
			Info: ir.FunctionHandlerInfo{
				Body:        bodyInsts,
				HandlerName: handlerName,
			},
		}}
	}

	return nil
}

func (c *Compiler) isImportAlias(name string) bool {
	if c.importAliases == nil {
		return false
	}
	_, ok := c.importAliases[name]
	return ok
}

func (c *Compiler) resolveImportAlias(name string) string {
	if c.importAliases == nil {
		return name
	}
	if path, ok := c.importAliases[name]; ok {
		return path
	}
	return name
}

func (c *Compiler) resolvePathExpr(pe *ast.PathExpression) string {
	if pe.Root == nil {
		return pe.String()
	}
	rootVal := pe.Root.Value
	resolvedRoot := c.resolveImportAlias(rootVal)

	// Reconstruct path with resolved root
	var parts []string
	parts = append(parts, resolvedRoot)
	parts = append(parts, pe.Elements...)
	return strings.Join(parts, ".")
}

func (c *Compiler) collectReads(stmt ast.Statement, reads *[]string) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *ast.BlockStatement:
		for _, subStmt := range s.Statements {
			c.collectReads(subStmt, reads)
		}
	case *ast.ReturnStatement:
		c.collectReadsInExpr(s.ReturnValue, reads)
	case *ast.ExpressionStatement:
		c.collectReadsInExpr(s.Expression, reads)
	case *ast.TriggerBlockStatement:
		c.collectReadsInExpr(s.Trigger, reads)
		for _, cCase := range s.Cases {
			c.collectReads(cCase.Body, reads)
		}
	}
}

func (c *Compiler) collectReadsInExpr(expr ast.Expression, reads *[]string) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.Identifier:
		if !c.isImportAlias(e.Value) {
			*reads = appendUnique(*reads, e.Value)
		}
	case *ast.PathExpression:
		if e.Root != nil && !c.isImportAlias(e.Root.Value) {
			*reads = appendUnique(*reads, e.Root.Value)
		}
	case *ast.AssignExpression:
		c.collectReadsInExpr(e.Value, reads)
	case *ast.CallExpression:
		c.collectReadsInExpr(e.Function, reads)
		for _, arg := range e.Arguments {
			c.collectReadsInExpr(arg.Value, reads)
		}
	case *ast.PipeExpression:
		for _, sub := range e.Expressions {
			c.collectReadsInExpr(sub, reads)
		}
	case *ast.StructInitializer:
		c.collectReadsInExpr(e.Type, reads)
		for _, f := range e.Fields {
			c.collectReadsInExpr(f.Value, reads)
		}
	case *ast.MapLiteral:
		for _, pair := range e.Pairs {
			c.collectReadsInExpr(pair.Value, reads)
		}
	case *ast.ArrayLiteral:
		for _, el := range e.Elements {
			c.collectReadsInExpr(el, reads)
		}
	case *ast.TupleExpression:
		for _, sub := range e.Expressions {
			c.collectReadsInExpr(sub, reads)
		}
	case *ast.MapperExpression:
		c.collectReadsInExpr(e.Path, reads)
		c.collectReads(e.Body, reads)
	case *ast.MatchExpression:
		c.collectReadsInExpr(e.Target, reads)
		for _, cCase := range e.Cases {
			c.collectReads(cCase.Body, reads)
		}
	case *ast.FunctionHandlerExpression:
		c.collectReadsInExpr(e.Expr, reads)
		c.collectReadsInExpr(e.Handler, reads)
	}
}

func (c *Compiler) collectWrites(stmt ast.Statement, writes *[]string) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *ast.BlockStatement:
		for _, subStmt := range s.Statements {
			c.collectWrites(subStmt, writes)
		}
	case *ast.ExpressionStatement:
		c.collectWritesInExpr(s.Expression, writes)
	case *ast.TriggerBlockStatement:
		c.collectWritesInExpr(s.Trigger, writes)
		for _, cCase := range s.Cases {
			c.collectWrites(cCase.Body, writes)
		}
	}
}

func (c *Compiler) collectWritesInExpr(expr ast.Expression, writes *[]string) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.AssignExpression:
		if e.Name != nil {
			*writes = appendUnique(*writes, e.Name.Value)
		}
		c.collectWritesInExpr(e.Value, writes)
	case *ast.PipeExpression:
		for _, sub := range e.Expressions {
			c.collectWritesInExpr(sub, writes)
		}
	case *ast.FunctionHandlerExpression:
		c.collectWritesInExpr(e.Expr, writes)
	case *ast.MapperExpression:
		c.collectWrites(e.Body, writes)
	case *ast.MatchExpression:
		for _, cCase := range e.Cases {
			c.collectWrites(cCase.Body, writes)
		}
	}
}

func appendUnique(slice []string, val string) []string {
	for _, item := range slice {
		if item == val {
			return slice
		}
	}
	return append(slice, val)
}
