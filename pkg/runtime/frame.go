package runtime

import (
	"fmt"
	"reflect"
	"strings"
)

// Frame representa o envelopamento de dados de entrada/saída que trafegam pelo pipeline Heddle
type Frame struct {
	Rows []any         // Linhas de dados (para compatibilidade de leitura direta)
	data DataContainer // O container de dados abstrato real
}

func NewFrame(rows []any) *Frame {
	dc := NewNestedListContainer(rows)
	return &Frame{
		Rows: dc.RawRows(),
		data: dc,
	}
}

func NewFrameFromData(data DataContainer) *Frame {
	if data == nil {
		dc := NewNestedListContainer([]any{})
		return &Frame{
			Rows: dc.RawRows(),
			data: dc,
		}
	}
	return &Frame{
		Rows: data.RawRows(),
		data: data,
	}
}

// NewScalarFrame cria um frame encapsulando um valor escalar convertido em lista de 1 elemento
func NewScalarFrame(val any) *Frame {
	if val == nil {
		return NewFrame([]any{})
	}
	if f, ok := val.(*Frame); ok {
		return f
	}
	if dc, ok := val.(DataContainer); ok {
		return NewFrameFromData(dc)
	}
	return NewFrame([]any{val})
}

// ToSlice converte as linhas do Frame para uma fatia tipada do Go usando reflexão
func (f *Frame) ToSlice(sliceType reflect.Type) (reflect.Value, error) {
	return f.data.ToGoValue(sliceType)
}

func (f *Frame) RawRows() []any {
	if f.data != nil {
		return f.data.RawRows()
	}
	return f.Rows
}

func (f *Frame) Data() DataContainer {
	return f.data
}

// ConvertValue converte de forma genérica um valor bruto para um tipo reflexivo do Go
func ConvertValue(val any, targetType reflect.Type) (reflect.Value, error) {
	if val == nil {
		return reflect.Zero(targetType), nil
	}

	// Se for Frame, extrai o container
	if f, ok := val.(*Frame); ok {
		return f.data.ToGoValue(targetType)
	}
	// Se for DataContainer, delega para ele
	if dc, ok := val.(DataContainer); ok {
		return dc.ToGoValue(targetType)
	}

	valVal := reflect.ValueOf(val)

	// Achatamento de lista para escalar (se o destino não for slice, mas a entrada for slice)
	if (valVal.Kind() == reflect.Slice || valVal.Kind() == reflect.Array) &&
		targetType.Kind() != reflect.Slice && targetType.Kind() != reflect.Array {
		if valVal.Len() == 0 {
			return reflect.Zero(targetType), nil
		}
		return ConvertValue(valVal.Index(0).Interface(), targetType)
	}

	// Conversão de slice/array para slice (ex: []any para []string)
	if targetType.Kind() == reflect.Slice && (valVal.Kind() == reflect.Slice || valVal.Kind() == reflect.Array) {
		length := valVal.Len()
		sliceVal := reflect.MakeSlice(targetType, length, length)
		for i := 0; i < length; i++ {
			converted, err := ConvertValue(valVal.Index(i).Interface(), targetType.Elem())
			if err != nil {
				return reflect.Value{}, err
			}
			sliceVal.Index(i).Set(converted)
		}
		return sliceVal, nil
	}

	// Envelopamento de escalar para lista (se o destino for slice, mas a entrada não for slice)
	if targetType.Kind() == reflect.Slice &&
		valVal.Kind() != reflect.Slice && valVal.Kind() != reflect.Array {
		sliceVal := reflect.MakeSlice(targetType, 1, 1)
		converted, err := ConvertValue(val, targetType.Elem())
		if err != nil {
			return reflect.Value{}, err
		}
		sliceVal.Index(0).Set(converted)
		return sliceVal, nil
	}

	if valVal.Type().AssignableTo(targetType) {
		return valVal, nil
	}
	if valVal.Type().ConvertibleTo(targetType) {
		return valVal.Convert(targetType), nil
	}

	// Lidar com ponteiros de destino
	isPtr := targetType.Kind() == reflect.Ptr
	baseTargetType := targetType
	if isPtr {
		baseTargetType = targetType.Elem()
	}

	// Conversões numéricas genéricas (ex: float64 para int64 e vice-versa)
	if valVal.Kind() >= reflect.Int && valVal.Kind() <= reflect.Float64 &&
		baseTargetType.Kind() >= reflect.Int && baseTargetType.Kind() <= reflect.Float64 {
		switch baseTargetType.Kind() {
		case reflect.Int, reflect.Int64:
			var intVal int64
			if valVal.Kind() == reflect.Float64 {
				intVal = int64(valVal.Float())
			} else {
				intVal = valVal.Int()
			}
			res := reflect.ValueOf(intVal)
			if baseTargetType.Kind() == reflect.Int {
				res = reflect.ValueOf(int(intVal))
			}
			if isPtr {
				ptr := reflect.New(baseTargetType)
				ptr.Elem().Set(res)
				return ptr, nil
			}
			return res, nil
		case reflect.Float64:
			var floatVal float64
			if valVal.Kind() == reflect.Float64 {
				floatVal = valVal.Float()
			} else {
				floatVal = float64(valVal.Int())
			}
			res := reflect.ValueOf(floatVal)
			if isPtr {
				ptr := reflect.New(baseTargetType)
				ptr.Elem().Set(res)
				return ptr, nil
			}
			return res, nil
		}
	}

	// Caso especial: Deserialização de mapas do Heddle para Structs do Go
	if baseTargetType.Kind() == reflect.Struct {
		if m, ok := val.(map[string]any); ok {
			newStruct := reflect.New(baseTargetType).Elem()
			for k, v := range m {
				fieldName := SnakeToPascal(k)
				field := newStruct.FieldByName(fieldName)
				if !field.IsValid() {
					field = newStruct.FieldByName(k)
				}
				if !field.IsValid() {
					// Busca case-insensitive
					for i := 0; i < baseTargetType.NumField(); i++ {
						f := baseTargetType.Field(i)
						if strings.EqualFold(f.Name, k) || SnakeToPascal(f.Name) == SnakeToPascal(k) {
							field = newStruct.Field(i)
							break
						}
					}
				}

				if field.IsValid() && field.CanSet() {
					convertedVal, err := ConvertValue(v, field.Type())
					if err == nil {
						field.Set(convertedVal)
					}
				}
			}
			if isPtr {
				return newStruct.Addr(), nil
			}
			return newStruct, nil
		}
	}

	return reflect.Value{}, fmt.Errorf("cannot convert %T (value: %v) to target type %v", val, val, targetType)
}

// FromGoValue constrói um Frame Heddle a partir de qualquer retorno do Go
func FromGoValue(val any) *Frame {
	if val == nil {
		return NewFrame([]any{})
	}
	if f, ok := val.(*Frame); ok {
		return f
	}
	if dc, ok := val.(DataContainer); ok {
		return NewFrameFromData(dc)
	}

	rows := toNestedList(val)
	return NewFrame(rows)
}

func toSnakeCase(s string) string {
	var res []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			res = append(res, '_')
		}
		if r >= 'A' && r <= 'Z' {
			res = append(res, r+32)
		} else {
			res = append(res, r)
		}
	}
	return string(res)
}

func SnakeToPascal(s string) string {
	parts := strings.Split(s, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[0:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}
