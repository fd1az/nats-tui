package bootstrap

import (
	"context"

	"github.com/clcsolutions/nats-tui/internal/app/consumers"
	"github.com/clcsolutions/nats-tui/internal/config"
)

func (r Runner) ListConsumerCatalog(ctx context.Context, settings config.Settings) ([]consumers.Summary, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return nil, err
	}
	defer monitor.Close()
	queryCtx, cancel := r.withTimeout(ctx, refreshTimeout)
	defer cancel()
	return consumers.NewListConsumersHandler(monitor).Handle(queryCtx)
}

func (r Runner) OperateConsumerConfig(ctx context.Context, settings config.Settings, command consumers.ConfigCommand) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, consumerActionTimeout)
	defer cancel()
	return consumers.NewConfigureHandler(monitor).Handle(actionCtx, command)
}

func (r Runner) OperateConsumer(ctx context.Context, settings config.Settings, command consumers.Command) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, consumerActionTimeout)
	defer cancel()
	return consumers.NewOperateHandler(monitor).Handle(actionCtx, command)
}
