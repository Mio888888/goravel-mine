package services

func payloadIDs(values []any, key string) []uint64 {
	ids := make([]uint64, 0, len(values))
	for _, value := range values {
		switch v := value.(type) {
		case float64:
			ids = append(ids, uint64(v))
		case int:
			ids = append(ids, uint64(v))
		case uint64:
			ids = append(ids, v)
		case map[string]any:
			ids = append(ids, payloadMapID(v, key))
		}
	}
	return compactPositiveIDs(ids)
}

func payloadMapID(value map[string]any, key string) uint64 {
	switch v := value[key].(type) {
	case float64:
		return uint64(v)
	case int:
		return uint64(v)
	case uint64:
		return v
	default:
		return 0
	}
}

func compactPositiveIDs(values []uint64) []uint64 {
	out := make([]uint64, 0, len(values))
	for _, value := range values {
		if value > 0 {
			out = append(out, value)
		}
	}
	return out
}
