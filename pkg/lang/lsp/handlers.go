package lsp

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"heddle/internal/config"
	"heddle/pkg/lang/ast"
	"heddle/pkg/lang/lexer"
	"heddle/pkg/lang/semantic"
)

func isNilNode(node ast.Node) bool {
	if node == nil {
		return true
	}
	v := reflect.ValueOf(node)
	return (v.Kind() == reflect.Ptr || v.Kind() == reflect.Pointer) && v.IsNil()
}

// walkAST realiza um caminhamento em profundidade na AST do Heddle
func walkAST(node ast.Node, visit func(ast.Node) bool) {
	if isNilNode(node) {
		return
	}
	if !visit(node) {
		return
	}

	switch n := node.(type) {
	case *ast.Program:
		for _, stmt := range n.Statements {
			walkAST(stmt, visit)
		}
	case *ast.BlockStatement:
		for _, stmt := range n.Statements {
			walkAST(stmt, visit)
		}
	case *ast.FlowStatement:
		walkAST(n.Name, visit)
		walkAST(n.Param, visit)
		walkAST(n.HandlerName, visit)
		walkAST(n.Body, visit)
	case *ast.HandlerStatement:
		walkAST(n.Name, visit)
		walkAST(n.Param, visit)
		walkAST(n.Body, visit)
	case *ast.ImportStatement:
		walkAST(n.Alias, visit)
	case *ast.TriggerBlockStatement:
		walkAST(n.Trigger, visit)
		for _, c := range n.Cases {
			walkAST(c.Param, visit)
			walkAST(c.Body, visit)
		}
	case *ast.ReturnStatement:
		walkAST(n.ReturnValue, visit)
	case *ast.ExpressionStatement:
		walkAST(n.Expression, visit)
	case *ast.AssignExpression:
		walkAST(n.Name, visit)
		walkAST(n.Value, visit)
	case *ast.PipeExpression:
		for _, expr := range n.Expressions {
			walkAST(expr, visit)
		}
	case *ast.PrefixExpression:
		walkAST(n.Right, visit)
	case *ast.PathExpression:
		walkAST(n.Root, visit)
	case *ast.CallExpression:
		walkAST(n.Function, visit)
		for _, arg := range n.Arguments {
			walkAST(arg.Name, visit)
			walkAST(arg.Value, visit)
		}
	case *ast.MapperExpression:
		walkAST(n.Path, visit)
		walkAST(n.Alias, visit)
		walkAST(n.Body, visit)
	case *ast.MatchExpression:
		walkAST(n.Target, visit)
		for _, c := range n.Cases {
			walkAST(c.Param, visit)
			walkAST(c.Body, visit)
		}
	case *ast.StructInitializer:
		walkAST(n.Type, visit)
		for _, field := range n.Fields {
			walkAST(field.Name, visit)
			walkAST(field.Value, visit)
		}
	case *ast.TupleExpression:
		for _, expr := range n.Expressions {
			walkAST(expr, visit)
		}
	case *ast.MapLiteral:
		for _, pair := range n.Pairs {
			walkAST(pair.Key, visit)
			walkAST(pair.Value, visit)
		}
	case *ast.ArrayLiteral:
		for _, expr := range n.Elements {
			walkAST(expr, visit)
		}
	case *ast.FunctionHandlerExpression:
		walkAST(n.Expr, visit)
		walkAST(n.Handler, visit)
	}
}

// findNodeAt localiza o nó mais profundo na AST contendo a coordenada informada
func findNodeAt(program *ast.Program, line, col int) ast.Node {
	var bestNode ast.Node
	walkAST(program, func(n ast.Node) bool {
		tok := n.GetToken()
		if tok.Line <= 0 {
			return true
		}
		if tok.Line == line {
			startCol := tok.Col
			endCol := tok.Col + len(tok.Literal)
			if pe, ok := n.(*ast.PathExpression); ok {
				length := len(tok.Literal)
				for _, elem := range pe.Elements {
					length += 1 + len(elem) // +1 for the dot '.'
				}
				endCol = tok.Col + length
			}
			if col >= startCol && col <= endCol {
				bestNode = n
			}
		}
		return true
	})
	return bestNode
}

// getBlockEndLine encontra a última linha ocupada por comandos do bloco
func getBlockEndLine(body *ast.BlockStatement) int {
	if body == nil {
		return 0
	}
	if len(body.Statements) == 0 {
		return body.GetToken().Line
	}
	maxLine := body.GetToken().Line
	walkAST(body, func(n ast.Node) bool {
		if n.GetToken().Line > maxLine {
			maxLine = n.GetToken().Line
		}
		return true
	})
	return maxLine + 1
}

func isLineInBlock(line, startLine, endLine int) bool {
	return line >= startLine && line <= endLine
}

// findSymbolDefinition resolve a definição local de um símbolo
func findSymbolDefinition(program *ast.Program, name string, currentLine int) (ast.Node, string) {
	var enclosingFlow *ast.FlowStatement
	var enclosingHandler *ast.HandlerStatement
	var enclosingMatchCase *ast.MatchCase

	walkAST(program, func(n ast.Node) bool {
		switch m := n.(type) {
		case *ast.FlowStatement:
			if isLineInBlock(currentLine, m.GetToken().Line, getBlockEndLine(m.Body)) {
				enclosingFlow = m
			}
		case *ast.HandlerStatement:
			if isLineInBlock(currentLine, m.GetToken().Line, getBlockEndLine(m.Body)) {
				enclosingHandler = m
			}
		case *ast.MatchExpression:
			for _, c := range m.Cases {
				if c.Body != nil && isLineInBlock(currentLine, c.Token.Line, getBlockEndLine(c.Body)) {
					enclosingMatchCase = c
				}
			}
		case *ast.TriggerBlockStatement:
			for _, c := range m.Cases {
				if c.Body != nil && isLineInBlock(currentLine, c.Token.Line, getBlockEndLine(c.Body)) {
					enclosingMatchCase = c
				}
			}
		}
		return true
	})

	if enclosingMatchCase != nil {
		if enclosingMatchCase.Param != nil && enclosingMatchCase.Param.Value == name {
			return enclosingMatchCase.Param, "parameter"
		}
	}

	if enclosingFlow != nil {
		if enclosingFlow.Param != nil && enclosingFlow.Param.Value == name {
			return enclosingFlow.Param, "parameter"
		}
		var defNode ast.Node
		walkAST(enclosingFlow.Body, func(n ast.Node) bool {
			if assign, ok := n.(*ast.AssignExpression); ok && assign.Name != nil && assign.Name.Value == name {
				if assign.GetToken().Line <= currentLine {
					defNode = assign.Name
				}
			}
			return true
		})
		if defNode != nil {
			return defNode, "variable"
		}
	}

	if enclosingHandler != nil {
		if enclosingHandler.Param != nil && enclosingHandler.Param.Value == name {
			return enclosingHandler.Param, "parameter"
		}
		var defNode ast.Node
		walkAST(enclosingHandler.Body, func(n ast.Node) bool {
			if assign, ok := n.(*ast.AssignExpression); ok && assign.Name != nil && assign.Name.Value == name {
				if assign.GetToken().Line <= currentLine {
					defNode = assign.Name
				}
			}
			return true
		})
		if defNode != nil {
			return defNode, "variable"
		}
	}

	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.FlowStatement:
			if s.Name != nil && s.Name.Value == name {
				return s.Name, "flow"
			}
		case *ast.HandlerStatement:
			if s.Name != nil && s.Name.Value == name {
				return s.Name, "handler"
			}
		case *ast.ImportStatement:
			if s.Alias != nil && s.Alias.Value == name {
				return s.Alias, "package"
			}
			parts := strings.Split(s.Path, "/")
			lastPart := parts[len(parts)-1]
			if lastPart == name {
				return s, "package"
			}
		case *ast.ExpressionStatement:
			if assign, ok := s.Expression.(*ast.AssignExpression); ok && assign.Name != nil && assign.Name.Value == name {
				return assign.Name, "global"
			}
		}
	}

	return nil, ""
}

// resolvePathHover analisa qual parte do PathExpression foi selecionada
func resolvePathHover(pe *ast.PathExpression, col int) (rootSelected bool, elementIndex int) {
	if pe.Root != nil {
		rootLen := len(pe.Root.Value)
		start := pe.Root.Token.Col
		if col >= start && col < start+rootLen {
			return true, -1
		}
		currCol := start + rootLen
		for idx, elem := range pe.Elements {
			currCol++ // '.'
			elemLen := len(elem)
			if col >= currCol && col < currCol+elemLen {
				return false, idx
			}
			currCol += elemLen
		}
	} else {
		currCol := pe.Token.Col
		for idx, elem := range pe.Elements {
			currCol++ // '.'
			elemLen := len(elem)
			if col >= currCol && col < currCol+elemLen {
				return false, idx
			}
			currCol += elemLen
		}
	}
	return false, -1
}

var (
	pkgCacheMu sync.RWMutex
	pkgCache   = make(map[string]*semantic.PackageSymbols)
)

// getGoPackageSymbols extrai os símbolos de um pacote Go usando o GoASTParser
func getGoPackageSymbols(docDir string, pkgPath string) (*semantic.PackageSymbols, error) {
	pkgCacheMu.RLock()
	cached, ok := pkgCache[pkgPath]
	pkgCacheMu.RUnlock()
	if ok {
		return cached, nil
	}

	projectRoot, _ := config.FindProjectRootFrom(docDir)
	var targetDir string
	if projectRoot != "" {
		targetDir = filepath.Join(projectRoot, "pkg/lib")
		if cfg, err := config.LoadConfigFromDir(docDir); err == nil {
			if p, ok := cfg.ResolveLocalPackagePath(projectRoot, "golang", ""); ok {
				targetDir = p
			}
		}
	}
	gp := &semantic.GoASTParser{}
	pkgSymbols, err := gp.ParsePackage(pkgPath, targetDir, nil)
	if err != nil {
		return nil, err
	}

	pkgCacheMu.Lock()
	pkgCache[pkgPath] = pkgSymbols
	pkgCacheMu.Unlock()

	return pkgSymbols, nil
}

// resolveGoSymbol localiza as informações de tipo ou assinatura do Go a partir do heddle
func resolveGoSymbol(docDir string, program *ast.Program, rootName, memberName string) (*semantic.FuncSymbol, *semantic.TypeSymbol) {
	var pkgPath string
	for _, stmt := range program.Statements {
		if imp, ok := stmt.(*ast.ImportStatement); ok {
			alias := ""
			if imp.Alias != nil {
				alias = imp.Alias.Value
			} else {
				parts := strings.Split(imp.Path, "/")
				alias = parts[len(parts)-1]
			}
			if alias == rootName {
				pkgPath = imp.Path
				break
			}
		}
	}

	if pkgPath == "" {
		return nil, nil
	}

	pkgSymbols, err := getGoPackageSymbols(docDir, pkgPath)
	if err != nil {
		return nil, nil
	}

	if fn, ok := pkgSymbols.Funcs[memberName]; ok {
		return fn, nil
	}
	pascalName := snakeToPascal(memberName)
	if fn, ok := pkgSymbols.Funcs[pascalName]; ok {
		return fn, nil
	}

	if t, ok := pkgSymbols.Types[memberName]; ok {
		return nil, t
	}
	if t, ok := pkgSymbols.Types[pascalName]; ok {
		return nil, t
	}

	return nil, nil
}

func snakeToPascal(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

// Definition implementa textDocument/definition para navegação
func (s *HeddleLspServer) Definition(ctx context.Context, params *protocol.DefinitionParams) (result protocol.DefinitionResult, err error) {
	uriStr := string(params.TextDocument.URI)
	if err := IsSafeURI(uriStr, s.workspaceRoot); err != nil {
		return nil, err
	}
	doc, ok := s.cache.Get(uriStr)
	if !ok {
		return nil, nil
	}
	program := doc.GetProgram()
	if program == nil {
		return nil, nil
	}

	docFsPath := uri.URI(doc.URI).FsPath()
	docDir := filepath.Dir(docFsPath)

	line, col := LSPPositionToLexer(params.Position)

	node := findNodeAt(program, line, col)
	if node == nil {
		return nil, nil
	}

	if ident, ok := node.(*ast.Identifier); ok {
		defNode, _ := findSymbolDefinition(program, ident.Value, line)
		if defNode != nil {
			tok := defNode.GetToken()
			return protocol.LocationSlice{
				{
					URI:   params.TextDocument.URI,
					Range: LexerTokenToLSPRange(tok),
				},
			}, nil
		}
	}

	if pe, ok := node.(*ast.PathExpression); ok {
		rootSelected, elemIdx := resolvePathHover(pe, col)
		if rootSelected && pe.Root != nil {
			defNode, _ := findSymbolDefinition(program, pe.Root.Value, line)
			if defNode != nil {
				tok := defNode.GetToken()
				return protocol.LocationSlice{
					{
						URI:   params.TextDocument.URI,
						Range: LexerTokenToLSPRange(tok),
					},
				}, nil
			}
		} else if elemIdx >= 0 && pe.Root != nil {
			memberName := pe.Elements[elemIdx]
			
			// Tentar resolver usando o tipo da variável a partir do inferredTypes
			var rootType *ast.TypeInfo
			if types := doc.GetInferredTypes(); types != nil {
				if t, ok := types[pe.Root]; ok {
					rootType = t
				}
			}
			
			var fn *semantic.FuncSymbol
			var typ *semantic.TypeSymbol
			
			if rootType != nil && rootType.Kind == "struct" && rootType.Package != "" {
				pkgSymbols, err := getGoPackageSymbols(docDir, rootType.Package)
				if err == nil && pkgSymbols != nil {
					fn = findMethod(pkgSymbols, rootType.Name, memberName)
					if fn == nil {
						// Se não for método, pode ser um campo
						typ = findStructType(pkgSymbols, rootType.Name)
					}
				}
			}
			
			if fn == nil && typ == nil {
				// Fallback para lógica original se não encontrou via tipo inferido
				rootName := pe.Root.Value
				if !isPackageNamespace(program, rootName) {
					if pkgAlias, _ := resolveVariableType(program, rootName, line); pkgAlias != "" {
						rootName = pkgAlias
					}
				}
				fn, typ = resolveGoSymbol(docDir, program, rootName, memberName)
			}

			if fn != nil && fn.FilePath != "" {
				return protocol.LocationSlice{
					{
						URI: uri.URI("file://" + fn.FilePath),
						Range: protocol.Range{
							Start: LexerToLSPPosition(fn.Line, fn.Col),
							End:   LexerToLSPPosition(fn.Line, fn.Col+len(fn.Name)),
						},
					},
				}, nil
			}
			if typ != nil && typ.FilePath != "" {
				return protocol.LocationSlice{
					{
						URI: uri.URI("file://" + typ.FilePath),
						Range: protocol.Range{
							Start: LexerToLSPPosition(typ.Line, typ.Col),
							End:   LexerToLSPPosition(typ.Line, typ.Col+len(typ.Name)),
						},
					},
				}, nil
			}
		}
	}

	return nil, nil
}

func findMethod(pkgSymbols *semantic.PackageSymbols, receiverType, methodName string) *semantic.FuncSymbol {
	pascalRecv := snakeToPascal(receiverType)
	snakeRecv := pascalToSnake(receiverType)
	receivers := []string{receiverType, pascalRecv, snakeRecv, strings.ToLower(receiverType)}
	methods := []string{methodName, snakeToPascal(methodName), pascalToSnake(methodName), strings.ToLower(methodName)}
	for _, r := range receivers {
		for _, m := range methods {
			key := r + "." + m
			if fn, ok := pkgSymbols.Funcs[key]; ok {
				return fn
			}
		}
	}
	return nil
}

func findStructType(pkgSymbols *semantic.PackageSymbols, typeName string) *semantic.TypeSymbol {
	pascal := snakeToPascal(typeName)
	snake := pascalToSnake(typeName)
	variations := []string{typeName, pascal, snake, strings.ToLower(typeName)}
	for _, v := range variations {
		if t, ok := pkgSymbols.Types[v]; ok {
			return t
		}
	}
	return nil
}

func isStructTypeName(name string) bool {
	if name == "" {
		return false
	}
	cleaned := strings.TrimPrefix(name, "*")
	cleaned = strings.TrimPrefix(cleaned, "[]")
	if idx := strings.LastIndex(cleaned, "."); idx != -1 {
		cleaned = cleaned[idx+1:]
	}
	if cleaned == "" {
		return false
	}
	first := cleaned[0]
	return first >= 'A' && first <= 'Z'
}

func resolveFieldTypeInfo(fieldTypeStr string, parentPackage string, program *ast.Program) *ast.TypeInfo {
	cleaned := strings.TrimPrefix(fieldTypeStr, "*")
	kind := "scalar"
	if strings.HasPrefix(cleaned, "[]") {
		kind = "slice"
	} else if strings.HasPrefix(cleaned, "map[") {
		kind = "map"
	} else if cleaned == "error" {
		kind = "error"
	} else if cleaned == "any" {
		kind = "generic"
	} else if isStructTypeName(cleaned) {
		kind = "struct"
	}

	if kind != "struct" {
		return &ast.TypeInfo{
			Kind: kind,
			Name: cleaned,
		}
	}

	name := cleaned
	pkgPath := parentPackage

	if idx := strings.Index(name, "."); idx != -1 {
		pkgAlias := name[:idx]
		name = name[idx+1:]

		for _, stmt := range program.Statements {
			if imp, ok := stmt.(*ast.ImportStatement); ok {
				alias := ""
				if imp.Alias != nil {
					alias = imp.Alias.Value
				} else {
					parts := strings.Split(imp.Path, "/")
					alias = parts[len(parts)-1]
				}
				if alias == pkgAlias {
					pkgPath = imp.Path
					break
				}
			}
		}
	}

	return &ast.TypeInfo{
		Kind:    "struct",
		Name:    name,
		Package: pkgPath,
	}
}

func pascalToSnake(s string) string {
	var res []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			if res[len(res)-1] != '_' {
				res = append(res, '_')
			}
		}
		res = append(res, r)
	}
	return strings.ToLower(string(res))
}

func findCallArgumentParamType(program *ast.Program, ident *ast.Identifier, docDir string, inferredTypes map[ast.Expression]*ast.TypeInfo) (string, bool) {
	var parentCall *ast.CallExpression
	var argName string

	walkAST(program, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpression); ok {
			for _, arg := range ce.Arguments {
				if arg.Name != nil && arg.Name == ident {
					parentCall = ce
					argName = arg.Name.Value
					return false
				}
			}
		}
		return true
	})

	if parentCall == nil {
		return "", false
	}

	var fn *semantic.FuncSymbol
	if pe, ok := parentCall.Function.(*ast.PathExpression); ok && pe.Root != nil {
		fnSym, _ := resolveGoSymbol(docDir, program, pe.Root.Value, pe.Elements[0])
		if fnSym != nil {
			fn = fnSym
		} else {
			defNode, _ := findSymbolDefinition(program, pe.Root.Value, pe.Root.Token.Line)
			var rootType *ast.TypeInfo
			if defNode != nil {
				if id, ok := defNode.(*ast.Identifier); ok {
					rootType = inferredTypes[id]
				}
			}
			if rootType == nil {
				if pe.Root.Value == "request" || pe.Root.Value == "req" {
					rootType = &ast.TypeInfo{Kind: "struct", Name: "Request", Package: "net/http"}
				}
			}

			if rootType != nil && rootType.Kind == "struct" && rootType.Package != "" {
				pkgSymbols, err := getGoPackageSymbols(docDir, rootType.Package)
				if err == nil && pkgSymbols != nil {
					fn = findMethod(pkgSymbols, rootType.Name, pe.Elements[0])
				}
			}
		}
	}

	if fn != nil {
		for i, name := range fn.ParamNames {
			if strings.EqualFold(name, argName) || strings.EqualFold(snakeToPascal(name), argName) || strings.EqualFold(pascalToSnake(name), argName) {
				return fn.ParamTypes[i], true
			}
		}
	}

	return "", false
}

// Hover implementa textDocument/hover para exibir documentação/tipos
func (s *HeddleLspServer) Hover(ctx context.Context, params *protocol.HoverParams) (result *protocol.Hover, err error) {
	uriStr := string(params.TextDocument.URI)
	if err := IsSafeURI(uriStr, s.workspaceRoot); err != nil {
		return nil, err
	}
	doc, ok := s.cache.Get(uriStr)
	if !ok {
		return nil, nil
	}
	program := doc.GetProgram()
	if program == nil {
		return nil, nil
	}

	docFsPath := uri.URI(doc.URI).FsPath()
	docDir := filepath.Dir(docFsPath)

	line, col := LSPPositionToLexer(params.Position)

	node := findNodeAt(program, line, col)
	if node == nil {
		return nil, nil
	}

	var hoverText string

	if ident, ok := node.(*ast.Identifier); ok {
		if paramType, found := findCallArgumentParamType(program, ident, docDir, doc.GetInferredTypes()); found {
			hoverText = fmt.Sprintf("argument %s: %s", ident.Value, paramType)
		} else {
			defNode, kind := findSymbolDefinition(program, ident.Value, line)
			var typeInfo *ast.TypeInfo
			if types := doc.GetInferredTypes(); types != nil {
				typeInfo = types[ident]
			}
			typeStr := "any"
			if typeInfo != nil {
				if typeInfo.Package != "" && typeInfo.Kind == "struct" {
					typeStr = typeInfo.Package + "." + typeInfo.Name
				} else {
					typeStr = typeInfo.Name
				}
			} else {
				if ident.Value == "request" || ident.Value == "req" {
					typeStr = "net/http.Request"
				} else if ident.Value == "now" {
					typeStr = "string"
				} else if ident.Value == "err" || ident.Value == "error" {
					typeStr = "error"
				}
			}
			if defNode != nil {
				hoverText = fmt.Sprintf("(%s) %s: %s", kind, ident.Value, typeStr)
			} else {
				hoverText = fmt.Sprintf("%s: %s", ident.Value, typeStr)
			}
		}
	} else if pe, ok := node.(*ast.PathExpression); ok {
		rootSelected, elemIdx := resolvePathHover(pe, col)
		if rootSelected && pe.Root != nil {
			hoverText = fmt.Sprintf("(package) %s", pe.Root.Value)
		} else if elemIdx >= 0 && pe.Root != nil {
			memberName := pe.Elements[elemIdx]

			// 1. Tentar primeiro como símbolo de pacote
			fn, typ := resolveGoSymbol(docDir, program, pe.Root.Value, memberName)
			if fn != nil {
				paramsList := []string{}
				for i := range fn.ParamNames {
					paramsList = append(paramsList, fmt.Sprintf("%s %s", fn.ParamNames[i], fn.ParamTypes[i]))
				}
				returns := strings.Join(fn.ReturnTypes, ", ")
				if len(fn.ReturnTypes) > 1 {
					returns = "(" + returns + ")"
				} else if len(fn.ReturnTypes) == 0 {
					returns = "void"
				}
				hoverText = fmt.Sprintf("func %s.%s(%s) %s", pe.Root.Value, fn.Name, strings.Join(paramsList, ", "), returns)
			} else if typ != nil {
				hoverText = fmt.Sprintf("type %s.%s struct", pe.Root.Value, typ.Name)
			} else {
				// 2. Se falhar, tentar como acesso a membro/método de variável local/parâmetro
				defNode, _ := findSymbolDefinition(program, pe.Root.Value, line)
				var rootType *ast.TypeInfo
				if defNode != nil {
					if ident, ok := defNode.(*ast.Identifier); ok {
						if types := doc.GetInferredTypes(); types != nil {
							rootType = types[ident]
						}
					}
				}

				currentType := rootType
				if currentType == nil {
					if pe.Root.Value == "request" || pe.Root.Value == "req" {
						currentType = &ast.TypeInfo{Kind: "struct", Name: "Request", Package: "net/http"}
					} else if pe.Root.Value == "now" {
						currentType = &ast.TypeInfo{Kind: "scalar", Name: "string"}
					} else if pe.Root.Value == "err" || pe.Root.Value == "error" {
						currentType = &ast.TypeInfo{Kind: "scalar", Name: "error"}
					}
				}

				for i := 0; i <= elemIdx; i++ {
					if currentType == nil {
						break
					}
					elem := pe.Elements[i]

					if currentType.Kind == "struct" && currentType.Package != "" {
						pkgSymbols, err := getGoPackageSymbols(docDir, currentType.Package)
						if err == nil && pkgSymbols != nil {
							// Se for o elemento final sob o cursor, verificar se é método
							if i == elemIdx {
								if fnSym := findMethod(pkgSymbols, currentType.Name, elem); fnSym != nil {
									paramsList := []string{}
									for k := range fnSym.ParamNames {
										paramsList = append(paramsList, fmt.Sprintf("%s %s", fnSym.ParamNames[k], fnSym.ParamTypes[k]))
									}
									returns := strings.Join(fnSym.ReturnTypes, ", ")
									if len(fnSym.ReturnTypes) > 1 {
										returns = "(" + returns + ")"
									} else if len(fnSym.ReturnTypes) == 0 {
										returns = "void"
									}
									hoverText = fmt.Sprintf("func (s *%s) %s(%s) %s", currentType.Name, fnSym.Name, strings.Join(paramsList, ", "), returns)
									break
								}
							}

							// Verificar se é campo
							typeSym := findStructType(pkgSymbols, currentType.Name)
							if typeSym != nil {
								if fieldTypeStr, exists := typeSym.Fields[elem]; exists {
									if i == elemIdx {
										hoverText = fmt.Sprintf("struct field %s.%s: %s", typeSym.Name, elem, fieldTypeStr)
										break
									} else {
										currentType = resolveFieldTypeInfo(fieldTypeStr, currentType.Package, program)
									}
									continue
								}
								pascalElem := snakeToPascal(elem)
								if fieldTypeStr, exists := typeSym.Fields[pascalElem]; exists {
									if i == elemIdx {
										hoverText = fmt.Sprintf("struct field %s.%s: %s", typeSym.Name, elem, fieldTypeStr)
										break
									} else {
										currentType = resolveFieldTypeInfo(fieldTypeStr, currentType.Package, program)
									}
									continue
								}
							}
						}
					}
					currentType = nil
				}

				if hoverText == "" {
					hoverText = fmt.Sprintf("%s.%s", pe.Root.Value, memberName)
				}
			}
		} else {
			hoverText = pe.String()
		}
	}

	if hoverText == "" {
		return nil, nil
	}

	return &protocol.Hover{
		Contents: &protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: fmt.Sprintf("```heddle\n%s\n```", hoverText),
		},
	}, nil
}

// SignatureHelp implementa textDocument/signatureHelp para preenchimento de parâmetros
func (s *HeddleLspServer) SignatureHelp(ctx context.Context, params *protocol.SignatureHelpParams) (result *protocol.SignatureHelp, err error) {
	uriStr := string(params.TextDocument.URI)
	if err := IsSafeURI(uriStr, s.workspaceRoot); err != nil {
		return nil, err
	}
	doc, ok := s.cache.Get(uriStr)
	if !ok {
		return nil, nil
	}
	program := doc.GetProgram()
	if program == nil {
		return nil, nil
	}

	docFsPath := uri.URI(doc.URI).FsPath()
	docDir := filepath.Dir(docFsPath)

	line, col := LSPPositionToLexer(params.Position)

	var bestCall *ast.CallExpression
	walkAST(program, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpression); ok {
			tok := ce.GetToken()
			if tok.Line == line && col >= tok.Col {
				bestCall = ce
			}
		}
		return true
	})

	if bestCall == nil {
		return nil, nil
	}

	var fnName string
	var pkgAlias string

	if ident, ok := bestCall.Function.(*ast.Identifier); ok {
		fnName = ident.Value
	} else if pe, ok := bestCall.Function.(*ast.PathExpression); ok {
		if pe.Root != nil {
			pkgAlias = pe.Root.Value
		}
		if len(pe.Elements) > 0 {
			fnName = pe.Elements[0]
		}
	}

	var paramNames []string
	var paramTypes []string

	if pkgAlias != "" {
		fn, _ := resolveGoSymbol(docDir, program, pkgAlias, fnName)
		if fn != nil {
			paramNames = fn.ParamNames
			paramTypes = fn.ParamTypes
		}
	}

	if len(paramNames) == 0 {
		return nil, nil
	}

	activeParam := uint32(0)
	for i, arg := range bestCall.Arguments {
		if col >= arg.Value.GetToken().Col {
			activeParam = uint32(i)
		}
	}

	paramsList := []string{}
	var paramInfos []protocol.ParameterInformation
	for i := range paramNames {
		lbl := fmt.Sprintf("%s %s", paramNames[i], paramTypes[i])
		paramsList = append(paramsList, lbl)
		paramInfos = append(paramInfos, protocol.ParameterInformation{Label: protocol.String(lbl)})
	}

	label := fmt.Sprintf("func %s(%s)", fnName, strings.Join(paramsList, ", "))
	sigInfo := protocol.SignatureInformation{
		Label:      label,
		Parameters: paramInfos,
	}

	activeSig := uint32(0)
	return &protocol.SignatureHelp{
		Signatures:      []protocol.SignatureInformation{sigInfo},
		ActiveSignature: &activeSig,
		ActiveParameter: protocol.NewNullable[uint32](activeParam),
	}, nil
}

// DocumentHighlight destaca todas as ocorrências do símbolo sob o cursor
func (s *HeddleLspServer) DocumentHighlight(ctx context.Context, params *protocol.DocumentHighlightParams) (result []protocol.DocumentHighlight, err error) {
	uriStr := string(params.TextDocument.URI)
	if err := IsSafeURI(uriStr, s.workspaceRoot); err != nil {
		return nil, err
	}
	doc, ok := s.cache.Get(uriStr)
	if !ok {
		return nil, nil
	}
	program := doc.GetProgram()
	if program == nil {
		return nil, nil
	}

	line, col := LSPPositionToLexer(params.Position)

	node := findNodeAt(program, line, col)
	if node == nil {
		return nil, nil
	}

	var targetName string
	if ident, ok := node.(*ast.Identifier); ok {
		targetName = ident.Value
	} else if pe, ok := node.(*ast.PathExpression); ok {
		rootSelected, elemIdx := resolvePathHover(pe, col)
		if rootSelected && pe.Root != nil {
			targetName = pe.Root.Value
		} else if elemIdx >= 0 {
			targetName = pe.Elements[elemIdx]
		}
	}

	if targetName == "" {
		return nil, nil
	}

	var highlights []protocol.DocumentHighlight
	walkAST(program, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Identifier); ok && ident.Value == targetName {
			tok := ident.GetToken()
			highlights = append(highlights, protocol.DocumentHighlight{
				Range: LexerTokenToLSPRange(tok),
				Kind:  protocol.DocumentHighlightKindText,
			})
		}
		return true
	})

	return highlights, nil
}

// InlayHint exibe dicas virtuais para nomes de parâmetros posicionais
func (s *HeddleLspServer) InlayHint(ctx context.Context, params *protocol.InlayHintParams) (result []protocol.InlayHint, err error) {
	uriStr := string(params.TextDocument.URI)
	if err := IsSafeURI(uriStr, s.workspaceRoot); err != nil {
		return nil, err
	}
	doc, ok := s.cache.Get(uriStr)
	if !ok {
		return nil, nil
	}
	program := doc.GetProgram()
	if program == nil {
		return nil, nil
	}

	docFsPath := uri.URI(doc.URI).FsPath()
	docDir := filepath.Dir(docFsPath)

	var hints []protocol.InlayHint
	tVal := true

	walkAST(program, func(n ast.Node) bool {
		ce, ok := n.(*ast.CallExpression)
		if !ok {
			return true
		}

		var fnName string
		var pkgAlias string
		if pe, ok := ce.Function.(*ast.PathExpression); ok {
			if pe.Root != nil {
				pkgAlias = pe.Root.Value
			}
			if len(pe.Elements) > 0 {
				fnName = pe.Elements[0]
			}
		}

		if pkgAlias == "" || fnName == "" {
			return true
		}

		fn, _ := resolveGoSymbol(docDir, program, pkgAlias, fnName)
		if fn == nil || len(fn.ParamNames) == 0 {
			return true
		}

		for i, arg := range ce.Arguments {
			if arg.Name == nil && i < len(fn.ParamNames) {
				tok := arg.Value.GetToken()
				hints = append(hints, protocol.InlayHint{
					Position:     LexerToLSPPosition(tok.Line, tok.Col),
					Label:        protocol.String(fn.ParamNames[i] + ":"),
					Kind:         protocol.InlayHintKindParameter,
					PaddingLeft:  &tVal,
					PaddingRight: &tVal,
				})
			}
		}

		return true
	})

	return hints, nil
}

// SemanticTokensFull implementa realce de sintaxe semântico
func (s *HeddleLspServer) SemanticTokensFull(ctx context.Context, params *protocol.SemanticTokensParams) (result *protocol.SemanticTokens, err error) {
	uriStr := string(params.TextDocument.URI)
	if err := IsSafeURI(uriStr, s.workspaceRoot); err != nil {
		return nil, err
	}
	doc, ok := s.cache.Get(uriStr)
	if !ok {
		return nil, nil
	}
	program := doc.GetProgram()
	content := doc.GetContent()

	semanticMap := make(map[tokenPos]int)
	if program != nil {
		buildSemanticMap(program, semanticMap)
	}

	l := lexer.New(content)
	var data []uint32

	prevLine := uint32(0)
	prevCol := uint32(0)

	for {
		tok := l.NextToken()
		if tok.Type == lexer.TokenEOF {
			break
		}

		idx := getTokenTypeIndexWithMap(tok, program, semanticMap)
		if idx < 0 {
			continue
		}

		pos := LexerToLSPPosition(tok.Line, tok.Col)
		line := pos.Line
		col := pos.Character

		deltaLine := line - prevLine
		var deltaStartChar uint32
		if deltaLine == 0 {
			deltaStartChar = col - prevCol
		} else {
			deltaStartChar = col
		}

		length := uint32(len(tok.Literal))

		data = append(data, deltaLine, deltaStartChar, length, uint32(idx), 0)

		prevLine = line
		prevCol = col
	}

	return &protocol.SemanticTokens{
		Data: data,
	}, nil
}

func getTokenTypeIndex(tok lexer.Token, program *ast.Program) int {
	switch tok.Type {
	case lexer.TokenImport, lexer.TokenFlow, lexer.TokenReturn, lexer.TokenHandler:
		return 7 // keyword
	case lexer.TokenAssign, lexer.TokenPipe, lexer.TokenQuestion:
		return 11 // operator
	case lexer.TokenString:
		return 9 // string
	case lexer.TokenInt, lexer.TokenFloat:
		return 10 // number
	case lexer.TokenBool:
		return 13 // boolean (true/false)
	case lexer.TokenIdent:
		if program != nil {
			for _, stmt := range program.Statements {
				switch s := stmt.(type) {
				case *ast.FlowStatement:
					if s.Name != nil && s.Name.Value == tok.Literal {
						return 5 // function (flow)
					}
				case *ast.HandlerStatement:
					if s.Name != nil && s.Name.Value == tok.Literal {
						return 5 // function (handler)
					}
				case *ast.ImportStatement:
					if s.Alias != nil && s.Alias.Value == tok.Literal {
						return 0 // namespace
					}
					parts := strings.Split(s.Path, "/")
					if parts[len(parts)-1] == tok.Literal {
						return 0 // namespace
					}
				}
			}
		}
		return 3 // variable
	default:
		return -1
	}
}

// FoldingRanges calcula os escopos que podem ser ocultados/dobrados
func (s *HeddleLspServer) FoldingRanges(ctx context.Context, params *protocol.FoldingRangeParams) (result []protocol.FoldingRange, err error) {
	uriStr := string(params.TextDocument.URI)
	if err := IsSafeURI(uriStr, s.workspaceRoot); err != nil {
		return nil, err
	}
	doc, ok := s.cache.Get(uriStr)
	if !ok {
		return nil, nil
	}
	program := doc.GetProgram()
	if program == nil {
		return nil, nil
	}

	var ranges []protocol.FoldingRange

	walkAST(program, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BlockStatement:
			start := LexerToLSPPosition(node.GetToken().Line, 1).Line
			end := LexerToLSPPosition(getBlockEndLine(node), 1).Line
			if end > start {
				ranges = append(ranges, protocol.FoldingRange{
					StartLine: start,
					EndLine:   end,
					Kind:      protocol.FoldingRangeKindRegion,
				})
			}
		case *ast.TriggerBlockStatement:
			start := LexerToLSPPosition(node.GetToken().Line, 1).Line
			maxLine := node.GetToken().Line
			for _, c := range node.Cases {
				if endL := getBlockEndLine(c.Body); endL > maxLine {
					maxLine = endL
				}
			}
			end := uint32(maxLine)
			if end > start {
				ranges = append(ranges, protocol.FoldingRange{
					StartLine: start,
					EndLine:   end,
					Kind:      protocol.FoldingRangeKindRegion,
				})
			}
		}
		return true
	})

	return ranges, nil
}

var urlRegex = regexp.MustCompile(`https?://[^\s"'()]+`)

// DocumentLink extrai URLs clicáveis do arquivo
func (s *HeddleLspServer) DocumentLink(ctx context.Context, params *protocol.DocumentLinkParams) (result []protocol.DocumentLink, err error) {
	uriStr := string(params.TextDocument.URI)
	if err := IsSafeURI(uriStr, s.workspaceRoot); err != nil {
		return nil, err
	}
	doc, ok := s.cache.Get(uriStr)
	if !ok {
		return nil, nil
	}
	content := doc.GetContent()

	var links []protocol.DocumentLink

	lines := strings.Split(content, "\n")
	for idx, lineText := range lines {
		matches := urlRegex.FindAllStringIndex(lineText, -1)
		for _, m := range matches {
			startCol := m[0]
			endCol := m[1]
			urlVal := uri.URI(lineText[startCol:endCol])

			links = append(links, protocol.DocumentLink{
				Range: protocol.Range{
					Start: protocol.Position{Line: uint32(idx), Character: uint32(startCol)},
					End:   protocol.Position{Line: uint32(idx), Character: uint32(endCol)},
				},
				Target: &urlVal,
			})
		}
	}

	return links, nil
}

type tokenPos struct {
	line int
	col  int
}

type ASTContext struct {
	IsCalled      bool
	IsStructField bool
}

func walkASTWithContext(node ast.Node, ctx ASTContext, visit func(ast.Node, ASTContext) bool) {
	if isNilNode(node) {
		return
	}
	if !visit(node, ctx) {
		return
	}

	switch n := node.(type) {
	case *ast.Program:
		for _, stmt := range n.Statements {
			walkASTWithContext(stmt, ASTContext{}, visit)
		}
	case *ast.BlockStatement:
		for _, stmt := range n.Statements {
			walkASTWithContext(stmt, ASTContext{}, visit)
		}
	case *ast.FlowStatement:
		walkASTWithContext(n.Name, ASTContext{}, visit)
		walkASTWithContext(n.Param, ASTContext{}, visit)
		walkASTWithContext(n.HandlerName, ASTContext{}, visit)
		walkASTWithContext(n.Body, ASTContext{}, visit)
	case *ast.HandlerStatement:
		walkASTWithContext(n.Name, ASTContext{}, visit)
		walkASTWithContext(n.Param, ASTContext{}, visit)
		walkASTWithContext(n.Body, ASTContext{}, visit)
	case *ast.ImportStatement:
		walkASTWithContext(n.Alias, ASTContext{}, visit)
	case *ast.TriggerBlockStatement:
		walkASTWithContext(n.Trigger, ASTContext{}, visit)
		for _, c := range n.Cases {
			walkASTWithContext(c.Param, ASTContext{}, visit)
			walkASTWithContext(c.Body, ASTContext{}, visit)
		}
	case *ast.ReturnStatement:
		walkASTWithContext(n.ReturnValue, ASTContext{}, visit)
	case *ast.ExpressionStatement:
		walkASTWithContext(n.Expression, ASTContext{}, visit)
	case *ast.AssignExpression:
		walkASTWithContext(n.Name, ASTContext{}, visit)
		walkASTWithContext(n.Value, ASTContext{}, visit)
	case *ast.PipeExpression:
		for _, expr := range n.Expressions {
			walkASTWithContext(expr, ASTContext{}, visit)
		}
	case *ast.PrefixExpression:
		walkASTWithContext(n.Right, ASTContext{}, visit)
	case *ast.PathExpression:
		walkASTWithContext(n.Root, ctx, visit)
	case *ast.CallExpression:
		walkASTWithContext(n.Function, ASTContext{IsCalled: true}, visit)
		for _, arg := range n.Arguments {
			walkASTWithContext(arg.Name, ASTContext{IsStructField: true}, visit)
			walkASTWithContext(arg.Value, ASTContext{}, visit)
		}
	case *ast.MapperExpression:
		walkASTWithContext(n.Path, ASTContext{}, visit)
		walkASTWithContext(n.Alias, ASTContext{}, visit)
		walkASTWithContext(n.Body, ASTContext{}, visit)
	case *ast.MatchExpression:
		walkASTWithContext(n.Target, ASTContext{}, visit)
		for _, c := range n.Cases {
			walkASTWithContext(c.Param, ASTContext{}, visit)
			walkASTWithContext(c.Body, ASTContext{}, visit)
		}
	case *ast.StructInitializer:
		walkASTWithContext(n.Type, ASTContext{}, visit)
		for _, field := range n.Fields {
			walkASTWithContext(field.Name, ASTContext{IsStructField: true}, visit)
			walkASTWithContext(field.Value, ASTContext{}, visit)
		}
	case *ast.TupleExpression:
		for _, expr := range n.Expressions {
			walkASTWithContext(expr, ASTContext{}, visit)
		}
	case *ast.MapLiteral:
		for _, pair := range n.Pairs {
			walkASTWithContext(pair.Key, ASTContext{IsStructField: true}, visit)
			walkASTWithContext(pair.Value, ASTContext{}, visit)
		}
	case *ast.ArrayLiteral:
		for _, expr := range n.Elements {
			walkASTWithContext(expr, ASTContext{}, visit)
		}
	case *ast.FunctionHandlerExpression:
		walkASTWithContext(n.Expr, ASTContext{}, visit)
		walkASTWithContext(n.Handler, ASTContext{}, visit)
	}
}

func isPackageNamespace(program *ast.Program, name string) bool {
	if program == nil {
		return false
	}
	for _, stmt := range program.Statements {
		if imp, ok := stmt.(*ast.ImportStatement); ok {
			alias := ""
			if imp.Alias != nil {
				alias = imp.Alias.Value
			} else {
				parts := strings.Split(imp.Path, "/")
				alias = parts[len(parts)-1]
			}
			if alias == name {
				return true
			}
		}
	}
	return false
}

func resolveVariableType(program *ast.Program, varName string, currentLine int) (pkgAlias string, typeName string) {
	var foundAssign *ast.AssignExpression
	walkAST(program, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignExpression); ok && assign.Name != nil && assign.Name.Value == varName {
			if assign.GetToken().Line <= currentLine {
				foundAssign = assign
			}
		}
		return true
	})

	if foundAssign == nil {
		return "", ""
	}

	switch val := foundAssign.Value.(type) {
	case *ast.StructInitializer:
		if pe, ok := val.Type.(*ast.PathExpression); ok {
			if pe.Root != nil {
				pkgAlias = pe.Root.Value
			}
			if len(pe.Elements) > 0 {
				typeName = pe.Elements[0]
			}
		} else if ident, ok := val.Type.(*ast.Identifier); ok {
			typeName = ident.Value
		}
	}
	return pkgAlias, typeName
}

func buildSemanticMap(program *ast.Program, semanticMap map[tokenPos]int) {
	walkASTWithContext(program, ASTContext{}, func(node ast.Node, ctx ASTContext) bool {
		switch n := node.(type) {
		case *ast.ImportStatement:
			if n.Alias != nil {
				semanticMap[tokenPos{line: n.Alias.Token.Line, col: n.Alias.Token.Col}] = 0 // namespace
			}
		case *ast.FlowStatement:
			if n.Name != nil {
				semanticMap[tokenPos{line: n.Name.Token.Line, col: n.Name.Token.Col}] = 5 // function
			}
			if n.Param != nil {
				semanticMap[tokenPos{line: n.Param.Token.Line, col: n.Param.Token.Col}] = 2 // parameter
			}
			if n.HandlerName != nil {
				semanticMap[tokenPos{line: n.HandlerName.Token.Line, col: n.HandlerName.Token.Col}] = 5 // function
			}
		case *ast.HandlerStatement:
			if n.Name != nil {
				semanticMap[tokenPos{line: n.Name.Token.Line, col: n.Name.Token.Col}] = 5 // function
			}
			if n.Param != nil {
				semanticMap[tokenPos{line: n.Param.Token.Line, col: n.Param.Token.Col}] = 2 // parameter
			}
		case *ast.Identifier:
			pos := tokenPos{line: n.Token.Line, col: n.Token.Col}
			if _, exists := semanticMap[pos]; !exists {
				if ctx.IsCalled {
					semanticMap[pos] = 6 // method
				} else if ctx.IsStructField {
					semanticMap[pos] = 4 // property
				} else if isPackageNamespace(program, n.Value) {
					semanticMap[pos] = 0 // namespace
				} else {
					_, kind := findSymbolDefinition(program, n.Value, n.Token.Line)
					if kind == "parameter" {
						semanticMap[pos] = 2 // parameter
					} else {
						semanticMap[pos] = 3 // variable
					}
				}
			}
		case *ast.PathExpression:
			if n.Root != nil {
				pos := tokenPos{line: n.Root.Token.Line, col: n.Root.Token.Col}
				if isPackageNamespace(program, n.Root.Value) {
					semanticMap[pos] = 0 // namespace
				} else {
					_, kind := findSymbolDefinition(program, n.Root.Value, n.Root.Token.Line)
					if kind == "parameter" {
						semanticMap[pos] = 2 // parameter
					} else {
						semanticMap[pos] = 3 // variable
					}
				}
			}
			line := n.GetToken().Line
			col := n.GetToken().Col
			if n.Root != nil {
				col += len(n.Root.Value)
			}
			for i, elem := range n.Elements {
				col += 1 // for '.'
				pos := tokenPos{line: line, col: col}
				if ctx.IsCalled && i == len(n.Elements)-1 {
					semanticMap[pos] = 6 // method
				} else {
					semanticMap[pos] = 4 // property
				}
				col += len(elem)
			}
		case *ast.MatchExpression:
			for _, c := range n.Cases {
				semanticMap[tokenPos{line: c.Token.Line, col: c.Token.Col}] = 12 // enumMember
			}
		case *ast.TriggerBlockStatement:
			for _, c := range n.Cases {
				semanticMap[tokenPos{line: c.Token.Line, col: c.Token.Col}] = 12 // enumMember
			}
		}
		return true
	})
}

func getTokenTypeIndexWithMap(tok lexer.Token, program *ast.Program, semanticMap map[tokenPos]int) int {
	if tok.Type == lexer.TokenIdent {
		pos := tokenPos{line: tok.Line, col: tok.Col}
		if idx, found := semanticMap[pos]; found {
			return idx
		}
	}
	return getTokenTypeIndex(tok, program)
}
