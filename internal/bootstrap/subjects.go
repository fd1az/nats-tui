package bootstrap

import (
	"context"

	"github.com/fd1az/nats-tui/internal/app/consumers"
	"github.com/fd1az/nats-tui/internal/app/operator"
	"github.com/fd1az/nats-tui/internal/app/streams"
	"github.com/fd1az/nats-tui/internal/app/subjects"
	"github.com/fd1az/nats-tui/internal/config"
)

// ExploreSubjects builds the same read model used by the TUI for non-
// interactive clients. Live rates are intentionally absent from this snapshot.
func (r Runner) ExploreSubjects(ctx context.Context, settings config.Settings) ([]subjects.Node, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return nil, err
	}
	defer monitor.Close()
	queryCtx, cancel := r.withTimeout(ctx, refreshTimeout)
	defer cancel()
	streamCatalog, err := streams.NewListStreamsHandler(monitor).Handle(queryCtx)
	if err != nil {
		return nil, err
	}
	consumerCatalog, err := consumers.NewListConsumersHandler(monitor).Handle(queryCtx)
	if err != nil {
		return nil, err
	}
	operatorCatalog, operatorErr := r.operatorCatalog(queryCtx, monitor, settings)
	if operatorErr != nil {
		operatorCatalog = operator.NewCatalog(operator.Capabilities{}, nil)
	}
	users := operatorCatalog.Users()
	permissions := make([]subjects.PrincipalPermissions, 0, len(users))
	for _, user := range users {
		principal := user.Name
		if principal == "" {
			principal = user.PublicKey
		}
		permissions = append(permissions, subjects.PrincipalPermissions{
			Principal: principal, Account: user.Account,
			PublishAllow: user.PublishAllow, PublishDeny: user.PublishDeny,
			SubscribeAllow: user.SubscribeAllow, SubscribeDeny: user.SubscribeDeny,
		})
	}
	return subjects.Enrich(streamCatalog, consumerCatalog, permissions, subjects.TrafficSnapshot{}), nil
}
