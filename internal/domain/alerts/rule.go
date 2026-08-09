package alerts

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrRuleIDRequired        = errors.New("alert rule ID is required")
	ErrRuleNameRequired      = errors.New("alert rule name is required")
	ErrInvalidSubjectPattern = errors.New("invalid NATS subject pattern")
)

type RuleID struct{ value string }

func NewRuleID(value string) (RuleID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return RuleID{}, ErrRuleIDRequired
	}
	return RuleID{value}, nil
}
func (id RuleID) String() string { return id.value }

type RuleKind struct{ value string }

var RuleEvent = RuleKind{"event"}
var RuleHealth = RuleKind{"health"}

func (k RuleKind) String() string { return k.value }

type Metric struct{ value string }

var (
	MetricSlowConsumers = Metric{"slow-consumers"}
	MetricPendingBytes  = Metric{"pending-bytes"}
	MetricServerCPU     = Metric{"server-cpu"}
	MetricServerMemory  = Metric{"server-memory"}
	MetricLinkDown      = Metric{"link-down"}
)

func ParseMetric(value string) (Metric, error) {
	for _, metric := range []Metric{MetricSlowConsumers, MetricPendingBytes, MetricServerCPU, MetricServerMemory, MetricLinkDown} {
		if strings.EqualFold(strings.TrimSpace(value), metric.value) {
			return metric, nil
		}
	}
	return Metric{}, fmt.Errorf("unknown health metric %q", value)
}
func (m Metric) String() string { return m.value }
func (m Metric) IsZero() bool   { return m.value == "" }

type Rule struct {
	id              RuleID
	name            string
	kind            RuleKind
	pattern         string
	metric          Metric
	threshold       float64
	severity        Severity
	cooldown        time.Duration
	enabled         bool
	lastTriggeredAt time.Time
}

func NewEventRule(id, name, pattern string, severity Severity, cooldown time.Duration, enabled bool) (*Rule, error) {
	return rehydrateRule(id, name, RuleEvent, pattern, Metric{}, 0, severity, cooldown, enabled, time.Time{})
}

func NewHealthRule(id, name string, metric Metric, threshold float64, severity Severity, cooldown time.Duration, enabled bool) (*Rule, error) {
	return rehydrateRule(id, name, RuleHealth, "", metric, threshold, severity, cooldown, enabled, time.Time{})
}

func RehydrateRule(id, name string, kind RuleKind, pattern string, metric Metric, threshold float64, severity Severity, cooldown time.Duration, enabled bool, lastTriggeredAt time.Time) (*Rule, error) {
	return rehydrateRule(id, name, kind, pattern, metric, threshold, severity, cooldown, enabled, lastTriggeredAt)
}

func rehydrateRule(idValue, name string, kind RuleKind, pattern string, metric Metric, threshold float64, severity Severity, cooldown time.Duration, enabled bool, lastTriggeredAt time.Time) (*Rule, error) {
	id, err := NewRuleID(idValue)
	if err != nil {
		return nil, err
	}
	name, pattern = strings.TrimSpace(name), strings.TrimSpace(pattern)
	if name == "" {
		return nil, ErrRuleNameRequired
	}
	if severity.IsZero() {
		return nil, errors.New("rule severity is required")
	}
	if cooldown < 0 {
		return nil, errors.New("rule cooldown cannot be negative")
	}
	switch kind {
	case RuleEvent:
		if !validSubjectPattern(pattern) {
			return nil, ErrInvalidSubjectPattern
		}
	case RuleHealth:
		if metric.IsZero() || threshold < 0 {
			return nil, errors.New("health rule metric and non-negative threshold are required")
		}
	default:
		return nil, errors.New("rule kind is required")
	}
	return &Rule{id: id, name: name, kind: kind, pattern: pattern, metric: metric, threshold: threshold, severity: severity, cooldown: cooldown, enabled: enabled, lastTriggeredAt: lastTriggeredAt}, nil
}

func (r *Rule) EvaluateEvent(subject string, at time.Time) (Severity, bool) {
	if !r.MatchesEvent(subject) || r.coolingDown(at) {
		return Severity{}, false
	}
	r.lastTriggeredAt = at
	return r.severity, true
}

func (r Rule) MatchesEvent(subject string) bool {
	return r.enabled && r.kind == RuleEvent && subjectMatches(r.pattern, subject)
}

func (r *Rule) EvaluateHealth(observation HealthObservation, at time.Time) (Severity, bool) {
	if !r.MatchesHealth(observation) || r.coolingDown(at) {
		return Severity{}, false
	}
	r.lastTriggeredAt = at
	return r.severity, true
}

func (r Rule) MatchesHealth(observation HealthObservation) bool {
	return r.enabled && r.kind == RuleHealth && r.metric == observation.metric && observation.value >= r.threshold
}

func (r Rule) coolingDown(at time.Time) bool {
	return !r.lastTriggeredAt.IsZero() && at.Before(r.lastTriggeredAt.Add(r.cooldown))
}

func (r *Rule) SetEnabled(enabled bool)   { r.enabled = enabled }
func (r Rule) ID() RuleID                 { return r.id }
func (r Rule) Name() string               { return r.name }
func (r Rule) Kind() RuleKind             { return r.kind }
func (r Rule) Pattern() string            { return r.pattern }
func (r Rule) Metric() Metric             { return r.metric }
func (r Rule) Threshold() float64         { return r.threshold }
func (r Rule) Severity() Severity         { return r.severity }
func (r Rule) Cooldown() time.Duration    { return r.cooldown }
func (r Rule) Enabled() bool              { return r.enabled }
func (r Rule) LastTriggeredAt() time.Time { return r.lastTriggeredAt }

type HealthObservation struct {
	metric             Metric
	value              float64
	resource           Resource
	title, description string
	observedAt         time.Time
}

func NewHealthObservation(metric Metric, value float64, resource Resource, title, description string, observedAt time.Time) (HealthObservation, error) {
	title, description = strings.TrimSpace(title), strings.TrimSpace(description)
	if metric.IsZero() || value < 0 || resource.IsZero() || title == "" || observedAt.IsZero() {
		return HealthObservation{}, errors.New("health observation is incomplete")
	}
	return HealthObservation{metric: metric, value: value, resource: resource, title: title, description: description, observedAt: observedAt}, nil
}
func (o HealthObservation) Metric() Metric        { return o.metric }
func (o HealthObservation) Value() float64        { return o.value }
func (o HealthObservation) Resource() Resource    { return o.resource }
func (o HealthObservation) Title() string         { return o.title }
func (o HealthObservation) Description() string   { return o.description }
func (o HealthObservation) ObservedAt() time.Time { return o.observedAt }

func validSubjectPattern(pattern string) bool {
	if pattern == "" || strings.ContainsAny(pattern, " \t\r\n") || strings.Contains(pattern, "..") {
		return false
	}
	tokens := strings.Split(pattern, ".")
	for index, token := range tokens {
		if token == "" {
			return false
		}
		if token == ">" && index != len(tokens)-1 {
			return false
		}
		if strings.Contains(token, ">") && token != ">" {
			return false
		}
		if strings.Contains(token, "*") && token != "*" {
			return false
		}
	}
	return true
}

func subjectMatches(pattern, subject string) bool {
	if !validSubjectPattern(pattern) || !validSubjectPattern(subject) {
		return false
	}
	patterns, subjects := strings.Split(pattern, "."), strings.Split(subject, ".")
	for index, token := range patterns {
		if token == ">" {
			return true
		}
		if index >= len(subjects) || (token != "*" && token != subjects[index]) {
			return false
		}
	}
	return len(patterns) == len(subjects)
}
