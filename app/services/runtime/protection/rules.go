package protection

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"goravel/app/models"
)

const (
	ScopeGlobal   = "GLOBAL"
	ScopeService  = "SERVICE"
	ScopeEndpoint = "ENDPOINT"
	ScopeCustom   = "CUSTOM"

	RuleTypeRateLimit          = "RATE_LIMIT"
	RuleTypeSlowCallCircuit    = "SLOW_CALL_CIRCUIT"
	RuleTypeFailureRateCircuit = "FAILURE_RATE_CIRCUIT"
	RuleTypeConcurrency        = "CONCURRENCY"

	StatusDraft     = "DRAFT"
	StatusPublished = "PUBLISHED"
	StatusArchived  = "ARCHIVED"
)

var resourcePattern = regexp.MustCompile(`^[A-Za-z0-9_./:{}-]+(?:\*)?$`)

type Rule struct {
	Type                string `json:"type"`
	Limit               int    `json:"limit,omitempty"`
	WindowMS            int    `json:"window_ms,omitempty"`
	SlowCallDurationMS  int    `json:"slow_call_duration_ms,omitempty"`
	ThresholdPercent    int    `json:"threshold_percent,omitempty"`
	MinimumRequests     int    `json:"minimum_requests,omitempty"`
	StatisticalWindowMS int    `json:"statistical_window_ms,omitempty"`
	OpenDurationMS      int    `json:"open_duration_ms,omitempty"`
	HalfOpenProbes      int    `json:"half_open_probes,omitempty"`
	HalfOpenSuccesses   int    `json:"half_open_successes,omitempty"`
	MaxConcurrency      int    `json:"max_concurrency,omitempty"`
}

type RuleConfig struct {
	Rules []Rule `json:"rules"`
}

type PublishedRuleSet struct {
	RuleSetID       uint64
	Version         int
	Name            string
	Scope           string
	ResourcePattern string
	Rules           []Rule
}

type ValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

func ParseRuleConfig(value models.JSONMap) (RuleConfig, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return RuleConfig{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var config RuleConfig
	if err := decoder.Decode(&config); err != nil {
		return RuleConfig{}, fmt.Errorf("保护规则格式无效: %w", err)
	}
	for index := range config.Rules {
		config.Rules[index].Type = strings.ToUpper(strings.TrimSpace(config.Rules[index].Type))
	}
	return config, nil
}

func ValidateRuleSet(scope, pattern string, value models.JSONMap) ValidationResult {
	result := ValidationResult{Valid: true, Errors: []string{}, Warnings: []string{}}
	fail := func(message string) {
		result.Valid = false
		result.Errors = append(result.Errors, message)
	}

	scope = strings.ToUpper(strings.TrimSpace(scope))
	pattern = strings.TrimSpace(pattern)
	switch scope {
	case ScopeGlobal:
		if pattern != "*" {
			fail("全局规则的资源表达式必须为 *")
		}
	case ScopeService, ScopeEndpoint, ScopeCustom:
		if pattern == "" || pattern == "*" || len(pattern) > 200 || !resourcePattern.MatchString(pattern) {
			fail("资源表达式无效，仅允许受控的精确值或末尾通配符")
		}
	default:
		fail("保护规则作用域无效")
	}

	config, err := ParseRuleConfig(value)
	if err != nil {
		fail(err.Error())
		return result
	}
	if len(config.Rules) == 0 {
		fail("保护规则至少需要一条规则")
	}
	if len(config.Rules) > 16 {
		fail("单个规则集最多允许 16 条规则")
	}

	seen := make(map[string]struct{}, len(config.Rules))
	for _, rule := range config.Rules {
		if _, exists := seen[rule.Type]; exists {
			fail("同一规则集中不能重复配置 " + rule.Type)
			continue
		}
		seen[rule.Type] = struct{}{}
		switch rule.Type {
		case RuleTypeRateLimit:
			if rule.Limit < 1 || rule.Limit > 1_000_000_000 {
				fail("限流阈值必须在 1 到 1000000000 之间")
			}
			if !validDurationMS(rule.WindowMS, 100, 24*time.Hour) {
				fail("限流窗口必须在 100 毫秒到 24 小时之间")
			}
		case RuleTypeSlowCallCircuit:
			validateCircuitRule(rule, true, fail)
		case RuleTypeFailureRateCircuit:
			validateCircuitRule(rule, false, fail)
		case RuleTypeConcurrency:
			if rule.MaxConcurrency < 1 || rule.MaxConcurrency > 1_000_000 {
				fail("并发隔离上限必须在 1 到 1000000 之间")
			}
		default:
			fail("不支持的保护规则类型: " + rule.Type)
		}
	}

	sort.Strings(result.Errors)
	return result
}

func validateCircuitRule(rule Rule, slowCall bool, fail func(string)) {
	if rule.ThresholdPercent < 1 || rule.ThresholdPercent > 100 {
		fail("熔断比例阈值必须在 1 到 100 之间")
	}
	if rule.MinimumRequests < 1 || rule.MinimumRequests > 1_000_000 {
		fail("熔断最小请求数必须在 1 到 1000000 之间")
	}
	if !validDurationMS(rule.StatisticalWindowMS, 1000, 24*time.Hour) {
		fail("熔断统计窗口必须在 1 秒到 24 小时之间")
	}
	if !validDurationMS(rule.OpenDurationMS, 100, 24*time.Hour) {
		fail("熔断持续时间必须在 100 毫秒到 24 小时之间")
	}
	if rule.HalfOpenProbes < 1 || rule.HalfOpenProbes > 1000 {
		fail("半开探测数必须在 1 到 1000 之间")
	}
	if rule.HalfOpenSuccesses < 1 || rule.HalfOpenSuccesses > rule.HalfOpenProbes {
		fail("半开成功数必须在 1 到半开探测数之间")
	}
	if slowCall && !validDurationMS(rule.SlowCallDurationMS, 1, 24*time.Hour) {
		fail("慢调用阈值必须在 1 毫秒到 24 小时之间")
	}
}

func validDurationMS(value, minimum int, maximum time.Duration) bool {
	return value >= minimum && time.Duration(value)*time.Millisecond <= maximum
}

func ValidatePublishedConflicts(ruleSets []PublishedRuleSet) error {
	seen := make(map[string]uint64, len(ruleSets))
	for _, ruleSet := range ruleSets {
		key := strings.ToUpper(strings.TrimSpace(ruleSet.Scope)) + "\x00" + strings.TrimSpace(ruleSet.ResourcePattern)
		if existing, exists := seen[key]; exists && existing != ruleSet.RuleSetID {
			return fmt.Errorf("保护规则与规则集 %d 的作用域和资源表达式冲突", existing)
		}
		seen[key] = ruleSet.RuleSetID
	}
	return nil
}

func ruleConfigJSONMap(config RuleConfig) models.JSONMap {
	encoded, _ := json.Marshal(config)
	var value models.JSONMap
	_ = json.Unmarshal(encoded, &value)
	return value
}

func normalizeScope(scope string) string {
	return strings.ToUpper(strings.TrimSpace(scope))
}

func normalizePattern(pattern string) string {
	return strings.TrimSpace(pattern)
}

func rulesFromJSON(value models.JSONMap) ([]Rule, error) {
	config, err := ParseRuleConfig(value)
	if err != nil {
		return nil, err
	}
	return config.Rules, nil
}

func validationError(result ValidationResult) error {
	if result.Valid {
		return nil
	}
	if len(result.Errors) == 0 {
		return errors.New("保护规则校验失败")
	}
	return errors.New(strings.Join(result.Errors, "；"))
}
