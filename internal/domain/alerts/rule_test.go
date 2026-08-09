package alerts

import (
	"errors"
	"testing"
	"time"
)

func TestEventRuleMatchesNATSSubjectAndEnforcesCooldown(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 15, 0, 0, 0, time.UTC)
	rule, err := NewEventRule("consumer-delivery", "Consumer delivery failures", "$JS.EVENT.ADVISORY.CONSUMER.>", SeverityCritical, 5*time.Minute, true)
	if err != nil {
		t.Fatal(err)
	}
	severity, accepted := rule.EvaluateEvent("$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.ORDERS.WORKER", now)
	if !accepted || severity != SeverityCritical {
		t.Fatalf("severity=%s accepted=%t", severity, accepted)
	}
	if _, accepted := rule.EvaluateEvent("$JS.EVENT.ADVISORY.CONSUMER.MSG_NAKED.ORDERS.WORKER", now.Add(time.Minute)); accepted {
		t.Fatal("event inside cooldown was accepted")
	}
	if _, accepted := rule.EvaluateEvent("$JS.EVENT.ADVISORY.STREAM.CREATED.ORDERS", now.Add(time.Minute)); accepted {
		t.Fatal("unmatched event was accepted")
	}
	if _, accepted := rule.EvaluateEvent("$JS.EVENT.ADVISORY.CONSUMER.MSG_NAKED.ORDERS.WORKER", now.Add(5*time.Minute)); !accepted {
		t.Fatal("event after cooldown was suppressed")
	}
}

func TestHealthRuleTriggersAtThresholdForMatchingMetric(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 15, 0, 0, 0, time.UTC)
	rule, err := NewHealthRule("slow-consumers", "Slow consumers", MetricSlowConsumers, 1, SeverityWarning, time.Minute, true)
	if err != nil {
		t.Fatal(err)
	}
	resource, _ := NewResource(ResourceServer, "N1", "")
	observation, err := NewHealthObservation(MetricSlowConsumers, 2, resource, "Slow consumers", "server N1 has two slow consumers", now)
	if err != nil {
		t.Fatal(err)
	}
	severity, accepted := rule.EvaluateHealth(observation, now)
	if !accepted || severity != SeverityWarning {
		t.Fatalf("severity=%s accepted=%t", severity, accepted)
	}
	below, _ := NewHealthObservation(MetricSlowConsumers, 0, resource, "Healthy", "none", now.Add(time.Minute))
	if _, accepted := rule.EvaluateHealth(below, now.Add(time.Minute)); accepted {
		t.Fatal("value below threshold was accepted")
	}
}

func TestRuleRejectsInvalidDefinitions(t *testing.T) {
	t.Parallel()
	if _, err := NewEventRule("", "name", ">", SeverityInfo, 0, true); !errors.Is(err, ErrRuleIDRequired) {
		t.Fatalf("error=%v", err)
	}
	if _, err := NewEventRule("id", "name", "bad..subject", SeverityInfo, 0, true); !errors.Is(err, ErrInvalidSubjectPattern) {
		t.Fatalf("error=%v", err)
	}
	if _, err := NewHealthRule("id", "name", Metric{}, -1, SeverityInfo, 0, true); err == nil {
		t.Fatal("invalid health rule was accepted")
	}
}
