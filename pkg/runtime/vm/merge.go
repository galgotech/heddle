package vm

import (
	"fmt"
	"reflect"

	"heddle/pkg/runtime"
)

type tupleMerger struct {
	vm *VM
}

func (m *tupleMerger) MergeResults(results []*runtime.Frame) []any {
	var consolidatedRows []any
	maxLength := 0
	for _, f := range results {
		if f != nil && len(f.Rows) > maxLength {
			maxLength = len(f.Rows)
		}
	}

	for idx := 0; idx < maxLength; idx++ {
		// Verificar se todas as linhas não nulas neste índice são maps
		allMaps := true
		nonNilCount := 0
		for _, f := range results {
			if f != nil && idx < len(f.Rows) {
				val := f.Rows[idx]
				if val != nil {
					nonNilCount++
					if _, ok := val.(map[string]any); !ok {
						allMaps = false
					}
				}
			}
		}

		if allMaps && nonNilCount > 0 {
			// Deep merge de todas as linhas
			var merged map[string]any
			for _, f := range results {
				if f != nil && idx < len(f.Rows) {
					val := f.Rows[idx]
					if val != nil {
						merged = deepMergeMaps(merged, val.(map[string]any))
					}
				}
			}
			consolidatedRows = append(consolidatedRows, merged)
		} else {
			// Fallback: empacotar em val0, val1, etc.
			tupleRow := make(map[string]any)
			for branchIdx, f := range results {
				key := fmt.Sprintf("val%d", branchIdx)
				if f != nil && idx < len(f.Rows) {
					tupleRow[key] = f.Rows[idx]
				} else {
					tupleRow[key] = nil
				}
			}
			consolidatedRows = append(consolidatedRows, tupleRow)
		}
	}
	return consolidatedRows
}

func deepMergeMaps(dest, src map[string]any) map[string]any {
	if dest == nil {
		dest = make(map[string]any)
	}
	res := make(map[string]any)
	for k, v := range dest {
		res[k] = v
	}
	for k, v := range src {
		if existing, found := res[k]; found {
			res[k] = deepMergeValues(existing, v)
		} else {
			res[k] = v
		}
	}
	return res
}

func deepMergeValues(destVal, srcVal any) any {
	if destVal == nil {
		return srcVal
	}
	if srcVal == nil {
		return destVal
	}

	dVal := reflect.ValueOf(destVal)
	sVal := reflect.ValueOf(srcVal)

	// Se ambos forem slices, mescla elemento a elemento
	if (dVal.Kind() == reflect.Slice || dVal.Kind() == reflect.Array) &&
		(sVal.Kind() == reflect.Slice || sVal.Kind() == reflect.Array) {
		maxLen := dVal.Len()
		if sVal.Len() > maxLen {
			maxLen = sVal.Len()
		}
		resSlice := make([]any, maxLen)
		for i := 0; i < maxLen; i++ {
			var dElem, sElem any
			if i < dVal.Len() {
				dElem = dVal.Index(i).Interface()
			}
			if i < sVal.Len() {
				sElem = sVal.Index(i).Interface()
			}
			resSlice[i] = deepMergeValues(dElem, sElem)
		}
		return resSlice
	}

	// Se ambos forem mapas, mescla recursivamente
	dMap, ok1 := destVal.(map[string]any)
	sMap, ok2 := srcVal.(map[string]any)
	if ok1 && ok2 {
		return deepMergeMaps(dMap, sMap)
	}

	// Fallback: o valor de src sobrescreve o de dest
	return srcVal
}
