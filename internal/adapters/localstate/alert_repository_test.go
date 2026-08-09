package localstate

import (
	"errors"
	"testing"
	"time"

	app "github.com/clcsolutions/nats-tui/internal/app/alerts"
	domain "github.com/clcsolutions/nats-tui/internal/domain/alerts"
)

func TestAlertRepositoryRoundTripsAlertsRulesAndTransitions(t *testing.T) {
	t.Parallel()
	file := &memoryStateFile{}
	repository := NewAlertRepository(file, 100)
	now := time.Date(2026, 8, 9, 17, 0, 0, 0, time.UTC)
	resource, _ := domain.NewResource(domain.ResourceConsumer, "ORDERS", "WORKER")
	alert, _ := domain.NewAlert(domain.AlertInput{ID: "evt-1", Kind: "max-deliver", Category: domain.CategoryConsumer, Severity: domain.SeverityWarning, Title: "Max deliveries", Description: "delivery exhausted", Source: "$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.ORDERS.WORKER", Resource: resource, OccurredAt: now, ReceivedAt: now.Add(time.Second), Payload: []byte(`{"stream_seq":42}`)})
	stored, err := repository.Add(t.Context(), alert)
	if err != nil || !stored {
		t.Fatalf("stored=%t err=%v", stored, err)
	}
	rule, _ := domain.NewEventRule("deliveries", "Delivery failures", "$JS.EVENT.ADVISORY.CONSUMER.>", domain.SeverityCritical, 5*time.Minute, true)
	if err := repository.AddRule(t.Context(), rule); err != nil {
		t.Fatal(err)
	}
	if err := repository.Update(t.Context(), alert.ID(), func(current *domain.Alert) error { return current.Acknowledge(now.Add(2 * time.Minute)) }); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateRule(t.Context(), rule.ID(), func(current *domain.Rule) error { _, _ = current.EvaluateEvent(alert.Source(), now); return nil }); err != nil {
		t.Fatal(err)
	}

	reloaded := NewAlertRepository(file, 100)
	alerts, err := reloaded.List(t.Context())
	if err != nil || len(alerts) != 1 || !alerts[0].Acknowledged() || string(alerts[0].Payload()) != `{"stream_seq":42}` {
		t.Fatalf("alerts=%#v err=%v", alerts, err)
	}
	rules, err := reloaded.ListRules(t.Context())
	if err != nil || len(rules) != 1 || !rules[0].LastTriggeredAt().Equal(now) {
		t.Fatalf("rules=%#v err=%v", rules, err)
	}
}

func TestAlertRepositoryDeduplicatesCapsAndMapsNotFound(t *testing.T) {
	t.Parallel()
	repository := NewAlertRepository(&memoryStateFile{}, 2)
	now := time.Date(2026, 8, 9, 17, 0, 0, 0, time.UTC)
	for index, id := range []string{"one", "two", "three"} {
		alert, _ := domain.NewAlert(domain.AlertInput{ID: id, Kind: "kind", Category: domain.CategoryAudit, Severity: domain.SeverityInfo, Title: id, Source: "$JS.EVENT.ADVISORY.API", OccurredAt: now.Add(time.Duration(index) * time.Second), ReceivedAt: now.Add(time.Duration(index) * time.Second)})
		if stored, err := repository.Add(t.Context(), alert); err != nil || !stored {
			t.Fatalf("id=%s stored=%t err=%v", id, stored, err)
		}
	}
	duplicate, _ := domain.NewAlert(domain.AlertInput{ID: "three", Kind: "kind", Category: domain.CategoryAudit, Severity: domain.SeverityInfo, Title: "duplicate", Source: "$JS.EVENT.ADVISORY.API", OccurredAt: now, ReceivedAt: now})
	if stored, err := repository.Add(t.Context(), duplicate); err != nil || stored {
		t.Fatalf("duplicate stored=%t err=%v", stored, err)
	}
	alerts, _ := repository.List(t.Context())
	if len(alerts) != 2 || alerts[0].ID().String() != "two" || alerts[1].ID().String() != "three" {
		t.Fatalf("alerts=%#v", alerts)
	}
	id, _ := domain.NewID("missing")
	if err := repository.Update(t.Context(), id, func(*domain.Alert) error { return nil }); !errors.Is(err, app.ErrAlertNotFound) {
		t.Fatalf("error=%v", err)
	}
	ruleID, _ := domain.NewRuleID("missing")
	if err := repository.RemoveRule(t.Context(), ruleID); !errors.Is(err, app.ErrRuleNotFound) {
		t.Fatalf("error=%v", err)
	}
}
