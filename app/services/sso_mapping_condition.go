package services

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

type ssoConditionParser struct {
	tokens []string
	pos    int
	claims map[string]any
}

func evalSSOCondition(condition string, claims map[string]any) bool {
	parser := ssoConditionParser{tokens: tokenizeSSOCondition(condition), claims: claims}
	return parser.parseExpression() && parser.pos == len(parser.tokens)
}

func (p *ssoConditionParser) parseExpression() bool {
	return p.parseOr()
}

func (p *ssoConditionParser) parseOr() bool {
	result := p.parseAnd()
	for p.match("||") {
		result = p.parseAnd() || result
	}
	return result
}

func (p *ssoConditionParser) parseAnd() bool {
	result := p.parseFactor()
	for p.match("&&") {
		result = p.parseFactor() && result
	}
	return result
}

func (p *ssoConditionParser) parseFactor() bool {
	if p.match("!") {
		return !p.parseFactor()
	}
	if p.match("(") {
		result := p.parseExpression()
		return p.match(")") && result
	}
	return p.parseComparison()
}

func (p *ssoConditionParser) parseComparison() bool {
	leftToken, ok := p.next()
	if !ok {
		return false
	}
	operator, ok := p.next()
	if !ok {
		return false
	}
	rightToken, ok := p.next()
	if !ok {
		return false
	}
	return evalSSOComparison(p.claimValue(leftToken), operator, parseConditionValue(rightToken))
}

func (p *ssoConditionParser) claimValue(token string) any {
	if value, ok := p.claims[token]; ok {
		return value
	}
	return parseConditionValue(token)
}

func (p *ssoConditionParser) match(token string) bool {
	if p.pos >= len(p.tokens) || p.tokens[p.pos] != token {
		return false
	}
	p.pos++
	return true
}

func (p *ssoConditionParser) next() (string, bool) {
	if p.pos >= len(p.tokens) {
		return "", false
	}
	token := p.tokens[p.pos]
	p.pos++
	return token, true
}

func evalSSOComparison(left any, operator string, right any) bool {
	switch operator {
	case "==":
		return anyStringEquals(left, fmt.Sprint(right))
	case "!=":
		return !anyStringEquals(left, fmt.Sprint(right))
	case "contains":
		return anyStringContains(left, fmt.Sprint(right))
	case "in":
		return rightContains(right, left)
	case "not_in":
		return !rightContains(right, left)
	case "starts_with":
		return strings.HasPrefix(firstString(left), fmt.Sprint(right))
	case "ends_with":
		return strings.HasSuffix(firstString(left), fmt.Sprint(right))
	case "matches":
		ok, _ := regexp.MatchString(fmt.Sprint(right), firstString(left))
		return ok
	case "not_matches":
		ok, _ := regexp.MatchString(fmt.Sprint(right), firstString(left))
		return !ok
	case ">", ">=", "<", "<=":
		return compareNumbers(left, right, operator)
	default:
		return false
	}
}

func parseConditionValue(value string) any {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
		return value[1 : len(value)-1]
	}
	switch value {
	case "true":
		return true
	case "false":
		return false
	case "null":
		return nil
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		var values []any
		if err := json.Unmarshal([]byte(strings.ReplaceAll(value, "'", `"`)), &values); err == nil {
			return values
		}
	}
	if number, err := strconv.ParseFloat(value, 64); err == nil {
		return number
	}
	return value
}

func anyStringEquals(value any, expected string) bool {
	for _, item := range stringsFromAny(value) {
		if item == expected {
			return true
		}
	}
	return false
}

func anyStringContains(value any, expected string) bool {
	for _, item := range stringsFromAny(value) {
		if strings.Contains(item, expected) {
			return true
		}
	}
	return false
}

func rightContains(right any, left any) bool {
	leftValues := stringsFromAny(left)
	for _, rightValue := range stringsFromAny(right) {
		for _, leftValue := range leftValues {
			if rightValue == leftValue {
				return true
			}
		}
	}
	return false
}

func firstString(value any) string {
	values := stringsFromAny(value)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func compareNumbers(left, right any, operator string) bool {
	leftNumber, leftOK := floatFromAny(left)
	rightNumber, rightOK := floatFromAny(right)
	if !leftOK || !rightOK {
		return false
	}
	switch operator {
	case ">":
		return leftNumber > rightNumber
	case ">=":
		return leftNumber >= rightNumber
	case "<":
		return leftNumber < rightNumber
	case "<=":
		return leftNumber <= rightNumber
	default:
		return false
	}
}

func floatFromAny(value any) (float64, bool) {
	if values := stringsFromAny(value); len(values) > 0 {
		parsed, err := strconv.ParseFloat(values[0], 64)
		return parsed, err == nil
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(reflected.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(reflected.Uint()), true
	case reflect.Float32, reflect.Float64:
		return reflected.Float(), true
	default:
		return 0, false
	}
}
