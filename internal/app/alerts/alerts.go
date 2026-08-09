package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	domain "github.com/clcsolutions/nats-tui/internal/domain/alerts"
)

var (
	ErrAlertNotFound     = errors.New("alert not found")
	ErrRuleNotFound      = errors.New("alert rule not found")
	ErrRuleAlreadyExists = errors.New("alert rule already exists")
)

type Record struct {
	ID                string          `json:"id"`
	Kind              string          `json:"kind,omitempty"`
	Category          string          `json:"category"`
	Severity          string          `json:"severity"`
	Title             string          `json:"title"`
	Description       string          `json:"description,omitempty"`
	Source            string          `json:"source"`
	ResourceKind      string          `json:"resource_kind,omitempty"`
	ResourcePrimary   string          `json:"resource_primary,omitempty"`
	ResourceSecondary string          `json:"resource_secondary,omitempty"`
	OccurredAt        time.Time       `json:"occurred_at"`
	ReceivedAt        time.Time       `json:"received_at"`
	AcknowledgedAt    time.Time       `json:"acknowledged_at,omitempty"`
	Acknowledged      bool            `json:"acknowledged"`
	Payload           json.RawMessage `json:"payload,omitempty"`
}

func RecordFrom(alert *domain.Alert) Record {
	resource := alert.Resource()
	return Record{ID: alert.ID().String(), Kind: alert.Kind(), Category: alert.Category().String(), Severity: alert.Severity().String(), Title: alert.Title(), Description: alert.Description(), Source: alert.Source(), ResourceKind: resource.Kind().String(), ResourcePrimary: resource.Primary(), ResourceSecondary: resource.Secondary(), OccurredAt: alert.OccurredAt(), ReceivedAt: alert.ReceivedAt(), AcknowledgedAt: alert.AcknowledgedAt(), Acknowledged: alert.Acknowledged(), Payload: append(json.RawMessage(nil), alert.Payload()...)}
}

type Query struct {
	Search, Severity, Category string
	Unacknowledged             bool
	Limit                      int
}

func (q Query) apply(values []Record) []Record {
	search, severity, category := strings.ToLower(strings.TrimSpace(q.Search)), strings.ToLower(strings.TrimSpace(q.Severity)), strings.ToLower(strings.TrimSpace(q.Category))
	result := make([]Record, 0, len(values))
	for _, value := range values {
		if q.Unacknowledged && value.Acknowledged {
			continue
		}
		if severity != "" && value.Severity != severity {
			continue
		}
		if category != "" && value.Category != category {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{value.ID, value.Kind, value.Category, value.Severity, value.Title, value.Description, value.Source, value.ResourceKind, value.ResourcePrimary, value.ResourceSecondary, string(value.Payload)}, " "))
		if search != "" && !strings.Contains(haystack, search) {
			continue
		}
		value.Payload = append(json.RawMessage(nil), value.Payload...)
		result = append(result, value)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].OccurredAt.After(result[j].OccurredAt) })
	if q.Limit > 0 && len(result) > q.Limit {
		result = result[:q.Limit]
	}
	return result
}

type AlertRepository interface {
	Add(context.Context, *domain.Alert) (bool, error)
	Exists(context.Context, domain.ID) (bool, error)
	List(context.Context) ([]*domain.Alert, error)
	Update(context.Context, domain.ID, func(*domain.Alert) error) error
}

type RuleRepository interface {
	AddRule(context.Context, *domain.Rule) error
	ListRules(context.Context) ([]*domain.Rule, error)
	UpdateRule(context.Context, domain.RuleID, func(*domain.Rule) error) error
	RemoveRule(context.Context, domain.RuleID) error
}

type Notifier interface {
	Notify(context.Context, Record) error
}
type Clock interface{ Now() time.Time }
type IDGenerator interface{ Next() string }

type IngestResult struct {
	Alert              Record
	Stored, Suppressed bool
}

type NotificationError struct{ Err error }

func (e NotificationError) Error() string {
	return "alert stored but notification failed: " + e.Err.Error()
}
func (e NotificationError) Unwrap() error { return e.Err }

type IngestHandler struct {
	alerts   AlertRepository
	rules    RuleRepository
	notifier Notifier
}

func NewIngestHandler(alerts AlertRepository, rules RuleRepository, notifier Notifier) IngestHandler {
	if alerts == nil || rules == nil || notifier == nil {
		panic("alert ingestion dependencies cannot be nil")
	}
	return IngestHandler{alerts: alerts, rules: rules, notifier: notifier}
}

func (h IngestHandler) Handle(ctx context.Context, alert *domain.Alert) (IngestResult, error) {
	if alert == nil {
		return IngestResult{}, errors.New("alert is required")
	}
	exists, err := h.alerts.Exists(ctx, alert.ID())
	if err != nil {
		return IngestResult{}, err
	}
	if exists {
		return IngestResult{Alert: RecordFrom(alert)}, nil
	}
	matched, accepted, severity, err := h.applyEventRules(ctx, alert.Source(), alert.ReceivedAt(), alert.Severity())
	if err != nil {
		return IngestResult{}, err
	}
	if matched && !accepted {
		return IngestResult{Alert: RecordFrom(alert), Suppressed: true}, nil
	}
	alert.Classify(severity)
	stored, err := h.alerts.Add(ctx, alert)
	result := IngestResult{Alert: RecordFrom(alert), Stored: stored}
	if err != nil || !stored {
		return result, err
	}
	if err := h.notifier.Notify(ctx, result.Alert); err != nil {
		return result, NotificationError{Err: err}
	}
	return result, nil
}

func (h IngestHandler) applyEventRules(ctx context.Context, subject string, at time.Time, fallback domain.Severity) (bool, bool, domain.Severity, error) {
	rules, err := h.rules.ListRules(ctx)
	if err != nil {
		return false, false, domain.Severity{}, err
	}
	matched, accepted, selected := false, false, fallback
	for _, candidate := range rules {
		if !candidate.MatchesEvent(subject) {
			continue
		}
		matched = true
		id := candidate.ID()
		err := h.rules.UpdateRule(ctx, id, func(rule *domain.Rule) error {
			severity, ok := rule.EvaluateEvent(subject, at)
			if ok && severity.Rank() > selected.Rank() {
				selected = severity
			}
			accepted = accepted || ok
			return nil
		})
		if err != nil {
			return false, false, domain.Severity{}, err
		}
	}
	return matched, accepted, selected, nil
}

type ListHandler struct{ repository AlertRepository }

func NewListHandler(repository AlertRepository) ListHandler {
	if repository == nil {
		panic("nil alert repository")
	}
	return ListHandler{repository}
}
func (h ListHandler) Handle(ctx context.Context, query Query) ([]Record, error) {
	if err := validateQuery(query); err != nil {
		return nil, err
	}
	alerts, err := h.repository.List(ctx)
	if err != nil {
		return nil, err
	}
	values := make([]Record, 0, len(alerts))
	for _, alert := range alerts {
		values = append(values, RecordFrom(alert))
	}
	return query.apply(values), nil
}

type AcknowledgeHandler struct {
	repository AlertRepository
	clock      Clock
}

func NewAcknowledgeHandler(repository AlertRepository, clock Clock) AcknowledgeHandler {
	if repository == nil || clock == nil {
		panic("acknowledge dependencies cannot be nil")
	}
	return AcknowledgeHandler{repository, clock}
}
func (h AcknowledgeHandler) Handle(ctx context.Context, value string) error {
	id, err := domain.NewID(value)
	if err != nil {
		return err
	}
	return h.repository.Update(ctx, id, func(alert *domain.Alert) error {
		err := alert.Acknowledge(h.clock.Now())
		if errors.Is(err, domain.ErrAlreadyAcknowledged) {
			return nil
		}
		return err
	})
}

type AcknowledgeAllHandler struct {
	repository AlertRepository
	clock      Clock
}

func NewAcknowledgeAllHandler(repository AlertRepository, clock Clock) AcknowledgeAllHandler {
	if repository == nil || clock == nil {
		panic("acknowledge all dependencies cannot be nil")
	}
	return AcknowledgeAllHandler{repository, clock}
}
func (h AcknowledgeAllHandler) Handle(ctx context.Context) (int, error) {
	alerts, err := h.repository.List(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	at := h.clock.Now()
	for _, alert := range alerts {
		if alert.Acknowledged() {
			continue
		}
		if err := h.repository.Update(ctx, alert.ID(), func(current *domain.Alert) error { return current.Acknowledge(at) }); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

type EvaluateHealthHandler struct {
	alerts   AlertRepository
	rules    RuleRepository
	notifier Notifier
	ids      IDGenerator
	clock    Clock
}

func NewEvaluateHealthHandler(alerts AlertRepository, rules RuleRepository, notifier Notifier, ids IDGenerator, clock Clock) EvaluateHealthHandler {
	if alerts == nil || rules == nil || notifier == nil || ids == nil || clock == nil {
		panic("health alert dependencies cannot be nil")
	}
	return EvaluateHealthHandler{alerts, rules, notifier, ids, clock}
}
func (h EvaluateHealthHandler) Handle(ctx context.Context, observations []domain.HealthObservation) ([]IngestResult, error) {
	results := make([]IngestResult, 0)
	for _, observation := range observations {
		severities, err := h.evaluateObservation(ctx, observation)
		if err != nil {
			return results, err
		}
		if len(severities) == 0 {
			continue
		}
		severity := severities[0]
		for _, candidate := range severities[1:] {
			if candidate.Rank() > severity.Rank() {
				severity = candidate
			}
		}
		category := domain.CategoryServer
		if observation.Resource().Kind() == domain.ResourceConnection {
			category = domain.CategoryConnection
		}
		receivedAt := h.clock.Now()
		if receivedAt.Before(observation.ObservedAt()) {
			receivedAt = observation.ObservedAt()
		}
		alert, err := domain.NewAlert(domain.AlertInput{ID: h.ids.Next(), Kind: "health." + observation.Metric().String(), Category: category, Severity: severity, Title: observation.Title(), Description: observation.Description(), Source: "health." + observation.Metric().String(), Resource: observation.Resource(), OccurredAt: observation.ObservedAt(), ReceivedAt: receivedAt})
		if err != nil {
			return results, err
		}
		stored, err := h.alerts.Add(ctx, alert)
		result := IngestResult{Alert: RecordFrom(alert), Stored: stored}
		results = append(results, result)
		if err != nil {
			return results, err
		}
		if stored {
			if err := h.notifier.Notify(ctx, result.Alert); err != nil {
				return results, NotificationError{Err: err}
			}
		}
	}
	return results, nil
}
func (h EvaluateHealthHandler) evaluateObservation(ctx context.Context, observation domain.HealthObservation) ([]domain.Severity, error) {
	rules, err := h.rules.ListRules(ctx)
	if err != nil {
		return nil, err
	}
	var severities []domain.Severity
	for _, candidate := range rules {
		if !candidate.MatchesHealth(observation) {
			continue
		}
		id := candidate.ID()
		err := h.rules.UpdateRule(ctx, id, func(rule *domain.Rule) error {
			if severity, ok := rule.EvaluateHealth(observation, observation.ObservedAt()); ok {
				severities = append(severities, severity)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return severities, nil
}

type NoopNotifier struct{}

func (NoopNotifier) Notify(context.Context, Record) error { return nil }

func validateQuery(query Query) error {
	if query.Limit < 0 {
		return fmt.Errorf("alert limit cannot be negative")
	}
	if query.Severity != "" {
		if _, err := domain.ParseSeverity(query.Severity); err != nil {
			return err
		}
	}
	if query.Category != "" {
		if _, err := domain.ParseCategory(query.Category); err != nil {
			return err
		}
	}
	return nil
}
