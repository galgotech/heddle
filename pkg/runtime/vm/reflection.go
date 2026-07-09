package vm

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"heddle/pkg/runtime"
	"heddle/pkg/runtime/bridge/reflection"
)

type reflectionInvoker struct {
	vm *VM
}

func (s *reflectionInvoker) CallMethod(ctx context.Context, obj any, methodName string, input *runtime.Frame, namedArgs map[string]any, positionalArgs []any) (*runtime.Frame, error) {
	val := reflect.ValueOf(obj)
	pascalName := runtime.SnakeToPascal(methodName)
	methodVal := val.MethodByName(pascalName)
	if !methodVal.IsValid() {
		methodVal = val.MethodByName(methodName)
	}

	if !methodVal.IsValid() {
		return nil, fmt.Errorf("method '%s' not found on object %T", methodName, obj)
	}

	// Reutilizar o ReflectionBridge populado com o objeto como pacote virtual "obj"
	tempPackages := map[string]any{
		"obj": obj,
	}
	tempMetadata := map[string]runtime.FuncMetadata{
		"obj." + methodName: s.getMetadataForMethod(obj, methodName),
	}

	tempDispatcher := reflection.NewReflectionBridge(tempPackages, tempMetadata)
	return tempDispatcher.Call(ctx, "obj."+methodName, input, namedArgs, positionalArgs)
}

func (s *reflectionInvoker) getMetadataForMethod(obj any, methodName string) runtime.FuncMetadata {
	t := reflect.TypeOf(obj)
	if t == nil {
		return runtime.FuncMetadata{}
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Name() == "HTTPServer" && strings.HasSuffix(t.PkgPath(), "pkg/lib/net/http") && (strings.EqualFold(methodName, "get") || strings.EqualFold(methodName, "post")) {
		return runtime.FuncMetadata{ParamNames: []string{"path"}}
	}
	return runtime.FuncMetadata{}
}

func (s *reflectionInvoker) ConvertMapToStruct(m map[string]any, destType reflect.Type) (reflect.Value, error) {
	isPtr := destType.Kind() == reflect.Ptr
	actualType := destType
	if isPtr {
		actualType = destType.Elem()
	}

	structVal := reflect.New(actualType).Elem()

	for idx := 0; idx < actualType.NumField(); idx++ {
		field := actualType.Field(idx)
		fieldName := field.Name

		var mapVal any
		found := false
		for k, v := range m {
			if strings.EqualFold(k, fieldName) ||
				runtime.SnakeToPascal(k) == runtime.SnakeToPascal(fieldName) ||
				field.Tag.Get("json") == k {
				mapVal = v
				found = true
				break
			}
		}

		if found {
			converted, err := runtime.ConvertValue(mapVal, field.Type)
			if err == nil {
				structVal.Field(idx).Set(converted)
			}
		}
	}

	if isPtr {
		return structVal.Addr(), nil
	}
	return structVal, nil
}
