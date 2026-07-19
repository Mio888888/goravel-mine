package idutil

func PayloadIDs(values []any, key string) []uint64 {
	ids := make([]uint64, 0, len(values))
	for _, value := range values {
		switch typed := value.(type) {
		case float64:
			ids = append(ids, uint64(typed))
		case int:
			ids = append(ids, uint64(typed))
		case uint64:
			ids = append(ids, typed)
		case map[string]any:
			ids = append(ids, payloadMapID(typed, key))
		}
	}
	return CompactPositiveIDs(ids)
}

func CompactPositiveIDs(values []uint64) []uint64 {
	out := make([]uint64, 0, len(values))
	for _, value := range values {
		if value > 0 {
			out = append(out, value)
		}
	}
	return out
}

func payloadMapID(value map[string]any, key string) uint64 {
	switch typed := value[key].(type) {
	case float64:
		return uint64(typed)
	case int:
		return uint64(typed)
	case uint64:
		return typed
	default:
		return 0
	}
}
