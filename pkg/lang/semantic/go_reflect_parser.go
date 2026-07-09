package semantic

import (
	"fmt"
	"reflect"
	"strings"
)

type GoReflectionParser struct {
	availablePkgs map[string]any
}

func NewGoReflectionParser(available map[string]any) *GoReflectionParser {
	return &GoReflectionParser{availablePkgs: available}
}

func (rp *GoReflectionParser) ParsePackage(pkgPath string, targetDir string, symbolsNeeded []string) (*PackageSymbols, error) {
	goPkg, exists := rp.availablePkgs[pkgPath]
	if !exists {
		return nil, fmt.Errorf("package '%s' was not found in the Go environment", pkgPath)
	}

	pkgSymbols := &PackageSymbols{
		Path:  pkgPath,
		Funcs: make(map[string]*FuncSymbol),
		Types: make(map[string]*TypeSymbol),
	}

	origVal := reflect.ValueOf(goPkg)
	val := reflect.ValueOf(goPkg)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	typ := val.Type()

	if typ.Kind() == reflect.Map {
		for _, symName := range symbolsNeeded {
			var fieldVal reflect.Value
			pascalName := snakeToPascal(symName)

			fieldVal = val.MapIndex(reflect.ValueOf(symName))
			if !fieldVal.IsValid() {
				fieldVal = val.MapIndex(reflect.ValueOf(pascalName))
			}
			if !fieldVal.IsValid() {
				fieldVal = val.MapIndex(reflect.ValueOf(snakeToPascal(symName)))
			}
			if !fieldVal.IsValid() {
				// Suporte a Construtor de Struct (ex: se pediu "server", buscar "NewServer")
				constructorName := "New" + pascalName
				fieldVal = val.MapIndex(reflect.ValueOf(constructorName))
			}
			if !fieldVal.IsValid() {
				constructorName := "new" + symName
				fieldVal = val.MapIndex(reflect.ValueOf(constructorName))
			}

			if !fieldVal.IsValid() {
				return nil, fmt.Errorf("symbol '%s' not found in package '%s'", symName, pkgPath)
			}

			var fnVal reflect.Value
			if fieldVal.Kind() == reflect.Func {
				fnVal = fieldVal
			} else if fieldVal.Kind() == reflect.Interface && !fieldVal.IsNil() && fieldVal.Elem().Kind() == reflect.Func {
				fnVal = fieldVal.Elem()
			}

			if fnVal.IsValid() {
				meta := rp.analyzeGoFunction(symName, fnVal)
				pkgSymbols.Funcs[symName] = meta
				pkgSymbols.Funcs[pascalName] = meta
				pkgSymbols.Funcs[snakeToPascal(symName)] = meta

				if len(meta.ReturnTypes) > 0 {
					retType := fnVal.Type().Out(0)
					if retType.Kind() == reflect.Ptr {
						retType = retType.Elem()
					}
					if retType.Kind() == reflect.Struct {
						typeName := retType.Name()
						typeSym := rp.extractTypeSymbol(typeName, retType)
						pkgSymbols.Types[typeName] = typeSym
						pkgSymbols.Types[strings.ToLower(typeName)] = typeSym
						pkgSymbols.Types[snakeToPascal(typeName)] = typeSym
						pkgSymbols.Types[pascalToSnake(typeName)] = typeSym
					}
				}
			}
		}
		return pkgSymbols, nil
	}

	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("invalid package structure for '%s', expected a struct", pkgPath)
	}

	for _, symName := range symbolsNeeded {
		var fieldVal reflect.Value
		pascalName := snakeToPascal(symName)

		// 1. Tentar encontrar campo por PascalCase
		fieldVal = val.FieldByName(pascalName)
		actualName := pascalName

		// 2. Tentar encontrar método por PascalCase no valor original
		if !fieldVal.IsValid() {
			fieldVal = origVal.MethodByName(pascalName)
			actualName = pascalName
		}

		// 3. Tentar encontrar campo pelo nome original
		if !fieldVal.IsValid() {
			fieldVal = val.FieldByName(symName)
			actualName = symName
		}

		// 4. Tentar encontrar método pelo nome original
		if !fieldVal.IsValid() {
			fieldVal = origVal.MethodByName(symName)
			actualName = symName
		}

		// 5. Suporte a Construtor de Struct (ex: se pediu "server", buscar "NewServer")
		if !fieldVal.IsValid() {
			constructorName := "New" + pascalName
			fieldVal = val.FieldByName(constructorName)
			if !fieldVal.IsValid() {
				fieldVal = origVal.MethodByName(constructorName)
			}
			actualName = constructorName
		}

		if !fieldVal.IsValid() {
			return nil, fmt.Errorf("symbol '%s' not found in package '%s'", symName, pkgPath)
		}

		// Extrair a função se for um valor de função ou um campo que armazena uma função
		var fnVal reflect.Value
		if fieldVal.Kind() == reflect.Func {
			fnVal = fieldVal
		} else if fieldVal.Kind() == reflect.Interface && !fieldVal.IsNil() && fieldVal.Elem().Kind() == reflect.Func {
			fnVal = fieldVal.Elem()
		}

		if fnVal.IsValid() {
			meta := rp.analyzeGoFunction(actualName, fnVal)
			pkgSymbols.Funcs[actualName] = meta
			pkgSymbols.Funcs[symName] = meta
			if actualName != pascalName {
				pkgSymbols.Funcs[pascalName] = meta
			}

			// Extrair tipo de retorno se for Struct ou Pointer para Struct (ex: para inicializadores)
			if len(meta.ReturnTypes) > 0 {
				retType := fnVal.Type().Out(0)
				if retType.Kind() == reflect.Ptr {
					retType = retType.Elem()
				}
				if retType.Kind() == reflect.Struct {
					typeName := retType.Name()
					typeSym := rp.extractTypeSymbol(typeName, retType)
					pkgSymbols.Types[typeName] = typeSym
					pkgSymbols.Types[strings.ToLower(typeName)] = typeSym
					pkgSymbols.Types[snakeToPascal(typeName)] = typeSym
					pkgSymbols.Types[pascalToSnake(typeName)] = typeSym
				}
			}
		}
	}

	return pkgSymbols, nil
}

func (rp *GoReflectionParser) extractTypeSymbol(name string, t reflect.Type) *TypeSymbol {
	sym := &TypeSymbol{
		Name:   name,
		Fields: make(map[string]string),
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// Ignorar campos privados
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		typeStr := field.Type.String()
		sym.Fields[field.Name] = typeStr
		sym.Fields[strings.ToLower(field.Name)] = typeStr
		sym.Fields[pascalToSnake(field.Name)] = typeStr
	}

	return sym
}

func (rp *GoReflectionParser) analyzeGoFunction(name string, val reflect.Value) *FuncSymbol {
	typ := val.Type()
	paramNames := make([]string, typ.NumIn())
	paramTypes := make([]string, typ.NumIn())
	for i := 0; i < typ.NumIn(); i++ {
		paramNames[i] = fmt.Sprintf("arg%d", i)
		paramTypes[i] = typ.In(i).String()
	}

	returnTypes := make([]string, typ.NumOut())
	for i := 0; i < typ.NumOut(); i++ {
		returnTypes[i] = typ.Out(i).String()
	}

	isSlice := false
	isScalar := false
	isGeneric := false
	if len(paramTypes) > 0 {
		first := typ.In(0)
		if first.Kind() == reflect.Slice {
			isSlice = true
		} else if first.Kind() == reflect.Interface {
			isGeneric = true
		} else {
			isScalar = true
		}
	}

	hasError := false
	if len(returnTypes) > 0 {
		lastRet := typ.Out(typ.NumOut() - 1)
		errorInterface := reflect.TypeOf((*error)(nil)).Elem()
		if lastRet.Implements(errorInterface) {
			hasError = true
		}
	}

	return &FuncSymbol{
		Name:           name,
		ParamNames:     paramNames,
		ParamTypes:     paramTypes,
		ReturnTypes:    returnTypes,
		IsSliceFn:      isSlice,
		IsScalarFn:     isScalar,
		IsGenericFn:    isGeneric,
		HasErrorReturn: hasError,
	}
}
