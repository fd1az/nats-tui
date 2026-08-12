package cli

import (
	"bytes"
	"context"
	"testing"

	app "github.com/fd1az/nats-tui/internal/app/alerts"
	"github.com/fd1az/nats-tui/internal/config"
)

type stubAlerts struct {
	query app.Query
	ackID string
	rule  app.CreateRule
}

func (s *stubAlerts) ListAlerts(_ context.Context, _ config.Settings, query app.Query) ([]app.Record, error) {
	s.query = query
	return []app.Record{{ID: "a-1", Severity: "critical", Category: "consumer", Title: "Max deliveries", ResourcePrimary: "ORDERS", ResourceSecondary: "WORKER"}}, nil
}
func (s *stubAlerts) AcknowledgeAlert(_ context.Context, _ config.Settings, id string) error {
	s.ackID = id
	return nil
}
func (s *stubAlerts) AcknowledgeAllAlerts(context.Context, config.Settings) (int, error) {
	return 2, nil
}
func (s *stubAlerts) ListAlertRules(context.Context, config.Settings) ([]app.RuleView, error) {
	return nil, nil
}
func (s *stubAlerts) CreateAlertRule(_ context.Context, _ config.Settings, rule app.CreateRule) error {
	s.rule = rule
	return nil
}
func (s *stubAlerts) SetAlertRuleEnabled(context.Context, config.Settings, string, bool) error {
	return nil
}
func (s *stubAlerts) DeleteAlertRule(context.Context, config.Settings, string) error { return nil }
func (s *stubAlerts) ScanAlerts(context.Context, config.Settings) ([]app.IngestResult, error) {
	return nil, nil
}
func (s *stubAlerts) WatchAlerts(context.Context, config.Settings) (<-chan app.Record, <-chan error, error) {
	records := make(chan app.Record)
	errs := make(chan error)
	close(records)
	close(errs)
	return records, errs, nil
}
func (s *stubAlerts) WatchAlertsWithNotifications(context.Context, config.Settings, string, []string) (<-chan app.Record, <-chan error, error) {
	return s.WatchAlerts(context.Background(), config.Settings{})
}

func TestAlertsListPassesFiltersAndRendersResource(t *testing.T) {
	t.Parallel()
	service, output := new(stubAlerts), new(bytes.Buffer)
	root := NewRoot(Dependencies{Alerts: service})
	root.SetOut(output)
	root.SetArgs([]string{"alerts", "list", "--severity", "critical", "--unacked", "--search", "orders"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.query.Severity != "critical" || !service.query.Unacknowledged || service.query.Search != "orders" {
		t.Fatalf("query=%+v", service.query)
	}
	if !bytes.Contains(output.Bytes(), []byte("ORDERS/WORKER")) {
		t.Fatalf("output=%s", output)
	}
}

func TestAlertRuleAddRequiresExactlyOneMatcher(t *testing.T) {
	t.Parallel()
	service := new(stubAlerts)
	invalid := NewRoot(Dependencies{Alerts: service})
	invalid.SetArgs([]string{"alerts", "rules", "add", "my-rule", "--severity", "warning"})
	if err := invalid.ExecuteContext(context.Background()); err == nil {
		t.Fatal("rule without matcher succeeded")
	}
	valid := NewRoot(Dependencies{Alerts: service})
	valid.SetArgs([]string{"alerts", "rules", "add", "my-rule", "--match", "$JS.EVENT.ADVISORY.CONSUMER.>", "--severity", "warning", "--cooldown", "1m"})
	if err := valid.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.rule.Name != "my-rule" || service.rule.Pattern == "" || !service.rule.Enabled {
		t.Fatalf("rule=%+v", service.rule)
	}
}
