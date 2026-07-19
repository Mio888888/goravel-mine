package redaction

import (
	"strings"

	"goravel/app/facades"
)

const Value = "***REDACTED***"

func SensitiveData(value any) any {
	return sensitiveData(value, sensitiveFieldSet())
}

func ConfigStringSlice(key string) []string {
	value := facades.Config().Get(key)
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	case string:
		if typed == "" {
			return nil
		}
		parts := strings.Split(typed, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		return out
	default:
		return nil
	}
}

func sensitiveData(value any, fields map[string]struct{}) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if _, ok := fields[strings.ToLower(key)]; ok {
				out[key] = Value
				continue
			}
			out[key] = sensitiveData(item, fields)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = sensitiveData(item, fields)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(typed))
		for i, item := range typed {
			out[i] = sensitiveData(item, fields).(map[string]any)
		}
		return out
	default:
		return value
	}
}

func sensitiveFieldSet() map[string]struct{} {
	items := ConfigStringSlice("security.sensitive_data.fields")
	if len(items) == 0 {
		items = []string{"password", "token", "secret", "client_secret"}
	}
	fields := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			fields[item] = struct{}{}
		}
	}
	return fields
}
