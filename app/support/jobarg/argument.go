package jobarg

func String(args []any, index int) string {
	if index >= len(args) {
		return ""
	}
	value, _ := args[index].(string)
	return value
}

func Uint64(args []any, index int) uint64 {
	if index >= len(args) {
		return 0
	}
	switch value := args[index].(type) {
	case uint64:
		return value
	case uint:
		return uint64(value)
	case int:
		if value >= 0 {
			return uint64(value)
		}
	case int64:
		if value >= 0 {
			return uint64(value)
		}
	}
	return 0
}
