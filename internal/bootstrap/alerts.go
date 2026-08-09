package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/clcsolutions/nats-tui/internal/adapters/alertnotify"
	app "github.com/clcsolutions/nats-tui/internal/app/alerts"
	"github.com/clcsolutions/nats-tui/internal/config"
)

func (r Runner) alertStore(profile string) alertStore {
	if r.alertRepository != nil {
		return r.alertRepository
	}
	return defaultAlertRepository(profile)
}

func (r Runner) ListAlerts(ctx context.Context, settings config.Settings, query app.Query) ([]app.Record, error) {
	return app.NewListHandler(r.alertStore(settings.Profile)).Handle(ctx, query)
}

func (r Runner) AcknowledgeAlert(ctx context.Context, settings config.Settings, id string) error {
	return app.NewAcknowledgeHandler(r.alertStore(settings.Profile), r.clock).Handle(ctx, id)
}

func (r Runner) AcknowledgeAllAlerts(ctx context.Context, settings config.Settings) (int, error) {
	return app.NewAcknowledgeAllHandler(r.alertStore(settings.Profile), r.clock).Handle(ctx)
}

func (r Runner) ListAlertRules(ctx context.Context, settings config.Settings) ([]app.RuleView, error) {
	return app.NewListRulesHandler(r.alertStore(settings.Profile)).Handle(ctx)
}

func (r Runner) CreateAlertRule(ctx context.Context, settings config.Settings, command app.CreateRule) error {
	if command.ID == "" {
		command.ID = r.ids.Next()
	}
	return app.NewCreateRuleHandler(r.alertStore(settings.Profile)).Handle(ctx, command)
}

func (r Runner) SetAlertRuleEnabled(ctx context.Context, settings config.Settings, id string, enabled bool) error {
	return app.NewSetRuleEnabledHandler(r.alertStore(settings.Profile)).Handle(ctx, id, enabled)
}

func (r Runner) DeleteAlertRule(ctx context.Context, settings config.Settings, id string) error {
	return app.NewDeleteRuleHandler(r.alertStore(settings.Profile)).Handle(ctx, id)
}

func (r Runner) ScanAlerts(ctx context.Context, settings config.Settings) ([]app.IngestResult, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return nil, err
	}
	defer monitor.Close()
	store := r.alertStore(settings.Profile)
	if err := r.ensureDefaultAlertRules(ctx, store); err != nil {
		return nil, err
	}
	queryCtx, cancel := r.withTimeout(ctx, refreshTimeout)
	defer cancel()
	observations, err := monitor.HealthObservations(queryCtx)
	if err != nil {
		return nil, err
	}
	handler := app.NewEvaluateHealthHandler(store, store, app.NoopNotifier{}, r.ids, r.clock)
	return handler.Handle(ctx, observations)
}

func (r Runner) WatchAlerts(ctx context.Context, settings config.Settings) (<-chan app.Record, <-chan error, error) {
	return r.watchAlerts(ctx, settings, app.NoopNotifier{})
}

func (r Runner) WatchAlertsWithNotifications(ctx context.Context, settings config.Settings, webhook string, command []string) (<-chan app.Record, <-chan error, error) {
	notifiers := app.MultiNotifier{}
	if webhook != "" {
		notifier, err := alertnotify.NewWebhook(webhook, http.DefaultClient)
		if err != nil {
			return nil, nil, err
		}
		notifiers = append(notifiers, notifier)
	}
	if len(command) > 0 {
		notifier, err := alertnotify.NewCommand(command[0], command[1:], alertnotify.OSCommandRunner{})
		if err != nil {
			return nil, nil, err
		}
		notifiers = append(notifiers, notifier)
	}
	if len(notifiers) == 0 {
		notifiers = append(notifiers, app.NoopNotifier{})
	}
	return r.watchAlerts(ctx, settings, notifiers)
}

func (r Runner) watchAlerts(ctx context.Context, settings config.Settings, notifier app.Notifier) (<-chan app.Record, <-chan error, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return nil, nil, err
	}
	store := r.alertStore(settings.Profile)
	if err := r.ensureDefaultAlertRules(ctx, store); err != nil {
		monitor.Close()
		return nil, nil, err
	}
	incoming, sourceErrors, err := monitor.WatchAlerts(ctx)
	if err != nil {
		monitor.Close()
		return nil, nil, err
	}
	records, watchErrors := make(chan app.Record, 256), make(chan error, 32)
	go func() {
		defer monitor.Close()
		defer close(records)
		defer close(watchErrors)
		ingest := app.NewIngestHandler(store, store, notifier)
		for incoming != nil || sourceErrors != nil {
			select {
			case <-ctx.Done():
				return
			case alert, ok := <-incoming:
				if !ok {
					incoming = nil
					continue
				}
				result, ingestErr := ingest.Handle(ctx, alert)
				if ingestErr != nil {
					sendAlertError(ctx, watchErrors, ingestErr)
					continue
				}
				if result.Stored {
					select {
					case records <- result.Alert:
					case <-ctx.Done():
						return
					}
				}
			case sourceErr, ok := <-sourceErrors:
				if !ok {
					sourceErrors = nil
					continue
				}
				sendAlertError(ctx, watchErrors, sourceErr)
			}
		}
	}()
	return records, watchErrors, nil
}

func sendAlertError(ctx context.Context, target chan<- error, err error) {
	select {
	case target <- err:
	case <-ctx.Done():
	}
}

func (r Runner) ensureDefaultAlertRules(ctx context.Context, store alertStore) error {
	defaults := []app.CreateRule{
		{ID: "builtin-slow-consumers", Name: "Slow consumers", Metric: "slow-consumers", Threshold: 1, Severity: "warning", Cooldown: time.Minute, Enabled: true},
		{ID: "builtin-pending-bytes", Name: "Connection backpressure", Metric: "pending-bytes", Threshold: 1 << 20, Severity: "warning", Cooldown: time.Minute, Enabled: true},
		{ID: "builtin-server-cpu", Name: "High server CPU", Metric: "server-cpu", Threshold: 90, Severity: "critical", Cooldown: 5 * time.Minute, Enabled: true},
		{ID: "builtin-link-down", Name: "Cluster link down", Metric: "link-down", Threshold: 1, Severity: "critical", Cooldown: time.Minute, Enabled: true},
	}
	handler := app.NewCreateRuleHandler(store)
	for _, rule := range defaults {
		if err := handler.Handle(ctx, rule); err != nil && !errors.Is(err, app.ErrRuleAlreadyExists) {
			return fmt.Errorf("seed alert rule %q: %w", rule.Name, err)
		}
	}
	return nil
}
