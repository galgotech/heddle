package semantic

import (
	"fmt"
	"path/filepath"
	"strings"

	"heddle/internal/config"
	"heddle/pkg/lang/ast"
)

type Analyzer struct {
	errors         []AnalysisError
	availablePkgs  map[string]any // Mantido para compatibilidade retroativa
	targetParser   TargetParser
	config         *config.ProjectConfig
	projectRootDir string
	inferredTypes  map[ast.Expression]*ast.TypeInfo
}

func (a *Analyzer) SetProjectRootDir(dir string) {
	a.projectRootDir = dir
}

func New(availablePkgs map[string]any) *Analyzer {
	// Construtor original com GoReflectionParser para compatibilidade retroativa com testes legados
	return &Analyzer{
		errors:        []AnalysisError{},
		availablePkgs: availablePkgs,
		targetParser:  NewGoReflectionParser(availablePkgs),
		inferredTypes: make(map[ast.Expression]*ast.TypeInfo),
	}
}

func NewWithParser(parser TargetParser) *Analyzer {
	return &Analyzer{
		errors:        []AnalysisError{},
		targetParser:  parser,
		inferredTypes: make(map[ast.Expression]*ast.TypeInfo),
	}
}

func (a *Analyzer) Errors() []string {
	var errs []string
	for _, e := range a.errors {
		if e.Node != nil {
			errs = append(errs, fmt.Sprintf("line %s: semantic error: %s", e.Node.TokenLiteral(), e.Msg))
		} else {
			errs = append(errs, e.Msg)
		}
	}
	return errs
}

func (a *Analyzer) DetailedErrors() []AnalysisError {
	return a.errors
}

func (a *Analyzer) error(node ast.Node, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	a.errors = append(a.errors, AnalysisError{Msg: msg, Node: node})
}

func (a *Analyzer) Analyze(program *ast.Program) bool {
	a.inferredTypes = make(map[ast.Expression]*ast.TypeInfo)
	globalTable := NewSymbolTable(nil)

	// Tentar encontrar heddle.toml se ainda não tivermos config carregada
	if a.config == nil {
		var cfg *config.ProjectConfig
		var err error
		if a.projectRootDir != "" {
			cfg, err = config.LoadConfigFromDir(a.projectRootDir)
		} else {
			cfg, err = config.LoadConfig()
		}
		if err == nil {
			a.config = cfg
		}
	}

	// --- ETAPA 1: Análise Semântica Pura do Heddle ---
	importsUsed, symbolsNeeded := a.runHeddleSemanticPass(program, globalTable)
	if len(a.errors) > 0 {
		return false
	}

	// --- ETAPA 2: Busca de Dependências e Resolução de Símbolos ---
	targetPackages := make(map[string]*PackageSymbols)
	var projectRoot string
	if a.projectRootDir != "" {
		projectRoot, _ = config.FindProjectRootFrom(a.projectRootDir)
	} else {
		projectRoot, _ = config.FindProjectRoot()
	}

	for _, path := range importsUsed {
		needed := symbolsNeeded[path]

		// Resolver diretório correspondente da linguagem
		var targetDir string
		if projectRoot != "" {
			langName := "golang"
			if _, ok := a.targetParser.(*GoASTParser); ok {
				langName = "golang"
			}
			if a.config != nil {
				if p, ok := a.config.ResolveLocalPackagePath(projectRoot, langName, ""); ok {
					targetDir = p
				}
			} else {
				if langName == "golang" {
					targetDir = filepath.Join(projectRoot, "pkg/lib")
				}
			}
		}

		pkgSymbols, err := a.targetParser.ParsePackage(path, targetDir, needed)
		if err != nil {
			a.errors = append(a.errors, AnalysisError{Msg: fmt.Sprintf("semantic error: %v", err), Node: nil})
			continue
		}
		targetPackages[path] = pkgSymbols
	}

	if len(a.errors) > 0 {
		return false
	}

	// --- ETAPA 3: Análise Semântica Cruzada e Propagação de Tipos ---
	a.runCrossSemanticPass(program, globalTable, targetPackages)

	return len(a.errors) == 0
}

func (a *Analyzer) runHeddleSemanticPass(program *ast.Program, table *SymbolTable) (map[string]string, map[string][]string) {
	importsUsed := make(map[string]string)
	symbolsNeeded := make(map[string][]string)

	// Primeiro passe: Registrar declarações globais (Imports, Flows, Handlers)
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.ImportStatement:
			path := s.Path
			alias := ""
			if s.Alias != nil {
				alias = s.Alias.Value
			} else {
				// Obter o último segmento do caminho do pacote
				parts := strings.Split(path, "/")
				alias = parts[len(parts)-1]
			}

			if !table.Define(alias, KindPackage, s) {
				a.error(s, "duplicate import or alias name '%s'", alias)
			} else {
				importsUsed[alias] = path
			}

		case *ast.FlowStatement:
			name := s.Name.Value
			if !table.Define(name, KindFlow, s) {
				a.error(s, "duplicate flow name '%s'", name)
			}

		case *ast.HandlerStatement:
			name := s.Name.Value
			if !table.Define(name, KindHandler, s) {
				a.error(s, "duplicate handler name '%s'", name)
			}
		}
	}

	if len(a.errors) > 0 {
		return nil, nil
	}

	// Segundo passe: Validar corpos e escopos internos recursivamente
	hasMain := false
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.FlowStatement:
			if s.Name.Value == "main" {
				hasMain = true
			}
			flowScope := NewSymbolTable(table)
			if s.Param != nil {
				flowScope.Define(s.Param.Value, KindParam, s.Param)
			}
			if s.HandlerName != nil {
				sym, exists := table.Resolve(s.HandlerName.Value)
				if !exists || sym.Kind != KindHandler {
					a.error(s.HandlerName, "undefined handler '%s' for flow '%s'", s.HandlerName.Value, s.Name.Value)
				}
			}
			a.checkBlock(s.Body, flowScope, true, symbolsNeeded, importsUsed)

		case *ast.HandlerStatement:
			handlerScope := NewSymbolTable(table)
			if s.Param != nil {
				handlerScope.Define(s.Param.Value, KindParam, s.Param)
			}
			a.checkBlock(s.Body, handlerScope, true, symbolsNeeded, importsUsed)

		case *ast.ImportStatement:
			// Já processado no primeiro passe

		case *ast.ReturnStatement:
			a.error(s, "return statement not allowed at global scope")

		default:
			// Instruções globais arbitrárias (atribuições globais de servidores, etc.)
			a.checkStatement(stmt, table, false, symbolsNeeded, importsUsed)
		}
	}

	if !hasMain {
		a.errors = append(a.errors, AnalysisError{Msg: "compilation error: flow 'main' must be declared as the program entry point", Node: nil})
	}

	return importsUsed, symbolsNeeded
}

func (a *Analyzer) checkBlock(block *ast.BlockStatement, scope *SymbolTable, inReturnContext bool, symbolsNeeded map[string][]string, importsUsed map[string]string) {
	if block == nil {
		return
	}
	blockScope := NewSymbolTable(scope)
	for _, stmt := range block.Statements {
		a.checkStatement(stmt, blockScope, inReturnContext, symbolsNeeded, importsUsed)
	}
}

func (a *Analyzer) checkStatement(stmt ast.Statement, scope *SymbolTable, inReturnContext bool, symbolsNeeded map[string][]string, importsUsed map[string]string) {
	if stmt == nil {
		return
	}

	switch s := stmt.(type) {
	case *ast.BlockStatement:
		a.checkBlock(s, scope, inReturnContext, symbolsNeeded, importsUsed)

	case *ast.ReturnStatement:
		if !inReturnContext {
			a.error(s, "return statement not allowed outside flow, handler, or callback bodies")
		}
		if s.ReturnValue != nil {
			a.checkExpression(s.ReturnValue, scope, inReturnContext, symbolsNeeded, importsUsed)
		}

	case *ast.ExpressionStatement:
		a.checkExpression(s.Expression, scope, inReturnContext, symbolsNeeded, importsUsed)

	case *ast.TriggerBlockStatement:
		a.checkExpression(s.Trigger, scope, inReturnContext, symbolsNeeded, importsUsed)
		for _, matchCase := range s.Cases {
			caseScope := NewSymbolTable(scope)
			if matchCase.Param != nil {
				caseScope.Define(matchCase.Param.Value, KindParam, matchCase.Param)
			}
			// Callbacks de triggers são contextos válidos para retorno
			a.checkBlock(matchCase.Body, caseScope, true, symbolsNeeded, importsUsed)
		}

	default:
		a.error(stmt, "unsupported statement type")
	}
}

func (a *Analyzer) checkExpression(expr ast.Expression, scope *SymbolTable, inReturnContext bool, symbolsNeeded map[string][]string, importsUsed map[string]string) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.Identifier:
		sym, exists := scope.Resolve(e.Value)
		if !exists {
			a.error(e, "undefined identifier '%s'", e.Value)
		} else if sym.Kind == KindPackage {
			a.error(e, "cannot use package name '%s' directly as value", e.Value)
		}

	case *ast.AssignExpression:
		a.checkExpression(e.Value, scope, inReturnContext, symbolsNeeded, importsUsed)
		// Registrar a variável de destino no escopo atual
		if e.Name != nil {
			scope.Define(e.Name.Value, KindVar, e.Name)
		}

	case *ast.PathExpression:
		if e.Root != nil {
			sym, exists := scope.Resolve(e.Root.Value)
			if !exists {
				a.error(e.Root, "undefined identifier '%s'", e.Root.Value)
				return
			}

			if sym.Kind == KindPackage {
				// Acesso a símbolo de pacote (ex: strings.to_upper)
				pkgPath, ok := importsUsed[e.Root.Value]
				if !ok {
					a.error(e.Root, "undefined package alias '%s'", e.Root.Value)
					return
				}
				// Coletar as funções/símbolos acessados
				for _, elem := range e.Elements {
					symbolsNeeded[pkgPath] = appendUnique(symbolsNeeded[pkgPath], elem)
				}
			}
		}

	case *ast.CallExpression:
		a.checkExpression(e.Function, scope, inReturnContext, symbolsNeeded, importsUsed)
		for _, arg := range e.Arguments {
			a.checkExpression(arg.Value, scope, inReturnContext, symbolsNeeded, importsUsed)
		}

	case *ast.PipeExpression:
		for _, subExpr := range e.Expressions {
			a.checkExpression(subExpr, scope, inReturnContext, symbolsNeeded, importsUsed)
		}

	case *ast.PrefixExpression:
		a.checkExpression(e.Right, scope, inReturnContext, symbolsNeeded, importsUsed)

	case *ast.MapperExpression:
		a.checkExpression(e.Path, scope, inReturnContext, symbolsNeeded, importsUsed)
		mapperScope := NewSymbolTable(scope)
		if e.Alias != nil {
			mapperScope.Define(e.Alias.Value, KindParam, e.Alias)
		}
		a.checkBlock(e.Body, mapperScope, inReturnContext, symbolsNeeded, importsUsed)

	case *ast.MatchExpression:
		a.checkExpression(e.Target, scope, inReturnContext, symbolsNeeded, importsUsed)
		for _, matchCase := range e.Cases {
			caseScope := NewSymbolTable(scope)
			if matchCase.Param != nil {
				caseScope.Define(matchCase.Param.Value, KindParam, matchCase.Param)
			}
			a.checkBlock(matchCase.Body, caseScope, true, symbolsNeeded, importsUsed)
		}

	case *ast.StructInitializer:
		a.checkExpression(e.Type, scope, inReturnContext, symbolsNeeded, importsUsed)
		for _, f := range e.Fields {
			a.checkExpression(f.Value, scope, inReturnContext, symbolsNeeded, importsUsed)
		}

	case *ast.TupleExpression:
		for _, item := range e.Expressions {
			a.checkExpression(item, scope, inReturnContext, symbolsNeeded, importsUsed)
		}

	case *ast.MapLiteral:
		for _, pair := range e.Pairs {
			a.checkExpression(pair.Value, scope, inReturnContext, symbolsNeeded, importsUsed)
		}

	case *ast.ArrayLiteral:
		for _, elem := range e.Elements {
			a.checkExpression(elem, scope, inReturnContext, symbolsNeeded, importsUsed)
		}

	case *ast.FunctionHandlerExpression:
		a.checkExpression(e.Expr, scope, inReturnContext, symbolsNeeded, importsUsed)
		a.checkExpression(e.Handler, scope, inReturnContext, symbolsNeeded, importsUsed)
		if ident, ok := e.Handler.(*ast.Identifier); ok {
			sym, exists := scope.Resolve(ident.Value)
			if !exists || sym.Kind != KindHandler {
				a.error(ident, "undefined error handler '%s'", ident.Value)
			}
		}

	case *ast.StringLiteral, *ast.IntegerLiteral, *ast.FloatLiteral, *ast.BooleanLiteral:
		// Literais básicos são sempre válidos semanticamente

	default:
		// Ignorar desconhecidos
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

func (a *Analyzer) runCrossSemanticPass(program *ast.Program, table *SymbolTable, targetPkgs map[string]*PackageSymbols) {
	for _, stmt := range program.Statements {
		a.checkCrossStatement(stmt, table, targetPkgs, false)
	}
}

func (a *Analyzer) checkCrossStatement(stmt ast.Statement, scope *SymbolTable, targetPkgs map[string]*PackageSymbols, isPipelineReceiver bool) {
	if stmt == nil {
		return
	}

	switch s := stmt.(type) {
	case *ast.BlockStatement:
		for _, subStmt := range s.Statements {
			a.checkCrossStatement(subStmt, scope, targetPkgs, false)
		}

	case *ast.ReturnStatement:
		if s.ReturnValue != nil {
			a.checkCrossExpression(s.ReturnValue, scope, targetPkgs, false)
		}

	case *ast.ExpressionStatement:
		a.checkCrossExpression(s.Expression, scope, targetPkgs, false)

	case *ast.TriggerBlockStatement:
		a.checkCrossExpression(s.Trigger, scope, targetPkgs, false)
		triggerType := a.getType(s.Trigger)
		for _, matchCase := range s.Cases {
			caseScope := NewSymbolTable(scope)
			if matchCase.Param != nil {
				caseScope.Define(matchCase.Param.Value, KindParam, matchCase.Param)
				if triggerType != nil {
					paramTypeStr := extractCallbackParamType(triggerType.Name, matchCase.Tag)
					if paramTypeStr != "any" {
						paramTypeInfo := resolveFieldTypeInfoInAnalyzer(paramTypeStr, triggerType.Package, targetPkgs)
						a.setType(matchCase.Param, paramTypeInfo)
					}
				}
			}
			a.checkCrossStatement(matchCase.Body, caseScope, targetPkgs, false)
		}

	case *ast.FlowStatement:
		a.checkCrossStatement(s.Body, scope, targetPkgs, false)

	case *ast.HandlerStatement:
		a.checkCrossStatement(s.Body, scope, targetPkgs, false)
	}
}

func (a *Analyzer) checkCrossExpression(expr ast.Expression, scope *SymbolTable, targetPkgs map[string]*PackageSymbols, isPipelineReceiver bool) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.Identifier:
		// Se existir na tabela de símbolos com tipo anotado, propagamos
		if sym, found := scope.Resolve(e.Value); found {
			if ident, ok := sym.Node.(*ast.Identifier); ok {
				if t := a.getType(ident); t != nil {
					a.setType(e, t)
				}
			}
		}

	case *ast.AssignExpression:
		a.checkCrossExpression(e.Value, scope, targetPkgs, false)
		// Propagar o tipo inferido da expressão atribuída para a variável de destino
		if valType := a.getType(e.Value); valType != nil {
			a.setType(e, valType)
			if e.Name != nil {
				a.setType(e.Name, valType)
				if sym, found := scope.Resolve(e.Name.Value); found {
					if ident, ok := sym.Node.(*ast.Identifier); ok {
						a.setType(ident, valType)
					}
				}
			}
		}

	case *ast.PathExpression:
		if e.Root != nil {
			if sym, found := scope.Resolve(e.Root.Value); found {
				if ident, ok := sym.Node.(*ast.Identifier); ok {
					if t := a.getType(ident); t != nil {
						a.setType(e.Root, t)
					}
				}
			}

			rootType := a.getType(e.Root)
			currentType := rootType
			for _, elem := range e.Elements {
				if currentType == nil {
					break
				}
				if currentType.Kind == "struct" && currentType.Package != "" {
					pkgSymbols := targetPkgs[currentType.Package]
					if pkgSymbols != nil {
						// Verificar se é campo
						typeSym := findStructTypeInAnalyzer(pkgSymbols, currentType.Name)
						if typeSym != nil {
							var fieldTypeStr string
							var exists bool
							if fieldTypeStr, exists = typeSym.Fields[elem]; !exists {
								pascalElem := snakeToPascal(elem)
								fieldTypeStr, exists = typeSym.Fields[pascalElem]
							}
							if exists {
								currentType = resolveFieldTypeInfoInAnalyzer(fieldTypeStr, currentType.Package, targetPkgs)
								continue
							}
						}
						// Verificar se é método
						if fnSym := findMethodInAnalyzer(pkgSymbols, currentType.Name, elem); fnSym != nil {
							if len(fnSym.ReturnTypes) > 0 {
								retTypeName := fnSym.ReturnTypes[0]
								kind := "scalar"
								if strings.HasPrefix(retTypeName, "[]") {
									kind = "slice"
								} else if strings.HasPrefix(retTypeName, "map[") {
									kind = "map"
								} else if retTypeName == "error" {
									kind = "error"
								} else if retTypeName == "any" {
									kind = "generic"
								} else if isStructTypeName(retTypeName) {
									kind = "struct"
								}
								currentType = &ast.TypeInfo{
									Kind:    kind,
									Name:    retTypeName,
									Package: pkgSymbols.Path,
								}
								continue
							}
						}
					}
				}
				currentType = nil
			}
			if currentType != nil {
				a.setType(e, currentType)
			}
		}

	case *ast.CallExpression:
		// Primeiro validar os argumentos
		for _, arg := range e.Arguments {
			a.checkCrossExpression(arg.Value, scope, targetPkgs, false)
		}
		a.checkCrossExpression(e.Function, scope, targetPkgs, false)

		// Validar a chamada contra os símbolos da linguagem target
		if pathExpr, ok := e.Function.(*ast.PathExpression); ok && pathExpr.Root != nil {
			sym, exists := scope.Resolve(pathExpr.Root.Value)
			if exists {
				if sym.Kind == KindPackage {
					pkgPath, ok := importsUsedInScope(scope, pathExpr.Root.Value, targetPkgs)
					if ok && pkgPath != nil && len(pathExpr.Elements) > 0 {
						funcName := pathExpr.Elements[0]
						funcMeta := pkgPath.Funcs[funcName]
						if funcMeta == nil {
							funcMeta = pkgPath.Funcs[snakeToPascal(funcName)]
						}
						if funcMeta == nil {
							funcMeta = pkgPath.Funcs[strings.ToLower(funcName)]
						}

						if funcMeta != nil {
							// Validar contagem de argumentos
							expected := len(funcMeta.ParamTypes)
							actual := len(e.Arguments)
							if isPipelineReceiver {
								actual++
								if actual > expected {
									a.error(e, "argument count mismatch for function '%s': expected at most %d, got %d", funcName, expected, actual)
								}
							} else {
								if actual != expected {
									a.error(e, "argument count mismatch for function '%s': expected %d, got %d", funcName, expected, actual)
								}
							}

							// Anotar tipo de retorno da função na AST
							if len(funcMeta.ReturnTypes) > 0 {
								retTypeName := funcMeta.ReturnTypes[0]
								kind := "scalar"
								if strings.HasPrefix(retTypeName, "[]") {
									kind = "slice"
								} else if strings.HasPrefix(retTypeName, "map[") {
									kind = "map"
								} else if retTypeName == "error" {
									kind = "error"
								} else if retTypeName == "any" {
									kind = "generic"
								} else if isStructTypeName(retTypeName) {
									kind = "struct"
								}

								typeInfo := &ast.TypeInfo{
									Kind:    kind,
									Name:    retTypeName,
									Package: pkgPath.Path,
								}
								a.setType(e, typeInfo)
								a.setType(pathExpr, typeInfo)
							}
						}
					}
				} else {
					// Chamada de método em um objeto/variável local/parâmetro
					var rootType *ast.TypeInfo
					if ident, ok := sym.Node.(*ast.Identifier); ok {
						rootType = a.getType(ident)
					}
					if rootType == nil {
						if pathExpr.Root.Value == "request" || pathExpr.Root.Value == "req" {
							rootType = &ast.TypeInfo{Kind: "struct", Name: "Request", Package: "net/http"}
						}
					}
					if rootType != nil && rootType.Kind == "struct" && rootType.Package != "" && len(pathExpr.Elements) > 0 {
						pkgPath := targetPkgs[rootType.Package]
						if pkgPath != nil {
							methodName := pathExpr.Elements[0]
							funcMeta := findMethodInAnalyzer(pkgPath, rootType.Name, methodName)
							if funcMeta != nil {
								// Validar contagem de argumentos
								expected := len(funcMeta.ParamTypes)
								actual := len(e.Arguments)
								if isPipelineReceiver {
									actual++
								}
								if actual != expected {
									a.error(e, "argument count mismatch for method '%s': expected %d, got %d", methodName, expected, actual)
								}

								// Anotar tipo de retorno da função na AST
								if len(funcMeta.ReturnTypes) > 0 {
									retTypeName := funcMeta.ReturnTypes[0]
									kind := "scalar"
									if strings.HasPrefix(retTypeName, "[]") {
										kind = "slice"
									} else if strings.HasPrefix(retTypeName, "map[") {
										kind = "map"
									} else if retTypeName == "error" {
										kind = "error"
									} else if retTypeName == "any" {
										kind = "generic"
									} else if isStructTypeName(retTypeName) {
										kind = "struct"
									}

									typeInfo := &ast.TypeInfo{
										Kind:    kind,
										Name:    retTypeName,
										Package: pkgPath.Path,
									}
									a.setType(e, typeInfo)
									a.setType(pathExpr, typeInfo)
								}
							}
						}
					}
				}
			}
		}

	case *ast.PipeExpression:
		var lastType *ast.TypeInfo
		for i, subExpr := range e.Expressions {
			a.checkCrossExpression(subExpr, scope, targetPkgs, i > 0)
			lastType = a.getType(subExpr)
		}
		if lastType != nil {
			a.setType(e, lastType)
		}

	case *ast.FunctionHandlerExpression:
		a.checkCrossExpression(e.Expr, scope, targetPkgs, isPipelineReceiver)
		a.checkCrossExpression(e.Handler, scope, targetPkgs, false)

		// Copiar tipo da expressão da esquerda
		if t := a.getType(e.Expr); t != nil {
			a.setType(e, t)
		}

		// Verificar se a chamada do lado esquerdo retorna erro
		if callExpr, ok := e.Expr.(*ast.CallExpression); ok {
			if pathExpr, ok := callExpr.Function.(*ast.PathExpression); ok && pathExpr.Root != nil {
				sym, exists := scope.Resolve(pathExpr.Root.Value)
				if exists && sym.Kind == KindPackage {
					pkgPath, ok := importsUsedInScope(scope, pathExpr.Root.Value, targetPkgs)
					if ok && pkgPath != nil && len(pathExpr.Elements) > 0 {
						funcName := pathExpr.Elements[0]
						funcMeta := pkgPath.Funcs[funcName]
						if funcMeta == nil {
							funcMeta = pkgPath.Funcs[snakeToPascal(funcName)]
						}
						if funcMeta == nil {
							funcMeta = pkgPath.Funcs[strings.ToLower(funcName)]
						}

						if funcMeta != nil {
							if !funcMeta.HasErrorReturn {
								a.error(e, "function '%s' does not return an error, cannot apply handler", funcName)
							}
						}
					}
				}
			}
		}

	case *ast.StructInitializer:
		if pathExpr, ok := e.Type.(*ast.PathExpression); ok && pathExpr.Root != nil {
			sym, exists := scope.Resolve(pathExpr.Root.Value)
			if exists && sym.Kind == KindPackage {
				pkgPath, ok := importsUsedInScope(scope, pathExpr.Root.Value, targetPkgs)
				if ok && pkgPath != nil && len(pathExpr.Elements) > 0 {
					typeName := pathExpr.Elements[0]
					pascalTypeName := snakeToPascal(typeName)

					var structType *TypeSymbol
					var exists bool
					structType, exists = pkgPath.Types[typeName]
					if !exists {
						structType, exists = pkgPath.Types[pascalTypeName]
					}
					if !exists {
						structType, exists = pkgPath.Types[strings.ToLower(typeName)]
					}
					if !exists {
						for k, t := range pkgPath.Types {
							if strings.HasPrefix(k, pascalTypeName) || strings.HasSuffix(k, pascalTypeName) || strings.EqualFold(k, pascalTypeName) {
								structType = t
								break
							}
						}
					}

					if structType == nil {
						// Tentar como construtor/função (ex: "secret" -> "NewSecret")
						funcName := typeName
						funcMeta := pkgPath.Funcs[funcName]
						if funcMeta == nil {
							funcMeta = pkgPath.Funcs[snakeToPascal(funcName)]
						}
						if funcMeta == nil {
							funcMeta = pkgPath.Funcs[strings.ToLower(funcName)]
						}
						if funcMeta != nil && len(funcMeta.ReturnTypes) > 0 {
							retTypeName := strings.TrimPrefix(funcMeta.ReturnTypes[0], "*")
							structType = findStructTypeInAnalyzer(pkgPath, retTypeName)
						}
					}

					if structType != nil {
						typeInfo := &ast.TypeInfo{
							Kind:    "struct",
							Name:    structType.Name,
							Package: pkgPath.Path,
						}
						a.setType(e, typeInfo)
						a.setType(pathExpr, typeInfo)

						for _, f := range e.Fields {
							fieldName := snakeToPascal(f.Name.Value)
							_, fieldExists := structType.Fields[fieldName]
							if !fieldExists {
								_, fieldExists = structType.Fields[f.Name.Value]
							}
							if !fieldExists {
								a.error(f.Name, "unknown field '%s' for struct '%s'", f.Name.Value, typeName)
							}
							a.checkCrossExpression(f.Value, scope, targetPkgs, false)
						}
					}
				}
			}
		}

	case *ast.MatchExpression:
		a.checkCrossExpression(e.Target, scope, targetPkgs, isPipelineReceiver)
		targetType := a.getType(e.Target)
		for _, matchCase := range e.Cases {
			caseScope := NewSymbolTable(scope)
			if matchCase.Param != nil {
				caseScope.Define(matchCase.Param.Value, KindParam, matchCase.Param)
				if targetType != nil && targetType.Kind == "struct" && targetType.Package != "" {
					pkgSymbols := targetPkgs[targetType.Package]
					if pkgSymbols != nil {
						typeSym := findStructTypeInAnalyzer(pkgSymbols, targetType.Name)
						if typeSym != nil {
							var fieldTypeStr string
							var exists bool
							if fieldTypeStr, exists = typeSym.Fields[matchCase.Tag]; !exists {
								pascalTag := snakeToPascal(matchCase.Tag)
								fieldTypeStr, exists = typeSym.Fields[pascalTag]
							}
							if exists {
								fieldTypeInfo := resolveFieldTypeInfoInAnalyzer(fieldTypeStr, targetType.Package, targetPkgs)
								a.setType(matchCase.Param, fieldTypeInfo)
							}
						}
					}
				}
			}
			a.checkCrossStatement(matchCase.Body, caseScope, targetPkgs, false)
		}

	case *ast.MapperExpression:
		a.checkCrossExpression(e.Path, scope, targetPkgs, false)
		a.checkCrossStatement(e.Body, scope, targetPkgs, false)

	case *ast.TupleExpression:
		if isPipelineReceiver {
			a.error(e, "tuple expression cannot be used as a pipeline receiver")
		}
		for _, sub := range e.Expressions {
			a.checkCrossExpression(sub, scope, targetPkgs, false)
		}

	case *ast.MapLiteral:
		a.setType(e, &ast.TypeInfo{Kind: "map", Name: "map"})
		for _, pair := range e.Pairs {
			a.checkCrossExpression(pair.Value, scope, targetPkgs, false)
		}

	case *ast.ArrayLiteral:
		a.setType(e, &ast.TypeInfo{Kind: "slice", Name: "array"})
		for _, elem := range e.Elements {
			a.checkCrossExpression(elem, scope, targetPkgs, false)
		}

	case *ast.StringLiteral:
		a.setType(e, &ast.TypeInfo{Kind: "scalar", Name: "string"})
	case *ast.IntegerLiteral:
		a.setType(e, &ast.TypeInfo{Kind: "scalar", Name: "int"})
	case *ast.FloatLiteral:
		a.setType(e, &ast.TypeInfo{Kind: "scalar", Name: "float64"})
	case *ast.BooleanLiteral:
		a.setType(e, &ast.TypeInfo{Kind: "scalar", Name: "bool"})
	}
}

func importsUsedInScope(scope *SymbolTable, alias string, targetPkgs map[string]*PackageSymbols) (*PackageSymbols, bool) {
	sym, exists := scope.Resolve(alias)
	if !exists || sym.Kind != KindPackage {
		return nil, false
	}
	if imp, ok := sym.Node.(*ast.ImportStatement); ok {
		pkgMeta, found := targetPkgs[imp.Path]
		return pkgMeta, found
	}
	return nil, false
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

var commonInitialisms = map[string]string{
	"http": "HTTP",
	"url":  "URL",
	"id":   "ID",
	"json": "JSON",
	"xml":  "XML",
	"ip":   "IP",
	"tcp":  "TCP",
	"udp":  "UDP",
	"html": "HTML",
}

func snakeToPascal(s string) string {
	parts := strings.Split(s, "_")
	for i, part := range parts {
		if len(part) > 0 {
			lower := strings.ToLower(part)
			if replacement, exists := commonInitialisms[lower]; exists {
				parts[i] = replacement
			} else {
				parts[i] = strings.ToUpper(part[0:1]) + part[1:]
			}
		}
	}
	return strings.Join(parts, "")
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

func (a *Analyzer) InferredTypes() map[ast.Expression]*ast.TypeInfo {
	return a.inferredTypes
}

func (a *Analyzer) getType(expr ast.Expression) *ast.TypeInfo {
	if expr == nil {
		return nil
	}
	return a.inferredTypes[expr]
}

func (a *Analyzer) setType(expr ast.Expression, t *ast.TypeInfo) {
	if expr == nil {
		return
	}
	a.inferredTypes[expr] = t
}

func findStructTypeInAnalyzer(pkgSymbols *PackageSymbols, typeName string) *TypeSymbol {
	pascal := snakeToPascal(typeName)
	snake := pascalToSnake(typeName)
	variations := []string{typeName, pascal, snake, strings.ToLower(typeName)}
	for _, v := range variations {
		if t, ok := pkgSymbols.Types[v]; ok {
			return t
		}
	}
	// Tentar correspondência parcial/sufixo
	for k, t := range pkgSymbols.Types {
		if strings.HasPrefix(k, pascal) || strings.HasSuffix(k, pascal) || strings.EqualFold(k, pascal) {
			return t
		}
	}
	return nil
}

func findMethodInAnalyzer(pkgSymbols *PackageSymbols, receiverType, methodName string) *FuncSymbol {
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

func resolveFieldTypeInfoInAnalyzer(fieldTypeStr string, parentPackage string, targetPkgs map[string]*PackageSymbols) *ast.TypeInfo {
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

		for path := range targetPkgs {
			parts := strings.Split(path, "/")
			last := parts[len(parts)-1]
			if last == pkgAlias {
				pkgPath = path
				break
			}
		}
	}

	return &ast.TypeInfo{
		Kind:    "struct",
		Name:    name,
		Package: pkgPath,
	}
}

func extractCallbackParamType(genericTypeStr string, caseTag string) string {
	if strings.Contains(genericTypeStr, "Seq[") {
		idx := strings.Index(genericTypeStr, "[")
		if idx == -1 {
			return "any"
		}
		partsStr := strings.TrimSuffix(genericTypeStr[idx+1:], "]")
		parts := strings.Split(partsStr, ",")
		if len(parts) >= 2 {
			return strings.TrimSpace(parts[1])
		}
	} else if strings.Contains(genericTypeStr, "ReqReply[") {
		idx := strings.Index(genericTypeStr, "[")
		if idx == -1 {
			return "any"
		}
		partsStr := strings.TrimSuffix(genericTypeStr[idx+1:], "]")
		parts := strings.Split(partsStr, ",")
		if len(parts) >= 2 {
			return strings.TrimSpace(parts[1])
		}
	}
	return "any"
}
