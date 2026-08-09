package localstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"time"

	app "github.com/clcsolutions/nats-tui/internal/app/alerts"
	domain "github.com/clcsolutions/nats-tui/internal/domain/alerts"
)

type AlertRepository struct {
	mu        sync.Mutex
	file      StateFile
	maxAlerts int
}

func NewAlertRepository(file StateFile, maxAlerts int) *AlertRepository {
	if file == nil {
		panic("nil alert state file")
	}
	if maxAlerts < 1 {
		panic("alert history limit must be positive")
	}
	return &AlertRepository{file: file, maxAlerts: maxAlerts}
}

func (r *AlertRepository) Add(_ context.Context, alert *domain.Alert) (bool, error) {
	if alert == nil {
		return false, errors.New("alert is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return false, err
	}
	for _, stored := range state.Alerts {
		if stored.ID == alert.ID().String() {
			return false, nil
		}
	}
	state.Alerts = append(state.Alerts, alertToDTO(alert))
	if overflow := len(state.Alerts) - r.maxAlerts; overflow > 0 {
		state.Alerts = append([]alertDTO(nil), state.Alerts[overflow:]...)
	}
	return true, r.save(state)
}

func (r *AlertRepository) Exists(_ context.Context, id domain.ID) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return false, err
	}
	for _, stored := range state.Alerts {
		if stored.ID == id.String() {
			return true, nil
		}
	}
	return false, nil
}

func (r *AlertRepository) List(context.Context) ([]*domain.Alert, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return nil, err
	}
	result := make([]*domain.Alert, 0, len(state.Alerts))
	for _, stored := range state.Alerts {
		alert, mapErr := stored.alert()
		if mapErr != nil {
			return nil, mapErr
		}
		result = append(result, alert)
	}
	return result, nil
}

func (r *AlertRepository) Update(_ context.Context, id domain.ID, update func(*domain.Alert) error) error {
	if update == nil {
		panic("nil alert update")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return err
	}
	for index, stored := range state.Alerts {
		if stored.ID != id.String() {
			continue
		}
		alert, err := stored.alert()
		if err != nil {
			return err
		}
		if err := update(alert); err != nil {
			return err
		}
		state.Alerts[index] = alertToDTO(alert)
		return r.save(state)
	}
	return app.ErrAlertNotFound
}

func (r *AlertRepository) AddRule(_ context.Context, rule *domain.Rule) error {
	if rule == nil {
		return errors.New("alert rule is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return err
	}
	if _, exists := state.Rules[rule.ID().String()]; exists {
		return app.ErrRuleAlreadyExists
	}
	state.Rules[rule.ID().String()] = ruleToDTO(rule)
	return r.save(state)
}

func (r *AlertRepository) ListRules(context.Context) ([]*domain.Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return nil, err
	}
	result := make([]*domain.Rule, 0, len(state.Rules))
	for _, stored := range state.Rules {
		rule, mapErr := stored.rule()
		if mapErr != nil {
			return nil, mapErr
		}
		result = append(result, rule)
	}
	return result, nil
}

func (r *AlertRepository) UpdateRule(_ context.Context, id domain.RuleID, update func(*domain.Rule) error) error {
	if update == nil {
		panic("nil rule update")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return err
	}
	stored, exists := state.Rules[id.String()]
	if !exists {
		return app.ErrRuleNotFound
	}
	rule, err := stored.rule()
	if err != nil {
		return err
	}
	if err := update(rule); err != nil {
		return err
	}
	state.Rules[id.String()] = ruleToDTO(rule)
	return r.save(state)
}

func (r *AlertRepository) RemoveRule(_ context.Context, id domain.RuleID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return err
	}
	if _, exists := state.Rules[id.String()]; !exists {
		return app.ErrRuleNotFound
	}
	delete(state.Rules, id.String())
	return r.save(state)
}

type alertState struct {
	Alerts []alertDTO         `json:"alerts,omitempty"`
	Rules  map[string]ruleDTO `json:"rules,omitempty"`
}
type alertDTO struct {
	ID                string    `json:"id"`
	Kind              string    `json:"kind,omitempty"`
	Category          string    `json:"category"`
	Severity          string    `json:"severity"`
	Title             string    `json:"title"`
	Description       string    `json:"description,omitempty"`
	Source            string    `json:"source"`
	ResourceKind      string    `json:"resource_kind,omitempty"`
	ResourcePrimary   string    `json:"resource_primary,omitempty"`
	ResourceSecondary string    `json:"resource_secondary,omitempty"`
	OccurredAt        time.Time `json:"occurred_at"`
	ReceivedAt        time.Time `json:"received_at"`
	AcknowledgedAt    time.Time `json:"acknowledged_at,omitempty"`
	Payload           []byte    `json:"payload,omitempty"`
}
type ruleDTO struct {
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

func (r *AlertRepository) load() (alertState, error) {
	data, err := r.file.Read()
	if errors.Is(err, fs.ErrNotExist) {
		return alertState{Rules: map[string]ruleDTO{}}, nil
	}
	if err != nil {
		return alertState{}, fmt.Errorf("read alert state: %w", err)
	}
	var state alertState
	if err := json.Unmarshal(data, &state); err != nil {
		return alertState{}, fmt.Errorf("decode alert state: %w", err)
	}
	if state.Rules == nil {
		state.Rules = map[string]ruleDTO{}
	}
	return state, nil
}
func (r *AlertRepository) save(state alertState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode alert state: %w", err)
	}
	if err := r.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write alert state: %w", err)
	}
	return nil
}

func alertToDTO(alert *domain.Alert) alertDTO {
	resource := alert.Resource()
	return alertDTO{ID: alert.ID().String(), Kind: alert.Kind(), Category: alert.Category().String(), Severity: alert.Severity().String(), Title: alert.Title(), Description: alert.Description(), Source: alert.Source(), ResourceKind: resource.Kind().String(), ResourcePrimary: resource.Primary(), ResourceSecondary: resource.Secondary(), OccurredAt: alert.OccurredAt(), ReceivedAt: alert.ReceivedAt(), AcknowledgedAt: alert.AcknowledgedAt(), Payload: alert.Payload()}
}
func (d alertDTO) alert() (*domain.Alert, error) {
	category, err := domain.ParseCategory(d.Category)
	if err != nil {
		return nil, err
	}
	severity, err := domain.ParseSeverity(d.Severity)
	if err != nil {
		return nil, err
	}
	var resource domain.Resource
	if d.ResourceKind != "" {
		kind, kindErr := domain.ParseResourceKind(d.ResourceKind)
		if kindErr != nil {
			return nil, kindErr
		}
		resource, err = domain.NewResource(kind, d.ResourcePrimary, d.ResourceSecondary)
		if err != nil {
			return nil, err
		}
	}
	return domain.RehydrateAlert(domain.AlertInput{ID: d.ID, Kind: d.Kind, Category: category, Severity: severity, Title: d.Title, Description: d.Description, Source: d.Source, Resource: resource, OccurredAt: d.OccurredAt, ReceivedAt: d.ReceivedAt, Payload: d.Payload}, d.AcknowledgedAt)
}
func ruleToDTO(rule *domain.Rule) ruleDTO {
	return ruleDTO{ID: rule.ID().String(), Name: rule.Name(), Kind: rule.Kind().String(), Pattern: rule.Pattern(), Metric: rule.Metric().String(), Threshold: rule.Threshold(), Severity: rule.Severity().String(), Cooldown: rule.Cooldown(), Enabled: rule.Enabled(), LastTriggeredAt: rule.LastTriggeredAt()}
}
func (d ruleDTO) rule() (*domain.Rule, error) {
	severity, err := domain.ParseSeverity(d.Severity)
	if err != nil {
		return nil, err
	}
	if d.Kind == domain.RuleEvent.String() {
		return domain.RehydrateRule(d.ID, d.Name, domain.RuleEvent, d.Pattern, domain.Metric{}, d.Threshold, severity, d.Cooldown, d.Enabled, d.LastTriggeredAt)
	}
	metric, err := domain.ParseMetric(d.Metric)
	if err != nil {
		return nil, err
	}
	return domain.RehydrateRule(d.ID, d.Name, domain.RuleHealth, d.Pattern, metric, d.Threshold, severity, d.Cooldown, d.Enabled, d.LastTriggeredAt)
}
