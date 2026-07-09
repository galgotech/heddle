package env

import (
	"os"
	"strconv"
)

// String retrieves the string value of the environment variable named by the key.
func String(key string) (string, error) {
	return os.Getenv(key), nil
}

// Int retrieves the integer value of the environment variable named by the key.
// If the variable is empty or cannot be parsed, it returns 0.
func Int(key string) (int, error) {
	valStr := os.Getenv(key)
	if valStr == "" {
		return 0, nil
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return 0, err
	}
	return val, nil
}
