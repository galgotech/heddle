package vm

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"heddle/pkg/lang/ir"
	"heddle/pkg/runtime"
	"heddle/pkg/runtime/bridge/reflection"
)

type instructionExecutor struct {
	vm *VM
}

func (s *instructionExecutor) ExecuteInstructions(ctx context.Context, insts []ir.Instruction, scope *runtime.Scope) (*runtime.Frame, error) {
	var stack []any

	pop := func() any {
		if len(stack) == 0 {
			return nil
		}
		val := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return val
	}

	for _, inst := range insts {
		switch i := inst.(type) {
		case ir.LoadConst:
			stack = append(stack, i.Value)

		case ir.LoadVar:
			val, exists := scope.Get(i.Name)
			if !exists {
				return nil, fmt.Errorf("runtime error: undefined variable '%s'", i.Name)
			}
			stack = append(stack, val)

		case ir.StoreVar:
			val := pop()
			scope.Set(i.Name, val)

		case ir.PropAccess:
			val := pop()
			res, err := s.resolvePropertyPath(val, i.Path)
			if err != nil {
				return nil, err
			}
			stack = append(stack, res)

		case ir.BuildStruct:
			info := i.Info
			fields := make(map[string]any)
			for idx := len(info.FieldNames) - 1; idx >= 0; idx-- {
				name := info.FieldNames[idx]
				fields[name] = pop()
			}

			constructorFound := false
			parts := strings.Split(info.TypeName, ".")
			if len(parts) == 2 {
				pkgAlias := parts[0]
				typeName := parts[1]
				constructorName := pkgAlias + "." + "New" + runtime.SnakeToPascal(typeName)

				if b, ok := s.vm.bridge.(*reflection.ReflectionBridge); ok {
					fnVal, ok := b.ResolveMember(constructorName)
					if !ok {
						constructorName = pkgAlias + "." + typeName
						fnVal, ok = b.ResolveMember(constructorName)
					}
					if ok {
						fnType := fnVal.Type()
						if fnType.NumIn() == 1 {
							optsType := fnType.In(0)
							optsVal, err := s.vm.reflector.ConvertMapToStruct(fields, optsType)
							if err == nil {
								outVals := fnVal.Call([]reflect.Value{optsVal})
								if len(outVals) > 0 {
									serverVal := outVals[0].Interface()
									stack = append(stack, serverVal)
									constructorFound = true
								}
							}
						}
					}
				}
			}

			if !constructorFound {
				stack = append(stack, fields)
			}

		case ir.BuildMap:
			pairCount := i.Size
			m := make(map[string]any)
			// Desempilhar em ordem reversa
			tempKeys := make([]string, pairCount)
			tempVals := make([]any, pairCount)
			for j := pairCount - 1; j >= 0; j-- {
				tempVals[j] = pop()
				tempKeys[j] = pop().(string)
			}
			for j := 0; j < pairCount; j++ {
				m[tempKeys[j]] = tempVals[j]
			}
			stack = append(stack, m)

		case ir.BuildArray:
			elemCount := i.Size
			arr := make([]any, elemCount)
			for j := elemCount - 1; j >= 0; j-- {
				arr[j] = pop()
			}
			stack = append(stack, arr)

		case ir.Call:
			info := i.Info
			// Desempilhar argumentos
			args := make([]any, info.NumArgs)
			for j := info.NumArgs - 1; j >= 0; j-- {
				args[j] = pop()
			}

			var receiver *runtime.Frame = runtime.NewFrame([]any{})
			if info.HasReceiver {
				receiverVal := pop()
				if f, ok := receiverVal.(*runtime.Frame); ok {
					receiver = f
				} else {
					receiver = runtime.NewScalarFrame(receiverVal)
				}
			}

			// Mapeamento de argumentos nomeados e posicionais
			namedArgs := make(map[string]any)
			var positionalArgs []any

			for idx, name := range info.ArgNames {
				if name != "" {
					namedArgs[name] = args[idx]
				} else {
					positionalArgs = append(positionalArgs, args[idx])
				}
			}
			if len(info.ArgNames) == 0 {
				positionalArgs = args
			}

			// Execução: Verificar se chama fluxo local, método em objeto local ou função Go externa
			var result *runtime.Frame
			var err error

			parts := strings.Split(info.FuncName, ".")
			isMethodCall := false
			var targetObj any

			if len(parts) == 2 {
				if val, exists := scope.Get(parts[0]); exists {
					if f, ok := val.(*runtime.Frame); ok && len(f.Rows) > 0 {
						targetObj = f.Rows[0]
						isMethodCall = true
					} else {
						targetObj = val
						isMethodCall = true
					}
				}
			}

			if _, exists := s.vm.flows[info.FuncName]; exists {
				// Chamada de fluxo local
				result, err = s.vm.ExecuteFlow(ctx, info.FuncName, receiver)
			} else if isMethodCall {
				// Chamada de método de objeto Go local
				result, err = s.vm.reflector.CallMethod(ctx, targetObj, parts[1], receiver, namedArgs, positionalArgs)
			} else {
				// Chamada delegada ao Bridge
				result, err = s.vm.bridge.Call(ctx, info.FuncName, receiver, namedArgs, positionalArgs)
			}

			if err != nil {
				return nil, err
			}
			stack = append(stack, result)

		case ir.Match:
			info := i.Info
			targetVal := pop()

			res, isReturn, err := s.vm.matcher.Match(ctx, targetVal, info.Cases, scope)
			if err != nil {
				return nil, err
			}
			if isReturn {
				return res, nil
			}
			if res != nil {
				stack = append(stack, res)
			}

		case ir.Spawn:
			info := i.Info
			var wg sync.WaitGroup
			results := make([]*runtime.Frame, len(info.Branches))
			errChan := make(chan error, len(info.Branches))

			for idx, branch := range info.Branches {
				wg.Add(1)
				go func(bIdx int, insts []ir.Instruction) {
					defer wg.Done()
					res, err := s.ExecuteInstructions(ctx, insts, scope)
					if err != nil {
						errChan <- err
						return
					}
					results[bIdx] = res
				}(idx, branch)
			}
			wg.Wait()

			select {
			case err := <-errChan:
				return nil, err
			default:
			}

			// Join: consolidar fatias de resultados concorrentes em um único Frame contendo tupla de linhas ou merge de objetos
			consolidatedRows := s.vm.merger.MergeResults(results)
			stack = append(stack, runtime.NewFrame(consolidatedRows))

		case ir.Mapper:
			info := i.Info
			containerVal := pop()

			// Aplicar mapeadores sobre containers mutando propriedades em lote
			updated, err := s.vm.mapper.ApplyMapper(ctx, containerVal, info.Path, info.Alias, info.Instructions, scope)
			if err != nil {
				return nil, err
			}
			stack = append(stack, updated)

		case ir.FunctionHandler:
			info := i.Info

			// Executa as instruções internas
			res, err := s.ExecuteInstructions(ctx, info.Body, scope)
			if err != nil {
				// Erro interceptado: invocar o handler associado (Function-level handler)
				var handlerRes *runtime.Frame
				handlerRes, err = s.vm.ExecuteHandler(ctx, info.HandlerName, err)
				if err != nil {
					return nil, err // Falhou dentro do handler ou handler disparou erro
				}
				stack = append(stack, handlerRes)
			} else if res != nil {
				stack = append(stack, res)
			}

		case ir.Return:
			val := pop()
			var retFrame *runtime.Frame
			if f, ok := val.(*runtime.Frame); ok {
				retFrame = f
			} else {
				retFrame = runtime.NewScalarFrame(val)
			}
			return retFrame, nil
		}
	}

	if len(stack) > 0 {
		top := stack[len(stack)-1]
		if f, ok := top.(*runtime.Frame); ok {
			return f, nil
		}
		return runtime.NewScalarFrame(top), nil
	}

	return nil, nil
}

func (s *instructionExecutor) resolvePropertyPath(val any, path []string) (any, error) {
	if val == nil {
		return nil, nil
	}

	var dc runtime.DataContainer
	if f, ok := val.(*runtime.Frame); ok {
		dc = f.Data()
	} else if container, ok := val.(runtime.DataContainer); ok {
		dc = container
	} else {
		dc = runtime.NewNestedListContainer([]any{val})
	}

	resContainer, err := dc.GetProperty(path)
	if err != nil {
		return nil, err
	}

	return runtime.NewFrameFromData(resContainer), nil
}
