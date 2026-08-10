package bootstrap

import (
	"context"

	"github.com/clcsolutions/nats-tui/internal/app/streams"
	"github.com/clcsolutions/nats-tui/internal/config"
)

func (r Runner) ListStreamCatalog(ctx context.Context, settings config.Settings) ([]streams.Summary, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return nil, err
	}
	defer monitor.Close()
	queryCtx, cancel := r.withTimeout(ctx, refreshTimeout)
	defer cancel()
	return streams.NewListStreamsHandler(monitor).Handle(queryCtx)
}

func (r Runner) OperateStreamConfig(ctx context.Context, settings config.Settings, command streams.Command) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return streams.NewOperateHandler(monitor).Handle(actionCtx, command)
}

func (r Runner) PurgeStreamMessages(ctx context.Context, settings config.Settings, command streams.PurgeCommand) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return streams.NewMaintenanceHandler(monitor).Purge(actionCtx, command)
}

func (r Runner) DeleteStoredStreamMessage(ctx context.Context, settings config.Settings, command streams.MessageDeleteCommand) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return streams.NewMaintenanceHandler(monitor).DeleteMessage(actionCtx, command)
}
