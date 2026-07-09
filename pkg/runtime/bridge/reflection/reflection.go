package reflection

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"heddle/pkg/runtime"
)

// ReflectionBridge executa chamadas locais a funções do Go usando o pacote reflect
type ReflectionBridge struct {
	Packages map[string]any
	Metadata map[string]runtime.FuncMetadata
}

func NewReflectionBridge(pkgs map[string]any, metadata map[string]runtime.FuncMetadata) *ReflectionBridge {
	return &ReflectionBridge{
		Packages: pkgs,
		Metadata: metadata,
	}
}

func (rb *ReflectionBridge) Call(ctx context.Context, fnName string, input *runtime.Frame, namedArgs map[string]any, positionalArgs []any) (*runtime.Frame, error) {
	// 1. Resolver o membro (função ou construtor) no pacote Go
	fnVal, ok := rb.ResolveMember(fnName)
	if !ok {
		return nil, fmt.Errorf("function '%s' could not be resolved in the registered Go packages", fnName)
	}

	fnType := fnVal.Type()
	numParams := fnType.NumIn()

	// 2. Determinar o modelo de execução com base no primeiro parâmetro da assinatura
	isSliceFn := false
	isGenericFn := false
	if numParams > 0 {
		firstParam := fnType.In(0)
		if firstParam.Kind() == reflect.Slice {
			isSliceFn = true
		} else if firstParam.Kind() == reflect.Interface {
			isGenericFn = true
		}
	}

	// 3. Obter metadados de nomes de parâmetros para mapeamento por nome
	var paramNames []string
	if meta, exists := rb.Metadata[fnName]; exists {
		paramNames = meta.ParamNames
	} else {
		// Fallback: Gerar nomes genéricos arg0, arg1...
		paramNames = make([]string, numParams)
		for i := 0; i < numParams; i++ {
			paramNames[i] = fmt.Sprintf("arg%d", i)
		}
	}

	// 4. Executar de acordo com a assinatura
	if isSliceFn {
		// Função de Slice: recebe o input inteiro convertido para fatia Go
		argsVal, err := rb.buildArgsList(fnVal, paramNames, input.Rows, namedArgs, positionalArgs, true)
		if err != nil {
			return nil, err
		}
		return rb.invokeFunc(fnVal, argsVal)
	}

	if isGenericFn {
		// Função Genérica: repassa o Frame/linha diretamente
		argsVal, err := rb.buildArgsList(fnVal, paramNames, input.Rows, namedArgs, positionalArgs, false)
		if err != nil {
			return nil, err
		}
		return rb.invokeFunc(fnVal, argsVal)
	}

	// Função Escalar: itera individualmente sobre cada elemento da entrada
	if len(input.Rows) == 0 {
		// Se a entrada for vazia, executa uma vez com nil/zero
		argsVal, err := rb.buildArgsSingle(fnVal, paramNames, nil, namedArgs, positionalArgs)
		if err != nil {
			return nil, err
		}
		return rb.invokeFunc(fnVal, argsVal)
	}

	var results []any
	for _, row := range input.Rows {
		argsVal, err := rb.buildArgsSingle(fnVal, paramNames, row, namedArgs, positionalArgs)
		if err != nil {
			return nil, err
		}
		resFrame, err := rb.invokeFunc(fnVal, argsVal)
		if err != nil {
			return nil, err
		}
		results = append(results, resFrame.Rows...)
	}

	return runtime.NewFrame(results), nil
}

func (rb *ReflectionBridge) ResolveMember(fnName string) (reflect.Value, bool) {
	parts := strings.Split(fnName, ".")
	if len(parts) != 2 {
		return reflect.Value{}, false
	}
	pkgAlias := parts[0]
	memberName := parts[1]

	pkgObj, exists := rb.Packages[pkgAlias]
	if !exists {
		return reflect.Value{}, false
	}

	origVal := reflect.ValueOf(pkgObj)
	val := reflect.ValueOf(pkgObj)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	pascalName := runtime.SnakeToPascal(memberName)

	var memberVal reflect.Value

	// Se o pacote for um map
	if val.Kind() == reflect.Map {
		memberVal = val.MapIndex(reflect.ValueOf(memberName))
		if !memberVal.IsValid() {
			memberVal = val.MapIndex(reflect.ValueOf(pascalName))
		}
		if !memberVal.IsValid() {
			memberVal = val.MapIndex(reflect.ValueOf(runtime.SnakeToPascal(memberName)))
		}
		if !memberVal.IsValid() {
			// Suporte a construtores implícitos New<Type>
			memberVal = val.MapIndex(reflect.ValueOf("New" + pascalName))
		}
		if !memberVal.IsValid() {
			// Suporte a construtores minúsculos no map
			memberVal = val.MapIndex(reflect.ValueOf("new" + memberName))
		}
		if memberVal.IsValid() {
			if memberVal.Kind() == reflect.Func {
				return memberVal, true
			}
			if memberVal.Kind() == reflect.Interface && !memberVal.IsNil() && memberVal.Elem().Kind() == reflect.Func {
				return memberVal.Elem(), true
			}
		}
		return reflect.Value{}, false
	}

	// 1. Tentar encontrar campo por PascalCase
	if val.Kind() == reflect.Struct {
		memberVal = val.FieldByName(pascalName)
	}

	// 2. Tentar encontrar método por PascalCase no ponteiro/valor original
	if !memberVal.IsValid() {
		memberVal = origVal.MethodByName(pascalName)
	}

	// 3. Tentar encontrar campo pelo nome original
	if !memberVal.IsValid() && val.Kind() == reflect.Struct {
		memberVal = val.FieldByName(memberName)
	}

	// 4. Tentar encontrar método pelo nome original
	if !memberVal.IsValid() {
		memberVal = origVal.MethodByName(memberName)
	}

	// 5. Tentar encontrar construtor New<Type>
	if !memberVal.IsValid() && val.Kind() == reflect.Struct {
		memberVal = val.FieldByName("New" + pascalName)
	}
	if !memberVal.IsValid() {
		memberVal = origVal.MethodByName("New" + pascalName)
	}

	if memberVal.IsValid() {
		if memberVal.Kind() == reflect.Func {
			return memberVal, true
		}
		// Desembrulhar interfaces que envelopam funções
		if memberVal.Kind() == reflect.Interface && !memberVal.IsNil() && memberVal.Elem().Kind() == reflect.Func {
			return memberVal.Elem(), true
		}
	}

	return reflect.Value{}, false
}

func (rb *ReflectionBridge) buildArgsList(fnVal reflect.Value, paramNames []string, rows []any, namedArgs map[string]any, positionalArgs []any, asSlice bool) ([]reflect.Value, error) {
	fnType := fnVal.Type()
	numParams := fnType.NumIn()
	args := make([]reflect.Value, numParams)

	posIdx := 0
	for i := 0; i < numParams; i++ {
		paramType := fnType.In(i)
		paramName := paramNames[i]

		// 1. Verificar se o argumento foi passado por nome
		if val, exists := namedArgs[paramName]; exists {
			converted, err := runtime.ConvertValue(val, paramType)
			if err != nil {
				return nil, fmt.Errorf("invalid named arg '%s': %w", paramName, err)
			}
			args[i] = converted
			continue
		}

		// 2. Tentar associar do pipe como receiver principal (somente no primeiro parâmetro)
		if i == 0 {
			if asSlice {
				// Converter a lista de linhas para um slice do tipo do parâmetro
				convertedSlice, err := runtime.NewFrame(rows).ToSlice(paramType)
				if err != nil {
					return nil, fmt.Errorf("failed converting pipeline frame to slice parameter: %w", err)
				}
				args[i] = convertedSlice
				continue
			} else {
				// Passar as linhas brutas (geralmente para any/any)
				var valToPass any = rows
				if len(rows) == 1 {
					valToPass = rows[0]
				}
				converted, err := runtime.ConvertValue(valToPass, paramType)
				if err == nil {
					args[i] = converted
					continue
				}
			}
		}

		// 3. Consumir de argumentos posicionais se sobrar
		if posIdx < len(positionalArgs) {
			converted, err := runtime.ConvertValue(positionalArgs[posIdx], paramType)
			if err != nil {
				return nil, fmt.Errorf("invalid positional arg %d: %w", posIdx, err)
			}
			args[i] = converted
			posIdx++
			continue
		}

		// 4. Fallback para valor padrão zero
		args[i] = reflect.Zero(paramType)
	}

	return args, nil
}

func (rb *ReflectionBridge) buildArgsSingle(fnVal reflect.Value, paramNames []string, row any, namedArgs map[string]any, positionalArgs []any) ([]reflect.Value, error) {
	fnType := fnVal.Type()
	numParams := fnType.NumIn()
	args := make([]reflect.Value, numParams)

	posIdx := 0
	for i := 0; i < numParams; i++ {
		paramType := fnType.In(i)
		paramName := paramNames[i]

		// 1. Verificar se foi passado explicitamente por nome
		if val, exists := namedArgs[paramName]; exists {
			converted, err := runtime.ConvertValue(val, paramType)
			if err != nil {
				return nil, fmt.Errorf("invalid named arg '%s': %w", paramName, err)
			}
			args[i] = converted
			continue
		}

		// 2. Consumir de argumentos posicionais se disponíveis
		if posIdx < len(positionalArgs) {
			converted, err := runtime.ConvertValue(positionalArgs[posIdx], paramType)
			if err != nil {
				return nil, fmt.Errorf("invalid positional arg %d: %w", posIdx, err)
			}
			args[i] = converted
			posIdx++
			continue
		}

		// 3. Tentar extrair do elemento/linha atual da entrada (omissão de parâmetros)
		if row != nil {
			if extracted, found := rb.extractFieldValue(reflect.ValueOf(row), paramName); found {
				converted, err := runtime.ConvertValue(extracted.Interface(), paramType)
				if err == nil {
					args[i] = converted
					continue
				}
			}
		}

		// 4. Se for o primeiro parâmetro e a linha atual for compatível
		if i == 0 && row != nil {
			converted, err := runtime.ConvertValue(row, paramType)
			if err == nil {
				args[i] = converted
				continue
			}
		}

		// 5. Fallback para valor padrão
		args[i] = reflect.Zero(paramType)
	}

	return args, nil
}

func (rb *ReflectionBridge) extractFieldValue(val reflect.Value, fieldName string) (reflect.Value, bool) {
	if !val.IsValid() {
		return reflect.Value{}, false
	}
	// Resolver ponteiros e interfaces
	for val.Kind() == reflect.Ptr || val.Kind() == reflect.Interface {
		if val.IsNil() {
			return reflect.Value{}, false
		}
		val = val.Elem()
	}

	if val.Kind() == reflect.Map {
		for _, k := range val.MapKeys() {
			kStr := k.String()
			// Mapeamento tolerante de caixa e compatibilidade de palavras (ex: type -> typ)
			if strings.EqualFold(kStr, fieldName) ||
				runtime.SnakeToPascal(kStr) == runtime.SnakeToPascal(fieldName) ||
				(fieldName == "typ" && kStr == "type") {
				return val.MapIndex(k), true
			}
		}
	} else if val.Kind() == reflect.Struct {
		typ := val.Type()
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.PkgPath != "" && !f.Anonymous {
				continue // Ignorar campo privado
			}
			if strings.EqualFold(f.Name, fieldName) ||
				runtime.SnakeToPascal(f.Name) == runtime.SnakeToPascal(fieldName) ||
				(fieldName == "typ" && f.Name == "Type") {
				return val.Field(i), true
			}
		}
	}

	return reflect.Value{}, false
}

func (rb *ReflectionBridge) invokeFunc(fnVal reflect.Value, args []reflect.Value) (resFrame *runtime.Frame, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("native panic intercepted: %v", r)
		}
	}()

	out := fnVal.Call(args)

	// Verificar se houve erro retornado como último parâmetro
	fnType := fnVal.Type()
	if fnType.NumOut() > 0 {
		lastRet := out[len(out)-1]
		errorInterface := reflect.TypeOf((*error)(nil)).Elem()
		if lastRet.Type().Implements(errorInterface) && !lastRet.IsNil() {
			return nil, lastRet.Interface().(error)
		}
	}

	// Extrair o retorno principal
	if fnType.NumOut() > 0 {
		// Se a função retorna apenas um erro
		if fnType.NumOut() == 1 && fnType.Out(0).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
			return runtime.NewFrame([]any{}), nil
		}
		// Caso contrário, o primeiro valor (índice 0) é o retorno principal
		return runtime.FromGoValue(out[0].Interface()), nil
	}

	return runtime.NewFrame([]any{}), nil
}

// GRPCBridge é um stub de proxy RPC para orquestração distribuída cross-language
type GRPCBridge struct {
	TargetAddr string
}

func NewGRPCBridge(targetAddr string) *GRPCBridge {
	return &GRPCBridge{TargetAddr: targetAddr}
}

func (gb *GRPCBridge) Call(ctx context.Context, fnName string, input *runtime.Frame, namedArgs map[string]any, positionalArgs []any) (*runtime.Frame, error) {
	// Stub de desenvolvimento para cross-language
	return nil, fmt.Errorf("gRPC function calls to language runner at '%s' are not implemented in this development phase (mock placeholder for '%s')", gb.TargetAddr, fnName)
}
