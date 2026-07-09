package runtime

import (
	"fmt"
	"reflect"
	"strings"
)

// DataContainer define a abstração para armazenamento e travessia de dados do Heddle
type DataContainer interface {
	GetProperty(path []string) (DataContainer, error)
	UpdateProperty(path []string, newVal any) (DataContainer, error)
	ToGoValue(targetType reflect.Type) (reflect.Value, error)
	RawRows() []any
}

// NestedListContainer implementa o armazenamento uniforme baseado em listas fisicamente aninhadas (Abordagem 2)
type NestedListContainer struct {
	rows []any
}

// NewNestedListContainer constrói um NestedListContainer garantindo a representação de lista uniforme
func NewNestedListContainer(rows []any) *NestedListContainer {
	formatted := make([]any, len(rows))
	for i, r := range rows {
		formatted[i] = toNestedListRow(r)
	}
	return &NestedListContainer{rows: formatted}
}

func (nlc *NestedListContainer) RawRows() []any {
	return nlc.rows
}

func (nlc *NestedListContainer) GetProperty(path []string) (DataContainer, error) {
	if len(path) == 0 {
		return nlc, nil
	}

	res, err := resolvePathStep(nlc.rows, path)
	if err != nil {
		return nil, err
	}

	var resRows []any
	if s, ok := res.([]any); ok {
		resRows = s
	} else if res != nil {
		resRows = []any{res}
	} else {
		resRows = []any{}
	}

	return &NestedListContainer{rows: resRows}, nil
}

func (nlc *NestedListContainer) UpdateProperty(path []string, newVal any) (DataContainer, error) {
	updated, err := updatePathStep(nlc.rows, path, newVal)
	if err != nil {
		return nil, err
	}

	var resRows []any
	if s, ok := updated.([]any); ok {
		resRows = s
	} else if updated != nil {
		resRows = []any{updated}
	} else {
		resRows = []any{}
	}

	return &NestedListContainer{rows: resRows}, nil
}

func (nlc *NestedListContainer) ToGoValue(targetType reflect.Type) (reflect.Value, error) {
	if targetType.Kind() == reflect.Slice {
		elemType := targetType.Elem()
		sliceVal := reflect.MakeSlice(targetType, len(nlc.rows), len(nlc.rows))
		for i, row := range nlc.rows {
			converted, err := ConvertValue(row, elemType)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("failed converting element %d: %w", i, err)
			}
			sliceVal.Index(i).Set(converted)
		}
		return sliceVal, nil
	}

	// Tratar o próprio tipo *Frame
	if targetType == reflect.TypeOf((*Frame)(nil)) {
		return reflect.ValueOf(&Frame{data: nlc}), nil
	}

	// Tratar interface vazia (any)
	if targetType.Kind() == reflect.Interface && targetType.NumMethod() == 0 {
		return reflect.ValueOf(nlc.rows), nil
	}

	if len(nlc.rows) == 0 {
		return reflect.Zero(targetType), nil
	}

	return ConvertValue(nlc.rows[0], targetType)
}

// Funções Auxiliares de Normalização para Listas Uniformes

func toNestedList(val any) []any {
	if val == nil {
		return []any{}
	}

	if f, ok := val.(*Frame); ok {
		return f.RawRows()
	}

	if dc, ok := val.(DataContainer); ok {
		return dc.RawRows()
	}

	valVal := reflect.ValueOf(val)
	if valVal.Kind() == reflect.Slice || valVal.Kind() == reflect.Array {
		res := make([]any, valVal.Len())
		for i := 0; i < valVal.Len(); i++ {
			res[i] = toNestedListRow(valVal.Index(i).Interface())
		}
		return res
	}

	return []any{toNestedListRow(val)}
}

func toNestedListRow(val any) any {
	if val == nil {
		return nil
	}

	if f, ok := val.(*Frame); ok {
		return f.RawRows()
	}
	if dc, ok := val.(DataContainer); ok {
		return dc.RawRows()
	}

	valVal := reflect.ValueOf(val)
	for valVal.Kind() == reflect.Ptr || valVal.Kind() == reflect.Interface {
		if valVal.IsNil() {
			return nil
		}
		valVal = valVal.Elem()
	}

	typeStr := valVal.Type().String()
	if valVal.NumMethod() > 0 ||
		strings.Contains(typeStr, "Seq") ||
		strings.Contains(typeStr, "ReqReply") ||
		strings.Contains(typeStr, "SeqAccum") ||
		valVal.Kind() == reflect.Func ||
		valVal.Kind() == reflect.Chan {
		return valVal.Interface()
	}

	if valVal.Kind() == reflect.Slice || valVal.Kind() == reflect.Array {
		return toNestedList(valVal.Interface())
	}

	if valVal.Kind() == reflect.Map {
		m := make(map[string]any)
		for _, k := range valVal.MapKeys() {
			kStr := k.String()
			m[toSnakeCase(kStr)] = toNestedList(valVal.MapIndex(k).Interface())
		}
		return m
	}

	if valVal.Kind() == reflect.Struct {
		m := make(map[string]any)
		t := valVal.Type()
		for i := 0; i < valVal.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" && !f.Anonymous {
				continue // Ignorar campos privados
			}
			m[toSnakeCase(f.Name)] = toNestedList(valVal.Field(i).Interface())
		}
		return m
	}

	return valVal.Interface()
}

// Funções Auxiliares de Travessia e Resolução de Caminhos com Achatamento (Flattening)

func resolvePathStep(val any, path []string) (any, error) {
	if len(path) == 0 {
		return val, nil
	}

	if val == nil {
		return nil, nil
	}

	valVal := reflect.ValueOf(val)
	for valVal.Kind() == reflect.Ptr || valVal.Kind() == reflect.Interface {
		if valVal.IsNil() {
			return nil, nil
		}
		valVal = valVal.Elem()
	}

	if valVal.Kind() == reflect.Slice || valVal.Kind() == reflect.Array {
		var results []any
		for idx := 0; idx < valVal.Len(); idx++ {
			elem := valVal.Index(idx).Interface()
			res, err := resolvePathStep(elem, path)
			if err != nil {
				return nil, err
			}
			if res != nil {
				resVal := reflect.ValueOf(res)
				if resVal.Kind() == reflect.Slice || resVal.Kind() == reflect.Array {
					for j := 0; j < resVal.Len(); j++ {
						results = append(results, resVal.Index(j).Interface())
					}
				} else {
					results = append(results, res)
				}
			}
		}
		return results, nil
	}

	elem := path[0]
	var nextVal any

	if valVal.Kind() == reflect.Map {
		found := false
		for _, k := range valVal.MapKeys() {
			if strings.EqualFold(k.String(), elem) {
				nextVal = valVal.MapIndex(k).Interface()
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("key '%s' not found in map", elem)
		}
	} else if valVal.Kind() == reflect.Struct {
		field := valVal.FieldByName(SnakeToPascal(elem))
		if !field.IsValid() {
			field = valVal.FieldByName(elem)
		}
		if !field.IsValid() {
			typ := valVal.Type()
			for idx := 0; idx < typ.NumField(); idx++ {
				f := typ.Field(idx)
				if strings.EqualFold(f.Name, elem) {
					field = valVal.Field(idx)
					break
				}
			}
		}
		if !field.IsValid() {
			return nil, fmt.Errorf("field '%s' not found in struct", elem)
		}
		nextVal = field.Interface()
	} else {
		return nil, fmt.Errorf("cannot access property '%s' on primitive %T", elem, val)
	}

	return resolvePathStep(nextVal, path[1:])
}

func updatePathStep(val any, path []string, newVal any) (any, error) {
	if len(path) == 0 {
		return newVal, nil
	}

	if val == nil {
		return nil, fmt.Errorf("cannot update property on nil value")
	}

	valVal := reflect.ValueOf(val)
	for valVal.Kind() == reflect.Ptr || valVal.Kind() == reflect.Interface {
		if valVal.IsNil() {
			return nil, fmt.Errorf("cannot update property on nil value")
		}
		valVal = valVal.Elem()
	}

	if valVal.Kind() == reflect.Slice || valVal.Kind() == reflect.Array {
		resSlice := make([]any, valVal.Len())
		for idx := 0; idx < valVal.Len(); idx++ {
			elem := valVal.Index(idx).Interface()
			updated, err := updatePathStep(elem, path, newVal)
			if err != nil {
				return nil, err
			}
			resSlice[idx] = updated
		}
		return resSlice, nil
	}

	if valVal.Kind() == reflect.Map {
		m := make(map[string]any)
		for _, k := range valVal.MapKeys() {
			m[k.String()] = valVal.MapIndex(k).Interface()
		}

		elem := path[0]
		var subVal any
		found := false
		var matchedKey string
		for k := range m {
			if strings.EqualFold(k, elem) {
				subVal = m[k]
				matchedKey = k
				found = true
				break
			}
		}

		if len(path) == 1 {
			nestedNewVal := toNestedList(newVal)
			if found {
				m[matchedKey] = nestedNewVal
			} else {
				m[toSnakeCase(elem)] = nestedNewVal
			}
			return m, nil
		}

		if !found {
			subVal = make(map[string]any)
			matchedKey = toSnakeCase(elem)
		}

		updatedSub, err := updatePathStep(subVal, path[1:], newVal)
		if err != nil {
			return nil, err
		}
		m[matchedKey] = updatedSub
		return m, nil
	}

	if valVal.Kind() == reflect.Struct {
		m := make(map[string]any)
		t := valVal.Type()
		for i := 0; i < valVal.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" && !f.Anonymous {
				continue
			}
			m[toSnakeCase(f.Name)] = valVal.Field(i).Interface()
		}

		elem := path[0]
		var subVal any
		found := false
		var matchedKey string
		for k := range m {
			if strings.EqualFold(k, elem) {
				subVal = m[k]
				matchedKey = k
				found = true
				break
			}
		}

		if len(path) == 1 {
			nestedNewVal := toNestedList(newVal)
			if found {
				m[matchedKey] = nestedNewVal
			} else {
				m[toSnakeCase(elem)] = nestedNewVal
			}
			return m, nil
		}

		if !found {
			subVal = make(map[string]any)
			matchedKey = toSnakeCase(elem)
		}

		updatedSub, err := updatePathStep(subVal, path[1:], newVal)
		if err != nil {
			return nil, err
		}
		m[matchedKey] = updatedSub
		return m, nil
	}

	return nil, fmt.Errorf("cannot update property '%s' on primitive %T", path[0], val)
}
