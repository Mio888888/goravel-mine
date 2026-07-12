package crudgen

import "strings"

func pascalName(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '_' || r == '-' || r == ' ' || r == '.'
	})
	for i, part := range parts {
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, "")
}

func camelName(value string) string {
	pascal := pascalName(value)
	if pascal == "" {
		return ""
	}
	return strings.ToLower(pascal[:1]) + pascal[1:]
}

func singular(value string) string {
	if strings.HasSuffix(value, "ies") && len(value) > 3 {
		return strings.TrimSuffix(value, "ies") + "y"
	}
	if strings.HasSuffix(value, "ses") && len(value) > 3 {
		return strings.TrimSuffix(value, "es")
	}
	if strings.HasSuffix(value, "s") && len(value) > 1 {
		return strings.TrimSuffix(value, "s")
	}
	return value
}

func kebabName(value string) string {
	return strings.ReplaceAll(value, "_", "-")
}

func packageName(value string) string {
	name := strings.ToLower(value)
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")
	if name == "" {
		return "generated"
	}
	return name
}
