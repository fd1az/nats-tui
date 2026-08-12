package bootstrap

import (
	"context"

	"github.com/fd1az/nats-tui/internal/app/dashboard"
	"github.com/fd1az/nats-tui/internal/config"
)

func (r Runner) dashboardPanels(settings config.Settings) dashboard.PanelService {
	return dashboard.NewPanelService(defaultDashboardRepository(settings.Profile))
}
func (r Runner) ListDashboardPanels(ctx context.Context, settings config.Settings) ([]dashboard.Panel, error) {
	return r.dashboardPanels(settings).List(ctx)
}
func (r Runner) SaveDashboardPanel(ctx context.Context, settings config.Settings, id, title string, metric dashboard.Metric) (dashboard.Panel, error) {
	return r.dashboardPanels(settings).Save(ctx, id, title, metric)
}
func (r Runner) DeleteDashboardPanel(ctx context.Context, settings config.Settings, id string) error {
	return r.dashboardPanels(settings).Delete(ctx, id)
}
