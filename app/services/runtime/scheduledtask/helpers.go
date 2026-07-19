package scheduledtask

import (
	"fmt"
	"strings"

	"goravel/app/models"
)

func jsonString(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func nullIfEmpty(value models.JSONMap) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func uint64Any(values []uint64) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}
