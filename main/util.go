package main

import (
	"encoding/json"
	"strconv"
	"strings"
)

func to_pretty_json[T any](obj T) string {
	bs, _ := json.MarshalIndent(obj, "", "  ")
	return string(bs)
}

// dec: 123
// hex: 1a, 0x1a, 1A
func parse_any_int(s string) (uint64, error) {
	var is_hex bool

	if strings.ContainsAny(s, "abcdefABCDEF") {
		is_hex = true
	}
	if strings.Contains(s, "0x") {
		is_hex = true
		s = strings.ReplaceAll(s, "0x", "")
	}

	if is_hex {
		return strconv.ParseUint(s, 16, 64)
	} else {
		return strconv.ParseUint(s, 10, 64)
	}
}
