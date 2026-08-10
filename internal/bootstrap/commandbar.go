package bootstrap

import (
	"context"
	"time"

	app "github.com/clcsolutions/nats-tui/internal/app/commandbar"
	"github.com/clcsolutions/nats-tui/internal/config"
	domain "github.com/clcsolutions/nats-tui/internal/domain/commandbar"
)

func (r Runner) commandStore(profile string) app.Repository {
	if r.commandRepository != nil {
		return r.commandRepository
	}
	return defaultCommandRepository(profile)
}
func (r Runner) ListCustomCommands(ctx context.Context, settings config.Settings) ([]app.Entry, error) {
	return app.NewListDefinitionsHandler(r.commandStore(settings.Profile)).Handle(ctx)
}
func (r Runner) SaveCustomCommand(ctx context.Context, settings config.Settings, input domain.DefinitionInput) error {
	return app.NewSaveDefinitionHandler(r.commandStore(settings.Profile)).Handle(ctx, input)
}
func (r Runner) DeleteCustomCommand(ctx context.Context, settings config.Settings, id string) error {
	return app.NewRemoveDefinitionHandler(r.commandStore(settings.Profile)).Handle(ctx, id)
}
func (r Runner) ListCommandHistory(ctx context.Context, settings config.Settings, limit int) ([]app.HistoryView, error) {
	return app.NewListHistoryHandler(r.commandStore(settings.Profile)).Handle(ctx, limit)
}
func (r Runner) RecordCommandExecution(ctx context.Context, settings config.Settings, id, title string, started, completed time.Time, executionErr error) error {
	return app.NewRecordExecutionHandler(r.commandStore(settings.Profile), r.ids).Handle(ctx, id, title, started, completed, executionErr)
}
func (r Runner) PreviewCustomCommand(ctx context.Context, settings config.Settings, id string, values map[string]string) (app.PreviewView, error) {
	entries, err := r.ListCustomCommands(ctx, settings)
	if err != nil {
		return app.PreviewView{}, err
	}
	for _, entry := range entries {
		if entry.ID == id {
			return app.Preview(entry.Definition, values)
		}
	}
	return app.PreviewView{}, app.ErrDefinitionNotFound
}
