package semantic

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type GoASTParser struct{}

// stdlibRemap mapeia caminhos curtos ou aliases comuns do Heddle para a biblioteca padrão do Go
var stdlibRemap = map[string]string{
	"json": "encoding/json",
	"xml":  "encoding/xml",
	"rand": "math/rand",
	"http": "net/http",
}

func (gp *GoASTParser) ParsePackage(pkgPath string, targetDir string, symbolsNeeded []string) (*PackageSymbols, error) {
	// 1. Mapear caminhos do stdlib se necessário
	resolvedPath := pkgPath
	if remapped, exists := stdlibRemap[pkgPath]; exists {
		resolvedPath = remapped
	}

	var dirPath string
	isStd := false

	// 2. Verificar se o pacote é local no diretório da linguagem de destino
	if targetDir != "" {
		localPath := filepath.Join(targetDir, resolvedPath)
		if info, err := os.Stat(localPath); err == nil && info.IsDir() {
			dirPath = localPath
		} else {
			// Tentar resolver retirando o prefixo do módulo
			if modName := getGoModuleName(targetDir); modName != "" {
				prefix := modName + "/"
				if strings.HasPrefix(resolvedPath, prefix) {
					subPath := strings.TrimPrefix(resolvedPath, prefix)
					localPathOpt := filepath.Join(targetDir, subPath)
					if info, err := os.Stat(localPathOpt); err == nil && info.IsDir() {
						dirPath = localPathOpt
					}
				}
			}
		}
	}

	// 2.5 Se não for local, verificar na biblioteca padrão do Heddle
	if dirPath == "" {
		if stdlibPath := findHeddleStdlibPath(); stdlibPath != "" {
			heddleStdPath := filepath.Join(stdlibPath, resolvedPath)
			if info, err := os.Stat(heddleStdPath); err == nil && info.IsDir() {
				dirPath = heddleStdPath
				isStd = true
			}
		}
	}

	// 3. Se não for local, buscar no GOROOT (biblioteca padrão)
	if dirPath == "" {
		goroot := runtime.GOROOT()
		if goroot != "" {
			stdPath := filepath.Join(goroot, "src", resolvedPath)
			if info, err := os.Stat(stdPath); err == nil && info.IsDir() {
				dirPath = stdPath
				isStd = true
			}
		}
	}

	if dirPath == "" {
		return nil, fmt.Errorf("package '%s' not found locally or in Go standard library", pkgPath)
	}

	// 4. Fazer parse do diretório
	fset := token.NewFileSet()
	filter := func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}

	pkgs, err := parser.ParseDir(fset, dirPath, filter, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse directory '%s': %w", dirPath, err)
	}

	pkgSymbols := &PackageSymbols{
		Path:  pkgPath,
		Funcs: make(map[string]*FuncSymbol),
		Types: make(map[string]*TypeSymbol),
	}

	// 5. Extrair os símbolos
	for _, pkg := range pkgs {
		for filePath, file := range pkg.Files {
			for _, decl := range file.Decls {
				switch d := decl.(type) {
				case *ast.FuncDecl:
					// Apenas funções exportadas (iniciando com maiúscula)
					if ast.IsExported(d.Name.Name) {
						gp.extractFuncSymbol(fset, filePath, d, pkgSymbols)
					}

				case *ast.GenDecl:
					if d.Tok == token.TYPE {
						for _, spec := range d.Specs {
							if typeSpec, ok := spec.(*ast.TypeSpec); ok {
								if ast.IsExported(typeSpec.Name.Name) {
									if structType, ok := typeSpec.Type.(*ast.StructType); ok {
										gp.extractStructSymbol(fset, filePath, typeSpec.Name.Name, structType, pkgSymbols)
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// 6. Tratar inicializadores implícitos ("New<Type>") para structs locais.
	// Se tivermos um struct "Server" e uma função "NewServer", mapeamos "server" para "NewServer" como construtor.
	tempFuncs := make(map[string]*FuncSymbol)
	tempTypes := make(map[string]*TypeSymbol)

	for typeName, structSym := range pkgSymbols.Types {
		constructorName := "New" + typeName
		if fnSym, exists := pkgSymbols.Funcs[constructorName]; exists {
			// Mapeia o construtor também com o nome em minúsculo do tipo (ex: "server")
			tempFuncs[strings.ToLower(typeName)] = fnSym
			tempFuncs[snakeToPascal(strings.ToLower(typeName))] = fnSym
		}
		// Também registrar tipos de forma flexível (snake_case/PascalCase/lowercase)
		tempTypes[strings.ToLower(typeName)] = structSym
		tempTypes[snakeToPascal(strings.ToLower(typeName))] = structSym
		tempTypes[pascalToSnake(typeName)] = structSym
	}

	for k, v := range tempTypes {
		pkgSymbols.Types[k] = v
	}

	// Adicionar remapeamento de nomes de funções exportadas para minúsculo/snake_case
	// para que a busca pelo Heddle (que usa snake_case ex: `to_upper`) funcione contra PascalCase Go (`ToUpper`).
	for fnName, fnSym := range pkgSymbols.Funcs {
		// Se for um construtor "NewXXX", mapear "xxx" / "XXX" para ele
		if strings.HasPrefix(fnName, "New") && len(fnName) > 3 {
			suffix := fnName[3:]
			tempFuncs[strings.ToLower(suffix)] = fnSym
			tempFuncs[snakeToPascal(strings.ToLower(suffix))] = fnSym
			tempFuncs[pascalToSnake(suffix)] = fnSym
		}

		snakeName := pascalToSnake(fnName)
		if snakeName != fnName {
			tempFuncs[snakeName] = fnSym
			tempFuncs[strings.ToLower(fnName)] = fnSym
		}
	}

	for k, v := range tempFuncs {
		pkgSymbols.Funcs[k] = v
	}

	// Se for a biblioteca padrão do Go e o pacote for "io", injetamos mock de print/println se não existirem
	// pois no heddle os fluxos chamam "io.print" que é injetado pelo runtime.
	if isStd && pkgPath == "io" {
		if _, exists := pkgSymbols.Funcs["print"]; !exists {
			pkgSymbols.Funcs["print"] = &FuncSymbol{
				Name:        "Print",
				ParamNames:  []string{"val"},
				ParamTypes:  []string{"any"},
				ReturnTypes: []string{},
				IsGenericFn: true,
			}
		}
	}

	return pkgSymbols, nil
}

func getReceiverTypeName(recv ast.Expr) string {
	if recv == nil {
		return ""
	}
	switch t := recv.(type) {
	case *ast.StarExpr:
		return getReceiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr: // suporte para estruturas genéricas
		return getReceiverTypeName(t.X)
	case *ast.IndexListExpr: // suporte para genéricos com múltiplos parâmetros
		return getReceiverTypeName(t.X)
	}
	return ""
}

func (gp *GoASTParser) extractFuncSymbol(fset *token.FileSet, filePath string, d *ast.FuncDecl, pkgSymbols *PackageSymbols) {
	fnName := d.Name.Name
	pos := fset.Position(d.Pos())
	sym := &FuncSymbol{
		Name:        fnName,
		ParamNames:  []string{},
		ParamTypes:  []string{},
		ReturnTypes: []string{},
		FilePath:    filePath,
		Line:        pos.Line,
		Col:         pos.Column,
	}

	// Extrair receiver se for um método
	if d.Recv != nil && len(d.Recv.List) > 0 {
		sym.Receiver = getReceiverTypeName(d.Recv.List[0].Type)
	}

	// Extrair parâmetros
	if d.Type.Params != nil {
		for _, field := range d.Type.Params.List {
			typeStr := astTypeToString(field.Type)
			if len(field.Names) == 0 {
				sym.ParamNames = append(sym.ParamNames, "_")
				sym.ParamTypes = append(sym.ParamTypes, typeStr)
			} else {
				for _, name := range field.Names {
					sym.ParamNames = append(sym.ParamNames, name.Name)
					sym.ParamTypes = append(sym.ParamTypes, typeStr)
				}
			}
		}
	}

	// Extrair retornos
	if d.Type.Results != nil {
		for _, field := range d.Type.Results.List {
			typeStr := astTypeToString(field.Type)
			if len(field.Names) == 0 {
				sym.ReturnTypes = append(sym.ReturnTypes, typeStr)
			} else {
				for range field.Names {
					sym.ReturnTypes = append(sym.ReturnTypes, typeStr)
				}
			}
		}
	}

	// Definir flags de compatibilidade do Heddle
	if len(sym.ParamTypes) > 0 {
		firstParamType := sym.ParamTypes[0]
		if strings.HasPrefix(firstParamType, "[]") {
			sym.IsSliceFn = true
		} else if firstParamType == "any" {
			sym.IsGenericFn = true
		} else {
			sym.IsScalarFn = true
		}
	}

	// Verificar se retorna error
	if len(sym.ReturnTypes) > 0 {
		lastRetType := sym.ReturnTypes[len(sym.ReturnTypes)-1]
		if lastRetType == "error" {
			sym.HasErrorReturn = true
		}
	}

	if sym.Receiver != "" {
		key := sym.Receiver + "." + fnName
		pkgSymbols.Funcs[key] = sym
		
		// Também registrar variações de case para flexibilidade de chamada no Heddle
		pkgSymbols.Funcs[sym.Receiver + "." + strings.ToLower(fnName)] = sym
		pkgSymbols.Funcs[sym.Receiver + "." + pascalToSnake(fnName)] = sym
	} else {
		pkgSymbols.Funcs[fnName] = sym
	}
}

func (gp *GoASTParser) extractStructSymbol(fset *token.FileSet, filePath string, name string, s *ast.StructType, pkgSymbols *PackageSymbols) {
	pos := fset.Position(s.Pos())
	sym := &TypeSymbol{
		Name:     name,
		Fields:   make(map[string]string),
		FilePath: filePath,
		Line:     pos.Line,
		Col:      pos.Column,
	}

	if s.Fields != nil {
		for _, field := range s.Fields.List {
			typeStr := astTypeToString(field.Type)
			if len(field.Names) == 0 {
				// Campo embutido
				typeName := typeStr
				if idx := strings.LastIndex(typeName, "."); idx != -1 {
					typeName = typeName[idx+1:]
				}
				sym.Fields[typeName] = typeStr
			} else {
				for _, fName := range field.Names {
					sym.Fields[fName.Name] = typeStr
					sym.Fields[strings.ToLower(fName.Name)] = typeStr
					sym.Fields[pascalToSnake(fName.Name)] = typeStr
				}
			}
		}
	}

	pkgSymbols.Types[name] = sym
}

func astTypeToString(expr ast.Expr) string {
	if expr == nil {
		return "unknown"
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.ArrayType:
		return "[]" + astTypeToString(e.Elt)
	case *ast.MapType:
		return "map[" + astTypeToString(e.Key) + "]" + astTypeToString(e.Value)
	case *ast.StarExpr:
		return astTypeToString(e.X)
	case *ast.SelectorExpr:
		return astTypeToString(e.X) + "." + e.Sel.Name
	case *ast.InterfaceType:
		return "any"
	case *ast.IndexExpr:
		return fmt.Sprintf("%s[%s]", astTypeToString(e.X), astTypeToString(e.Index))
	case *ast.IndexListExpr:
		var indices []string
		for _, idx := range e.Indices {
			indices = append(indices, astTypeToString(idx))
		}
		return fmt.Sprintf("%s[%s]", astTypeToString(e.X), strings.Join(indices, ", "))
	default:
		return "unknown"
	}
}

func findHeddleStdlibPath() string {
	if exePath, err := os.Executable(); err == nil {
		dir := filepath.Dir(exePath)
		for {
			stdPath := filepath.Join(dir, "pkg", "lib")
			if info, err := os.Stat(stdPath); err == nil && info.IsDir() {
				if _, err := os.Stat(filepath.Join(stdPath, "net", "http", "http.go")); err == nil {
					return stdPath
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	if dir, err := os.Getwd(); err == nil {
		for {
			stdPath := filepath.Join(dir, "pkg", "lib")
			if info, err := os.Stat(stdPath); err == nil && info.IsDir() {
				if _, err := os.Stat(filepath.Join(stdPath, "net", "http", "http.go")); err == nil {
					return stdPath
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	commonPaths := []string{
		"/home/andre/Projects/galgotech/fhub/heddle-pure/heddle-pure/pkg/lib",
	}
	for _, p := range commonPaths {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}

	return ""
}

func getGoModuleName(targetDir string) string {
	goModPath := filepath.Join(targetDir, "go.mod")
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return ""
}
