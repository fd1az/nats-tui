package alerts

import (
	"testing"
	"time"

	domain "github.com/clcsolutions/nats-tui/internal/domain/alerts"
)

func TestManageRulesUsesConstructorsAndRepository(t *testing.T) {
	t.Parallel()
	repository := newMemoryRuleRepository()
	handler := NewCreateRuleHandler(repository)
	command := CreateRule{ID: "stream-actions", Name: "Stream actions", Pattern: "$JS.EVENT.ADVISORY.STREAM.*.>", Severity: "warning", Cooldown: time.Minute, Enabled: true}
	if err := handler.Handle(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	views, err := NewListRulesHandler(repository).Handle(t.Context())
	if err != nil || len(views) != 1 || views[0].ID != "stream-actions" || views[0].Severity != "warning" {
		t.Fatalf("views=%#v err=%v", views, err)
	}
	if err := NewSetRuleEnabledHandler(repository).Handle(t.Context(), "stream-actions", false); err != nil {
		t.Fatal(err)
	}
	if repository.rules["stream-actions"].Enabled() {
		t.Fatal("rule stayed enabled")
	}
	if err := NewDeleteRuleHandler(repository).Handle(t.Context(), "stream-actions"); err != nil {
		t.Fatal(err)
	}
	if len(repository.rules) != 0 {
		t.Fatalf("rules=%#v", repository.rules)
	}
}

func TestCreateHealthRuleParsesMetric(t *testing.T) {
	t.Parallel()
	repository := newMemoryRuleRepository()
	err := NewCreateRuleHandler(repository).Handle(t.Context(), CreateRule{ID: "cpu", Name: "CPU", Metric: domain.MetricServerCPU.String(), Threshold: 90, Severity: "critical", Cooldown: 5 * time.Minute, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if repository.rules["cpu"].Kind() != domain.RuleHealth {
		t.Fatalf("rule=%#v", repository.rules["cpu"])
	}
}
