package cmp

import (
	"fmt"
	"reflect"

	"heddle/pkg/runtime/interaction"
)

type ComparTag string

const (
	Less    ComparTag = "less"
	Equals  ComparTag = "equals"
	Greater ComparTag = "greater"
)

func Compare(x any, y any) (interaction.Match[ComparTag, any], error) {
	xf, ok1 := convertToFloat(x)
	yf, ok2 := convertToFloat(y)
	if ok1 && ok2 {
		if xf < yf {
			return interaction.Match[ComparTag, any]{Tag: Less, Val: xf}, nil
		} else if xf > yf {
			return interaction.Match[ComparTag, any]{Tag: Greater, Val: xf}, nil
		} else {
			return interaction.Match[ComparTag, any]{Tag: Equals, Val: xf}, nil
		}
	}
	xs := fmt.Sprintf("%v", x)
	ys := fmt.Sprintf("%v", y)
	if xs < ys {
		return interaction.Match[ComparTag, any]{Tag: Less, Val: xs}, nil
	} else if xs > ys {
		return interaction.Match[ComparTag, any]{Tag: Greater, Val: xs}, nil
	} else {
		return interaction.Match[ComparTag, any]{Tag: Equals, Val: xs}, nil
	}
}

func convertToFloat(v any) (float64, bool) {
	if v == nil {
		return 0, false
	}
	val := reflect.ValueOf(v)
	for val.Kind() == reflect.Ptr || val.Kind() == reflect.Interface {
		if val.IsNil() {
			return 0, false
		}
		val = val.Elem()
	}
	switch val.Kind() {
	case reflect.Int, reflect.Int64:
		return float64(val.Int()), true
	case reflect.Float64:
		return val.Float(), true
	case reflect.Float32:
		return float64(val.Float()), true
	}
	return 0, false
}
