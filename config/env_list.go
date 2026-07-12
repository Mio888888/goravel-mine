package config

import (
	"strings"

	"goravel/app/facades"
)

func envStringList(name string, fallback ...string) []string {
	return splitConfigList(facades.Config().EnvString(name), fallback...)
}

func splitConfigList(value string, fallback ...string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	if len(out) > 0 {
		return out
	}
	return append([]string(nil), fallback...)
}
