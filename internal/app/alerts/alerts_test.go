package alerts

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/fd1az/nats-tui/internal/domain/alerts"
)

type memoryAlertRepository struct{ alerts map[string]*domain.Alert }

func newMemoryAlertRepository() *memoryAlertRepository {
	return &memoryAlertRepository{alerts: map[string]*domain.Alert{}}
}
func (r *memoryAlertRepository) Add(_ context.Context, alert *domain.Alert) (bool, error) {
	id := alert.ID().String()
	if _, exists := r.alerts[id]; exists {
		return false, nil
	}
	r.alerts[id] = alert
	return true, nil
}
func (r *memoryAlertRepository) Exists(_ context.Context, id domain.ID) (bool, error) {
	_, exists := r.alerts[id.String()]
	return exists, nil
}
func (r *memoryAlertRepository) List(context.Context) ([]*domain.Alert, error) {
	result := make([]*domain.Alert, 0, len(r.alerts))
	for _, alert := range r.alerts {
		result = append(result, alert)
	}
	return result, nil
}
func (r *memoryAlertRepository) Update(_ context.Context, id domain.ID, update func(*domain.Alert) error) error {
	alert := r.alerts[id.String()]
	if alert == nil {
		return ErrAlertNotFound
	}
	return update(alert)
}

type memoryRuleRepository struct{ rules map[string]*domain.Rule }

func newMemoryRuleRepository(rules ...*domain.Rule) *memoryRuleRepository {
	result := &memoryRuleRepository{rules: map[string]*domain.Rule{}}
	for _, rule := range rules {
		result.rules[rule.ID().String()] = rule
	}
	return result
}
func (r *memoryRuleRepository) AddRule(_ context.Context, rule *domain.Rule) error {
	if _, exists := r.rules[rule.ID().String()]; exists {
		return ErrRuleAlreadyExists
	}
	r.rules[rule.ID().String()] = rule
	return nil
}
func (r *memoryRuleRepository) ListRules(context.Context) ([]*domain.Rule, error) {
	result := make([]*domain.Rule, 0, len(r.rules))
	for _, rule := range r.rules {
		result = append(result, rule)
	}
	return result, nil
}
func (r *memoryRuleRepository) UpdateRule(_ context.Context, id domain.RuleID, update func(*domain.Rule) error) error {
	rule := r.rules[id.String()]
	if rule == nil {
		return ErrRuleNotFound
	}
	return update(rule)
}
func (r *memoryRuleRepository) RemoveRule(_ context.Context, id domain.RuleID) error {
	if _, exists := r.rules[id.String()]; !exists {
		return ErrRuleNotFound
	}
	delete(r.rules, id.String())
	return nil
}

type recordingNotifier struct {
	ids []string
	err error
}

func (n *recordingNotifier) Notify(_ context.Context, alert Record) error {
	n.ids = append(n.ids, alert.ID)
	return n.err
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type sequenceIDs struct{ values []string }

func (s *sequenceIDs) Next() string { value := s.values[0]; s.values = s.values[1:]; return value }

func TestIngestAdvisoryDeduplicatesAndAppliesRuleCooldown(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC)
	rule, _ := domain.NewEventRule("consumer", "Consumer failures", "$JS.EVENT.ADVISORY.CONSUMER.>", domain.SeverityCritical, 5*time.Minute, true)
	alerts, rules, notifier := newMemoryAlertRepository(), newMemoryRuleRepository(rule), &recordingNotifier{}
	handler := NewIngestHandler(alerts, rules, notifier)
	first := newDomainAlert(t, "evt-1", now, "$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.ORDERS.WORKER")
	result, err := handler.Handle(t.Context(), first)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Stored || result.Alert.Severity != "critical" || len(notifier.ids) != 1 {
		t.Fatalf("result=%#v notifications=%v", result, notifier.ids)
	}
	duplicateEvent := newDomainAlert(t, "evt-1", now.Add(10*time.Minute), "$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.ORDERS.WORKER")
	duplicate, err := handler.Handle(t.Context(), duplicateEvent)
	if err != nil || duplicate.Stored || duplicate.Suppressed {
		t.Fatalf("duplicate=%#v err=%v", duplicate, err)
	}
	if !rules.rules["consumer"].LastTriggeredAt().Equal(now) {
		t.Fatalf("duplicate advanced cooldown to %s", rules.rules["consumer"].LastTriggeredAt())
	}
	second := newDomainAlert(t, "evt-2", now.Add(time.Minute), "$JS.EVENT.ADVISORY.CONSUMER.MSG_NAKED.ORDERS.WORKER")
	suppressed, err := handler.Handle(t.Context(), second)
	if err != nil || !suppressed.Suppressed || len(alerts.alerts) != 1 {
		t.Fatalf("suppressed=%#v err=%v alerts=%d", suppressed, err, len(alerts.alerts))
	}
}

func TestAcknowledgeUsesInjectedClockAndListFilters(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC)
	repository := newMemoryAlertRepository()
	alert := newDomainAlert(t, "evt-1", now, "$JS.EVENT.ADVISORY.STREAM.CREATED.ORDERS")
	_, _ = repository.Add(t.Context(), alert)
	if err := NewAcknowledgeHandler(repository, fixedClock{now.Add(time.Minute)}).Handle(t.Context(), "evt-1"); err != nil {
		t.Fatal(err)
	}
	records, err := NewListHandler(repository).Handle(t.Context(), Query{Unacknowledged: true})
	if err != nil || len(records) != 0 {
		t.Fatalf("records=%#v err=%v", records, err)
	}
	records, _ = NewListHandler(repository).Handle(t.Context(), Query{Search: "orders", Limit: 10})
	if len(records) != 1 || !records[0].Acknowledged {
		t.Fatalf("records=%#v", records)
	}
}

func TestEvaluateHealthCreatesAlertOnlyWhenRuleTriggers(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC)
	rule, _ := domain.NewHealthRule("slow", "Slow consumers", domain.MetricSlowConsumers, 1, domain.SeverityWarning, time.Minute, true)
	repository, rules := newMemoryAlertRepository(), newMemoryRuleRepository(rule)
	resource, _ := domain.NewResource(domain.ResourceServer, "N1", "")
	observation, _ := domain.NewHealthObservation(domain.MetricSlowConsumers, 2, resource, "Slow consumers", "N1 has slow consumers", now)
	handler := NewEvaluateHealthHandler(repository, rules, &recordingNotifier{}, &sequenceIDs{values: []string{"health-1"}}, fixedClock{now})
	results, err := handler.Handle(t.Context(), []domain.HealthObservation{observation})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].Stored || results[0].Alert.Category != "server" {
		t.Fatalf("results=%#v", results)
	}
}

func TestNotificationFailureDoesNotLoseStoredAlert(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repository := newMemoryAlertRepository()
	handler := NewIngestHandler(repository, newMemoryRuleRepository(), &recordingNotifier{err: errors.New("offline")})
	result, err := handler.Handle(t.Context(), newDomainAlert(t, "evt", now, "$JS.EVENT.ADVISORY.API"))
	if err == nil || !result.Stored || len(repository.alerts) != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func newDomainAlert(t *testing.T, id string, now time.Time, subject string) *domain.Alert {
	t.Helper()
	alert, err := domain.NewAlert(domain.AlertInput{ID: id, Kind: "kind", Category: domain.CategoryConsumer, Severity: domain.SeverityWarning, Title: "NATS event", Description: "orders", Source: subject, OccurredAt: now, ReceivedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	return alert
}
