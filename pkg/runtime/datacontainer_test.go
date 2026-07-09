package runtime

import (
	"reflect"
	"testing"
)

type TestNestedStruct struct {
	B int `json:"b"`
}

type TestRootStruct struct {
	A TestNestedStruct `json:"a"`
}

func TestNestedListNormalization(t *testing.T) {
	// Teste 1: Estrutura física aninhada com structs convertidos em mapas e listas
	val := TestRootStruct{
		A: TestNestedStruct{B: 123},
	}

	container := NewNestedListContainer([]any{val})
	rows := container.RawRows()

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	row0, ok := rows[0].(map[string]any)
	if !ok {
		t.Fatalf("expected row 0 to be map[string]any, got %T", rows[0])
	}

	// a deve ser []any contendo map[string]any
	aVal, exists := row0["a"]
	if !exists {
		t.Fatalf("expected key 'a' to exist in row 0")
	}

	aList, ok := aVal.([]any)
	if !ok {
		t.Fatalf("expected value of 'a' to be []any, got %T", aVal)
	}
	if len(aList) != 1 {
		t.Fatalf("expected 'a' list to have 1 element, got %d", len(aList))
	}

	nestedMap, ok := aList[0].(map[string]any)
	if !ok {
		t.Fatalf("expected element of 'a' list to be map[string]any, got %T", aList[0])
	}

	// b deve ser []any contendo 123
	bVal, exists := nestedMap["b"]
	if !exists {
		t.Fatalf("expected key 'b' to exist in nested map")
	}

	bList, ok := bVal.([]any)
	if !ok {
		t.Fatalf("expected value of 'b' to be []any, got %T", bVal)
	}
	if len(bList) != 1 {
		t.Fatalf("expected 'b' list to have 1 element, got %d", len(bList))
	}

	if bList[0] != 123 {
		t.Errorf("expected leaf value to be 123, got %v", bList[0])
	}
}

func TestPropertyResolutionAndFlattening(t *testing.T) {
	// Preparar um container com dados duplicados na lista para validar flattening
	data := []any{
		map[string]any{
			"a": []any{
				map[string]any{"b": []any{10}},
				map[string]any{"b": []any{20}},
			},
		},
		map[string]any{
			"a": []any{
				map[string]any{"b": []any{30}},
			},
		},
	}

	container := NewNestedListContainer(data)

	// Resolver .a
	resA, err := container.GetProperty([]string{"a"})
	if err != nil {
		t.Fatalf("failed resolving 'a': %v", err)
	}

	rowsA := resA.RawRows()
	// Deve ter 3 elementos mapeados e achatados de .a (2 do primeiro mapa, 1 do segundo)
	if len(rowsA) != 3 {
		t.Fatalf("expected 3 elements after resolving 'a', got %d: %v", len(rowsA), rowsA)
	}

	// Resolver .a.b
	resB, err := container.GetProperty([]string{"a", "b"})
	if err != nil {
		t.Fatalf("failed resolving 'a.b': %v", err)
	}

	rowsB := resB.RawRows()
	if len(rowsB) != 3 {
		t.Fatalf("expected 3 elements after resolving 'a.b', got %d: %v", len(rowsB), rowsB)
	}

	expected := []any{10, 20, 30}
	for i, val := range expected {
		if rowsB[i] != val {
			t.Errorf("expected element %d to be %v, got %v", i, val, rowsB[i])
		}
	}
}

func TestDataContainerToGoValue(t *testing.T) {
	// Teste de desempacotamento de lista para escalar no Go
	container := NewNestedListContainer([]any{123})

	// 1. Converter para int
	valInt, err := container.ToGoValue(reflect.TypeOf(0))
	if err != nil {
		t.Fatalf("failed converting to int: %v", err)
	}
	if valInt.Int() != 123 {
		t.Errorf("expected 123, got %v", valInt.Interface())
	}

	// 2. Converter para []int
	valSlice, err := container.ToGoValue(reflect.TypeOf([]int{}))
	if err != nil {
		t.Fatalf("failed converting to []int: %v", err)
	}
	if valSlice.Len() != 1 || valSlice.Index(0).Int() != 123 {
		t.Errorf("expected []int{123}, got %v", valSlice.Interface())
	}
}

func TestUpdateNestedProperty(t *testing.T) {
	data := []any{
		map[string]any{
			"a": []any{
				map[string]any{"b": []any{100}},
			},
		},
	}

	container := NewNestedListContainer(data)

	// Atualizar .a.b para 200
	updated, err := container.UpdateProperty([]string{"a", "b"}, 200)
	if err != nil {
		t.Fatalf("failed updating property: %v", err)
	}

	// Resolver o novo valor para conferir
	res, err := updated.GetProperty([]string{"a", "b"})
	if err != nil {
		t.Fatalf("failed getting updated property: %v", err)
	}

	rows := res.RawRows()
	if len(rows) != 1 || rows[0] != 200 {
		t.Errorf("expected updated value to be 200, got %v", rows)
	}
}
