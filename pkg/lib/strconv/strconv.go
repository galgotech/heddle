package strconv

import (
	"strconv"
)

func ParseInt(s string, base int, bitSize int) (int64, error) {
	return strconv.ParseInt(s, base, bitSize)
}
