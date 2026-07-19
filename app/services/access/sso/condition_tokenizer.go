package sso

import "strings"

func tokenizeCondition(condition string) []string {
	tokens := make([]string, 0)
	for i := 0; i < len(condition); {
		if condition[i] == ' ' || condition[i] == '\t' ||
			condition[i] == '\n' || condition[i] == '\r' {
			i++
			continue
		}
		if token, next, ok := quotedConditionToken(condition, i); ok {
			tokens = append(tokens, token)
			i = next
			continue
		}
		if token, next, ok := arrayConditionToken(condition, i); ok {
			tokens = append(tokens, token)
			i = next
			continue
		}
		if token, next, ok := variableConditionToken(condition, i); ok {
			tokens = append(tokens, token)
			i = next
			continue
		}
		if token, next, ok := operatorConditionToken(condition, i); ok {
			tokens = append(tokens, token)
			i = next
			continue
		}
		start := i
		for i < len(condition) && isConditionIdentifierByte(condition[i]) {
			i++
		}
		if start == i {
			return nil
		}
		tokens = append(tokens, condition[start:i])
	}
	return tokens
}

func quotedConditionToken(input string, start int) (string, int, bool) {
	if input[start] != '\'' && input[start] != '"' {
		return "", start, false
	}
	quote := input[start]
	for i := start + 1; i < len(input); i++ {
		if input[i] == '\\' {
			i++
			continue
		}
		if input[i] == quote {
			return input[start : i+1], i + 1, true
		}
	}
	return "", start, false
}

func arrayConditionToken(input string, start int) (string, int, bool) {
	if input[start] != '[' {
		return "", start, false
	}
	depth := 0
	for i := start; i < len(input); i++ {
		if _, next, ok := quotedConditionToken(input, i); ok {
			i = next - 1
			continue
		}
		if input[i] == '[' {
			depth++
		}
		if input[i] == ']' {
			depth--
			if depth == 0 {
				return input[start : i+1], i + 1, true
			}
		}
	}
	return "", start, false
}

func variableConditionToken(input string, start int) (string, int, bool) {
	if !strings.HasPrefix(input[start:], "{{") {
		return "", start, false
	}
	end := strings.Index(input[start+2:], "}}")
	if end < 0 {
		return "", start, false
	}
	next := start + 2 + end + 2
	return strings.TrimSpace(input[start+2 : start+2+end]), next, true
}

func operatorConditionToken(input string, start int) (string, int, bool) {
	for _, op := range []string{
		"not_matches", "starts_with", "ends_with", "not_in", "contains", "matches", "in",
	} {
		if strings.HasPrefix(input[start:], op) {
			end := start + len(op)
			if end < len(input) && isConditionIdentifierByte(input[end]) {
				continue
			}
			return op, start + len(op), true
		}
	}
	for _, op := range []string{"&&", "||", ">=", "<=", "==", "!=", ">", "<", "!", "(", ")"} {
		if strings.HasPrefix(input[start:], op) {
			return op, start + len(op), true
		}
	}
	return "", start, false
}

func isConditionIdentifierByte(value byte) bool {
	return value == '_' || value == '.' || value == '-' ||
		(value >= 'A' && value <= 'Z') ||
		(value >= 'a' && value <= 'z') ||
		(value >= '0' && value <= '9')
}
