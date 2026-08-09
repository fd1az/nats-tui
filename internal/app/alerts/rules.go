package alerts

import (
	"context"
	"sort"
	"strings"
	"time"

	domain "github.com/clcsolutions/nats-tui/internal/domain/alerts"
)

type CreateRule struct {
	ID, Name, Pattern, Metric, Severity string
	Threshold                           float64
	Cooldown                            time.Duration
	Enabled                             bool
}
type RuleView struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	Kind            string        `json:"kind"`
	Pattern         string        `json:"pattern,omitempty"`
	Metric          string        `json:"metric,omitempty"`
	Severity        string        `json:"severity"`
	Threshold       float64       `json:"threshold,omitempty"`
	Cooldown        time.Duration `json:"cooldown"`
	Enabled         bool          `json:"enabled"`
	LastTriggeredAt time.Time     `json:"last_triggered_at,omitempty"`
}

type CreateRuleHandler struct{ repository RuleRepository }

func NewCreateRuleHandler(repository RuleRepository) CreateRuleHandler {
	if repository == nil {
		panic("nil rule repository")
	}
	return CreateRuleHandler{repository}
}
func (h CreateRuleHandler) Handle(ctx context.Context, command CreateRule) error {
	severity, err := domain.ParseSeverity(command.Severity)
	if err != nil {
		return err
	}
	var rule *domain.Rule
	if strings.TrimSpace(command.Metric) != "" {
		metric, metricErr := domain.ParseMetric(command.Metric)
		if metricErr != nil {
			return metricErr
		}
		rule, err = domain.NewHealthRule(command.ID, command.Name, metric, command.Threshold, severity, command.Cooldown, command.Enabled)
	} else {
		rule, err = domain.NewEventRule(command.ID, command.Name, command.Pattern, severity, command.Cooldown, command.Enabled)
	}
	if err != nil {
		return err
	}
	return h.repository.AddRule(ctx, rule)
}

type ListRulesHandler struct{ repository RuleRepository }

func NewListRulesHandler(repository RuleRepository) ListRulesHandler {
	if repository == nil {
		panic("nil rule repository")
	}
	return ListRulesHandler{repository}
}
func (h ListRulesHandler) Handle(ctx context.Context) ([]RuleView, error) {
	rules, err := h.repository.ListRules(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]RuleView, 0, len(rules))
	for _, rule := range rules {
		result = append(result, ruleView(rule))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
func ruleView(rule *domain.Rule) RuleView {
	return RuleView{ID: rule.ID().String(), Name: rule.Name(), Kind: rule.Kind().String(), Pattern: rule.Pattern(), Metric: rule.Metric().String(), Threshold: rule.Threshold(), Severity: rule.Severity().String(), Cooldown: rule.Cooldown(), Enabled: rule.Enabled(), LastTriggeredAt: rule.LastTriggeredAt()}
}

type SetRuleEnabledHandler struct{ repository RuleRepository }

func NewSetRuleEnabledHandler(repository RuleRepository) SetRuleEnabledHandler {
	if repository == nil {
		panic("nil rule repository")
	}
	return SetRuleEnabledHandler{repository}
}
func (h SetRuleEnabledHandler) Handle(ctx context.Context, value string, enabled bool) error {
	id, err := domain.NewRuleID(value)
	if err != nil {
		return err
	}
	return h.repository.UpdateRule(ctx, id, func(rule *domain.Rule) error { rule.SetEnabled(enabled); return nil })
}

type DeleteRuleHandler struct{ repository RuleRepository }

func NewDeleteRuleHandler(repository RuleRepository) DeleteRuleHandler {
	if repository == nil {
		panic("nil rule repository")
	}
	return DeleteRuleHandler{repository}
}
func (h DeleteRuleHandler) Handle(ctx context.Context, value string) error {
	id, err := domain.NewRuleID(value)
	if err != nil {
		return err
	}
	return h.repository.RemoveRule(ctx, id)
}
