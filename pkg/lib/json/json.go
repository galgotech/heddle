package json

import (
	"encoding/json"
)

func Marshal(v any) (string, error) {
	bytes, err := json.Marshal(v)
	return string(bytes), err
}

func Unmarshal(s string) (any, error) {
	var val any
	err := json.Unmarshal([]byte(s), &val)
	return val, err
}
