package vm

import (
	"context"
	"reflect"

	"heddle/pkg/lang/ir"
	"heddle/pkg/runtime"
)

type mapper struct {
	vm *VM
}

func (m *mapper) ApplyMapper(ctx context.Context, containerVal any, path []string, alias string, insts []ir.Instruction, scope *runtime.Scope) (any, error) {
	if len(path) == 0 {
		return containerVal, nil
	}

	vVal := reflect.ValueOf(containerVal)
	for vVal.Kind() == reflect.Ptr || vVal.Kind() == reflect.Interface {
		if vVal.IsNil() {
			return containerVal, nil
		}
		vVal = vVal.Elem()
	}

	// Lidar com slices/arrays nativos
	if vVal.Kind() == reflect.Slice {
		newSlice := reflect.MakeSlice(vVal.Type(), vVal.Len(), vVal.Len())
		for idx := 0; idx < vVal.Len(); idx++ {
			elem := vVal.Index(idx).Interface()
			transformed, err := m.ApplyMapper(ctx, elem, path, alias, insts, scope)
			if err != nil {
				return nil, err
			}
			newSlice.Index(idx).Set(reflect.ValueOf(transformed))
		}
		return newSlice.Interface(), nil
	}

	// Se for Frame, aplicamos a transformação recursivamente nas linhas
	if f, ok := containerVal.(*runtime.Frame); ok {
		var newRows []any
		for _, row := range f.Rows {
			transformed, err := m.ApplyMapper(ctx, row, path, alias, insts, scope)
			if err != nil {
				return nil, err
			}
			newRows = append(newRows, transformed)
		}
		return runtime.NewFrame(newRows), nil
	}

	// Navegar pelo caminho do mapeador
	elemName := path[0]
	if len(path) == 1 {
		// Chegou ao campo alvo. Aplicamos as instruções do corpo sobre ele.
		fieldVal, err := m.vm.executor.resolvePropertyPath(containerVal, []string{elemName})
		if err != nil {
			return containerVal, nil
		}

		// Executa instruções do corpo com o alias definido
		localScope := runtime.NewScope(scope)
		localScope.Set(alias, runtime.NewScalarFrame(fieldVal))
		resFrame, err := m.vm.executor.ExecuteInstructions(ctx, insts, localScope)
		if err != nil {
			return nil, err
		}

		newVal := any(nil)
		if resFrame != nil && len(resFrame.Rows) > 0 {
			newVal = resFrame.Rows[0]
		}

		// Retorna o objeto container com o campo atualizado
		return m.UpdateField(containerVal, elemName, newVal)
	}

	// Caminho aninhado. Recupera o sub-objeto e aplica recursivamente.
	fieldVal, err := m.vm.executor.resolvePropertyPath(containerVal, []string{elemName})
	if err != nil {
		return containerVal, nil
	}

	transformedSub, err := m.ApplyMapper(ctx, fieldVal, path[1:], alias, insts, scope)
	if err != nil {
		return nil, err
	}

	return m.UpdateField(containerVal, elemName, transformedSub)
}

func (m *mapper) UpdateField(container any, fieldName string, newVal any) (any, error) {
	vVal := reflect.ValueOf(container)
	isPtr := vVal.Kind() == reflect.Ptr
	baseVal := vVal
	if isPtr {
		baseVal = vVal.Elem()
	}

	if baseVal.Kind() == reflect.Map {
		// Criar uma cópia do mapa para manter imutabilidade relativa
		newMap := reflect.MakeMap(baseVal.Type())
		for _, k := range baseVal.MapKeys() {
			newMap.SetMapIndex(k, baseVal.MapIndex(k))
		}
		newMap.SetMapIndex(reflect.ValueOf(fieldName), reflect.ValueOf(newVal))
		if isPtr {
			ptr := reflect.New(newMap.Type())
			ptr.Elem().Set(newMap)
			return ptr.Interface(), nil
		}
		return newMap.Interface(), nil
	}

	if baseVal.Kind() == reflect.Struct {
		// Criar uma cópia da struct
		newStruct := reflect.New(baseVal.Type()).Elem()
		newStruct.Set(baseVal)

		field := newStruct.FieldByName(runtime.SnakeToPascal(fieldName))
		if !field.IsValid() {
			field = newStruct.FieldByName(fieldName)
		}

		if field.IsValid() && field.CanSet() {
			convVal, err := runtime.ConvertValue(newVal, field.Type())
			if err == nil {
				field.Set(convVal)
			}
		}
		if isPtr {
			return newStruct.Addr().Interface(), nil
		}
		return newStruct.Interface(), nil
	}

	return container, nil
}
