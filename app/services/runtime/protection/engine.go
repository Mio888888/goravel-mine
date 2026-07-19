package protection

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	CircuitClosed   = "CLOSED"
	CircuitOpen     = "OPEN"
	CircuitHalfOpen = "HALF_OPEN"
)

type RequestContext struct {
	Service        string `json:"service"`
	Endpoint       string `json:"endpoint"`
	CustomResource string `json:"custom_resource"`
	RateLimitKey   string `json:"rate_limit_key"`
}

type Decision struct {
	Allowed         bool   `json:"allowed"`
	Matched         bool   `json:"matched"`
	RuleSetID       uint64 `json:"rule_set_id,omitempty"`
	RuleSetVersion  int    `json:"rule_set_version,omitempty"`
	Scope           string `json:"scope,omitempty"`
	ResourcePattern string `json:"resource_pattern,omitempty"`
	Rejection       string `json:"rejection,omitempty"`
	State           string `json:"state,omitempty"`
	Error           error  `json:"-"`
	completionID    uint64
}

type CircuitSnapshot struct {
	RuleType          string     `json:"rule_type"`
	State             string     `json:"state"`
	SampleCount       int        `json:"sample_count"`
	FailureCount      int        `json:"failure_count"`
	SlowCount         int        `json:"slow_count"`
	HalfOpenInFlight  int        `json:"half_open_inflight"`
	HalfOpenSuccesses int        `json:"half_open_successes"`
	OpenedAt          *time.Time `json:"opened_at,omitempty"`
}

type RuleSetState struct {
	RuleSetID  uint64            `json:"rule_set_id"`
	Version    int               `json:"version"`
	Circuits   []CircuitSnapshot `json:"circuits"`
	Concurrent int               `json:"concurrent"`
}

type Metric struct {
	RuleSetID           uint64  `json:"rule_set_id"`
	Version             int     `json:"version"`
	Scope               string  `json:"scope"`
	ResourcePattern     string  `json:"resource_pattern"`
	Passed              uint64  `json:"passed"`
	RateLimited         uint64  `json:"rate_limited"`
	CircuitRejected     uint64  `json:"circuit_rejected"`
	ConcurrencyRejected uint64  `json:"concurrency_rejected"`
	HalfOpenProbes      uint64  `json:"half_open_probes"`
	Calls               uint64  `json:"calls"`
	Failures            uint64  `json:"failures"`
	DurationSumMS       float64 `json:"duration_sum_ms"`
}

type sample struct {
	at      time.Time
	failure bool
	slow    bool
}

type circuitRuntime struct {
	state             string
	openedAt          time.Time
	samples           []sample
	halfOpenInFlight  int
	halfOpenSuccesses int
}

type rateWindow struct {
	start time.Time
	count int
}

type decisionCompletion struct {
	ruleSetID           uint64
	ruleSetVersion      int
	resourceName        string
	rules               []Rule
	acquiredHalfOpen    []string
	acquiredConcurrency bool
}

type Engine struct {
	mu               sync.Mutex
	now              func() time.Time
	ruleSets         []PublishedRuleSet
	circuits         map[string]*circuitRuntime
	rates            map[string]rateWindow
	concurrency      map[uint64]int
	metrics          map[uint64]Metric
	nextCompletionID uint64
	completions      map[uint64]decisionCompletion
}

func NewEngine() *Engine {
	return &Engine{
		now:         time.Now,
		circuits:    make(map[string]*circuitRuntime),
		rates:       make(map[string]rateWindow),
		concurrency: make(map[uint64]int),
		metrics:     make(map[uint64]Metric),
		completions: make(map[uint64]decisionCompletion),
	}
}

func (e *Engine) ReplaceRules(ruleSets []PublishedRuleSet) error {
	if err := ValidatePublishedConflicts(ruleSets); err != nil {
		return err
	}
	copied := make([]PublishedRuleSet, len(ruleSets))
	copy(copied, ruleSets)
	for index := range copied {
		copied[index].Scope = normalizeScope(copied[index].Scope)
		copied[index].ResourcePattern = normalizePattern(copied[index].ResourcePattern)
	}
	sort.Slice(copied, func(i, j int) bool {
		left, right := ruleSetSpecificity(copied[i]), ruleSetSpecificity(copied[j])
		if left != right {
			return left > right
		}
		if len(copied[i].ResourcePattern) != len(copied[j].ResourcePattern) {
			return len(copied[i].ResourcePattern) > len(copied[j].ResourcePattern)
		}
		return copied[i].RuleSetID < copied[j].RuleSetID
	})

	e.mu.Lock()
	defer e.mu.Unlock()
	previousVersions := make(map[uint64]int, len(e.ruleSets))
	for _, ruleSet := range e.ruleSets {
		previousVersions[ruleSet.RuleSetID] = ruleSet.Version
	}
	e.ruleSets = copied
	active := make(map[uint64]struct{}, len(copied))
	for _, ruleSet := range copied {
		active[ruleSet.RuleSetID] = struct{}{}
		if previousVersion, exists := previousVersions[ruleSet.RuleSetID]; exists && previousVersion != ruleSet.Version {
			e.resetRuleSetRuntime(ruleSet.RuleSetID)
		}
		metric := e.metrics[ruleSet.RuleSetID]
		metric.RuleSetID = ruleSet.RuleSetID
		metric.Version = ruleSet.Version
		metric.Scope = ruleSet.Scope
		metric.ResourcePattern = ruleSet.ResourcePattern
		e.metrics[ruleSet.RuleSetID] = metric
	}
	for id := range e.concurrency {
		if _, exists := active[id]; !exists {
			delete(e.concurrency, id)
		}
	}
	return nil
}

func (e *Engine) Evaluate(resourceName string, request RequestContext) Decision {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now().UTC()
	ruleSet, matched := e.matchRuleSet(strings.TrimSpace(resourceName), request)
	if !matched {
		return Decision{Allowed: true, Matched: false}
	}
	decision := Decision{
		Allowed: true, Matched: true, RuleSetID: ruleSet.RuleSetID,
		RuleSetVersion: ruleSet.Version, Scope: ruleSet.Scope,
		ResourcePattern: ruleSet.ResourcePattern,
	}
	metric := e.metrics[ruleSet.RuleSetID]
	acquiredHalfOpen := make([]string, 0, 2)

	for _, rule := range ruleSet.Rules {
		if rule.Type != RuleTypeSlowCallCircuit && rule.Type != RuleTypeFailureRateCircuit {
			continue
		}
		key := circuitKey(ruleSet.RuleSetID, rule.Type)
		state := e.circuit(key)
		if state.state == CircuitOpen && now.Sub(state.openedAt) >= time.Duration(rule.OpenDurationMS)*time.Millisecond {
			state.state = CircuitHalfOpen
			state.halfOpenInFlight = 0
			state.halfOpenSuccesses = 0
		}
		if state.state == CircuitOpen || (state.state == CircuitHalfOpen && state.halfOpenInFlight >= rule.HalfOpenProbes) {
			e.releaseHalfOpen(acquiredHalfOpen)
			metric.CircuitRejected++
			e.metrics[ruleSet.RuleSetID] = metric
			decision.Allowed = false
			decision.Rejection = RejectionCircuitOpen
			decision.State = state.state
			decision.Error = Error{Kind: RejectionCircuitOpen, Resource: resourceName}
			return decision
		}
		if state.state == CircuitHalfOpen {
			state.halfOpenInFlight++
			acquiredHalfOpen = append(acquiredHalfOpen, key)
			metric.HalfOpenProbes++
		}
	}

	for _, rule := range ruleSet.Rules {
		if rule.Type != RuleTypeRateLimit {
			continue
		}
		key := strings.TrimSpace(request.RateLimitKey)
		if key == "" {
			key = resourceName
		}
		windowKey := fmt.Sprintf("%d:%s", ruleSet.RuleSetID, key)
		window := e.rates[windowKey]
		duration := time.Duration(rule.WindowMS) * time.Millisecond
		if window.start.IsZero() || now.Sub(window.start) >= duration {
			window = rateWindow{start: now}
		}
		if window.count >= rule.Limit {
			e.releaseHalfOpen(acquiredHalfOpen)
			metric.RateLimited++
			e.metrics[ruleSet.RuleSetID] = metric
			decision.Allowed = false
			decision.Rejection = RejectionRateLimited
			decision.Error = Error{Kind: RejectionRateLimited, Resource: resourceName}
			return decision
		}
		window.count++
		e.rates[windowKey] = window
	}

	for _, rule := range ruleSet.Rules {
		if rule.Type != RuleTypeConcurrency {
			continue
		}
		if e.concurrency[ruleSet.RuleSetID] >= rule.MaxConcurrency {
			e.releaseHalfOpen(acquiredHalfOpen)
			metric.ConcurrencyRejected++
			e.metrics[ruleSet.RuleSetID] = metric
			decision.Allowed = false
			decision.Rejection = RejectionConcurrencyLimited
			decision.Error = Error{Kind: RejectionConcurrencyLimited, Resource: resourceName}
			return decision
		}
		e.concurrency[ruleSet.RuleSetID]++
	}

	e.nextCompletionID++
	decision.completionID = e.nextCompletionID
	e.completions[decision.completionID] = decisionCompletion{
		ruleSetID:           ruleSet.RuleSetID,
		ruleSetVersion:      ruleSet.Version,
		resourceName:        strings.TrimSpace(resourceName),
		rules:               append([]Rule(nil), ruleSet.Rules...),
		acquiredHalfOpen:    append([]string(nil), acquiredHalfOpen...),
		acquiredConcurrency: hasRuleType(ruleSet.Rules, RuleTypeConcurrency),
	}
	metric.Passed++
	e.metrics[ruleSet.RuleSetID] = metric
	for _, key := range acquiredHalfOpen {
		if state := e.circuits[key]; state != nil {
			decision.State = state.state
		}
	}
	return decision
}

func (e *Engine) RecordSuccess(resourceName string, duration time.Duration) {
	e.record(resourceName, duration, false)
}

func (e *Engine) RecordFailure(resourceName string, duration time.Duration) {
	e.record(resourceName, duration, true)
}

func (e *Engine) RecordDecisionSuccess(decision Decision, duration time.Duration) {
	e.recordDecision(decision, duration, false)
}

func (e *Engine) RecordDecisionFailure(decision Decision, duration time.Duration) {
	e.recordDecision(decision, duration, true)
}

func (e *Engine) State(ruleSetID uint64) RuleSetState {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := RuleSetState{RuleSetID: ruleSetID, Circuits: []CircuitSnapshot{}, Concurrent: e.concurrency[ruleSetID]}
	for _, ruleSet := range e.ruleSets {
		if ruleSet.RuleSetID == ruleSetID {
			result.Version = ruleSet.Version
			break
		}
	}
	for _, ruleType := range []string{RuleTypeFailureRateCircuit, RuleTypeSlowCallCircuit} {
		state, exists := e.circuits[circuitKey(ruleSetID, ruleType)]
		if !exists {
			continue
		}
		snapshot := CircuitSnapshot{
			RuleType: ruleType, State: state.state, SampleCount: len(state.samples),
			HalfOpenInFlight: state.halfOpenInFlight, HalfOpenSuccesses: state.halfOpenSuccesses,
		}
		for _, item := range state.samples {
			if item.failure {
				snapshot.FailureCount++
			}
			if item.slow {
				snapshot.SlowCount++
			}
		}
		if !state.openedAt.IsZero() {
			openedAt := state.openedAt
			snapshot.OpenedAt = &openedAt
		}
		result.Circuits = append(result.Circuits, snapshot)
	}
	return result
}

func (e *Engine) Metrics() []Metric {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make([]Metric, 0, len(e.metrics))
	for _, metric := range e.metrics {
		result = append(result, metric)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].RuleSetID < result[j].RuleSetID })
	return result
}

func (e *Engine) SetNowForTest(now func() time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if now == nil {
		e.now = time.Now
		return
	}
	e.now = now
}

func (e *Engine) record(resourceName string, duration time.Duration, failure bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	resourceName = strings.TrimSpace(resourceName)
	var completionID uint64
	var completion decisionCompletion
	for id, candidate := range e.completions {
		if candidate.resourceName == resourceName && (completionID == 0 || id < completionID) {
			completionID = id
			completion = candidate
		}
	}
	if completionID != 0 {
		delete(e.completions, completionID)
		e.recordRuleSet(
			completion.ruleSetID,
			completion.ruleSetVersion,
			completion.rules,
			completion.acquiredHalfOpen,
			completion.acquiredConcurrency,
			duration,
			failure,
		)
		return
	}
	ruleSet, matched := e.matchRuleSet(strings.TrimSpace(resourceName), RequestContext{
		Service: resourceName, Endpoint: resourceName, CustomResource: resourceName,
	})
	if !matched {
		return
	}
	e.recordRuleSet(ruleSet.RuleSetID, ruleSet.Version, ruleSet.Rules, nil, true, duration, failure)
}

func (e *Engine) recordDecision(decision Decision, duration time.Duration, failure bool) {
	if !decision.Allowed || !decision.Matched || decision.completionID == 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	completion, exists := e.completions[decision.completionID]
	if !exists {
		return
	}
	delete(e.completions, decision.completionID)
	e.recordRuleSet(
		completion.ruleSetID,
		completion.ruleSetVersion,
		completion.rules,
		completion.acquiredHalfOpen,
		completion.acquiredConcurrency,
		duration,
		failure,
	)
}

func (e *Engine) recordRuleSet(
	ruleSetID uint64,
	ruleSetVersion int,
	rules []Rule,
	acquiredHalfOpen []string,
	acquiredConcurrency bool,
	duration time.Duration,
	failure bool,
) {
	now := e.now().UTC()
	if !e.ruleSetVersionActive(ruleSetID, ruleSetVersion) {
		return
	}
	if acquiredConcurrency && e.concurrency[ruleSetID] > 0 {
		e.concurrency[ruleSetID]--
	}
	metric := e.metrics[ruleSetID]
	metric.Calls++
	metric.DurationSumMS += float64(duration.Microseconds()) / 1000
	if failure {
		metric.Failures++
	}
	e.metrics[ruleSetID] = metric

	for _, rule := range rules {
		if rule.Type != RuleTypeSlowCallCircuit && rule.Type != RuleTypeFailureRateCircuit {
			continue
		}
		state := e.circuit(circuitKey(ruleSetID, rule.Type))
		if state.state == CircuitHalfOpen {
			if state.halfOpenInFlight > 0 {
				state.halfOpenInFlight--
			}
			if failure {
				openCircuit(state, now)
				continue
			}
			state.halfOpenSuccesses++
			if state.halfOpenSuccesses >= rule.HalfOpenSuccesses {
				closeCircuit(state)
			}
			continue
		}
		if state.state == CircuitOpen {
			continue
		}

		cutoff := now.Add(-time.Duration(rule.StatisticalWindowMS) * time.Millisecond)
		recent := state.samples[:0]
		for _, item := range state.samples {
			if item.at.After(cutoff) {
				recent = append(recent, item)
			}
		}
		state.samples = append(recent, sample{
			at: now, failure: failure,
			slow: duration >= time.Duration(rule.SlowCallDurationMS)*time.Millisecond,
		})
		if len(state.samples) < rule.MinimumRequests {
			continue
		}
		matchedCount := 0
		for _, item := range state.samples {
			if (rule.Type == RuleTypeFailureRateCircuit && item.failure) ||
				(rule.Type == RuleTypeSlowCallCircuit && item.slow) {
				matchedCount++
			}
		}
		if matchedCount*100 >= rule.ThresholdPercent*len(state.samples) {
			openCircuit(state, now)
		}
	}
}

func (e *Engine) ruleSetVersionActive(ruleSetID uint64, version int) bool {
	for _, ruleSet := range e.ruleSets {
		if ruleSet.RuleSetID == ruleSetID {
			return ruleSet.Version == version
		}
	}
	return false
}

func hasRuleType(rules []Rule, ruleType string) bool {
	for _, rule := range rules {
		if rule.Type == ruleType {
			return true
		}
	}
	return false
}

func (e *Engine) resetRuleSetRuntime(ruleSetID uint64) {
	delete(e.concurrency, ruleSetID)
	for _, ruleType := range []string{RuleTypeFailureRateCircuit, RuleTypeSlowCallCircuit} {
		delete(e.circuits, circuitKey(ruleSetID, ruleType))
	}
	prefix := fmt.Sprintf("%d:", ruleSetID)
	for key := range e.rates {
		if strings.HasPrefix(key, prefix) {
			delete(e.rates, key)
		}
	}
}

func (e *Engine) matchRuleSet(resourceName string, request RequestContext) (PublishedRuleSet, bool) {
	for _, ruleSet := range e.ruleSets {
		var candidate string
		switch ruleSet.Scope {
		case ScopeCustom:
			candidate = strings.TrimSpace(request.CustomResource)
			if candidate == "" {
				candidate = resourceName
			}
		case ScopeEndpoint:
			candidate = strings.TrimSpace(request.Endpoint)
		case ScopeService:
			candidate = strings.TrimSpace(request.Service)
		case ScopeGlobal:
			candidate = "*"
		}
		if matchesPattern(ruleSet.ResourcePattern, candidate) {
			return ruleSet, true
		}
	}
	return PublishedRuleSet{}, false
}

func (e *Engine) circuit(key string) *circuitRuntime {
	state := e.circuits[key]
	if state == nil {
		state = &circuitRuntime{state: CircuitClosed}
		e.circuits[key] = state
	}
	return state
}

func (e *Engine) releaseHalfOpen(keys []string) {
	for _, key := range keys {
		if state := e.circuits[key]; state != nil && state.halfOpenInFlight > 0 {
			state.halfOpenInFlight--
		}
	}
}

func ruleSetSpecificity(ruleSet PublishedRuleSet) int {
	base := 0
	switch ruleSet.Scope {
	case ScopeCustom:
		base = 400
	case ScopeEndpoint:
		base = 300
	case ScopeService:
		base = 200
	case ScopeGlobal:
		base = 100
	}
	if !strings.HasSuffix(ruleSet.ResourcePattern, "*") {
		base += 10
	}
	return base
}

func matchesPattern(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	if value == "" {
		return false
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}
	return value == pattern
}

func circuitKey(ruleSetID uint64, ruleType string) string {
	return fmt.Sprintf("%d:%s", ruleSetID, ruleType)
}

func openCircuit(state *circuitRuntime, now time.Time) {
	state.state = CircuitOpen
	state.openedAt = now
	state.halfOpenInFlight = 0
	state.halfOpenSuccesses = 0
}

func closeCircuit(state *circuitRuntime) {
	state.state = CircuitClosed
	state.openedAt = time.Time{}
	state.samples = nil
	state.halfOpenInFlight = 0
	state.halfOpenSuccesses = 0
}
