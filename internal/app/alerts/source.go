package alerts

import (
	"context"

	domain "github.com/clcsolutions/nats-tui/internal/domain/alerts"
)

// AdvisorySource is the driven port for the external NATS event stream.
type AdvisorySource interface {
	WatchAlerts(context.Context) (<-chan *domain.Alert, <-chan error, error)
}

// HealthSource observes infrastructure state. Threshold decisions remain in
// the domain rules rather than leaking into the adapter.
type HealthSource interface {
	HealthObservations(context.Context) ([]domain.HealthObservation, error)
}
