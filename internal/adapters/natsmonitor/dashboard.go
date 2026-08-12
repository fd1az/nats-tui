package natsmonitor

import (
	"context"

	"github.com/fd1az/nats-tui/internal/app/dashboard"
)

func aggregateDashboardRuntime(responses []statsResponse) dashboard.Runtime {
	result := dashboard.Runtime{Available: len(responses) > 0, Servers: len(responses)}
	var cpu float64
	for _, response := range responses {
		stats := response.Stats
		cpu += stats.CPU
		if stats.Mem > 0 {
			result.MemoryBytes += uint64(stats.Mem)
		}
		result.Cores += stats.Cores
		result.MaxProcs += stats.MaxProcs
		if stats.Goroutines > 0 {
			result.GoroutinesAvailable = true
			result.Goroutines += stats.Goroutines
		}
		if result.StartedAt.IsZero() || (!stats.Start.IsZero() && stats.Start.Before(result.StartedAt)) {
			result.StartedAt = stats.Start
		}
		result.Connections += stats.Connections
		result.Subscriptions += stats.Subscriptions
		result.SlowConsumers += stats.SlowConsumers
		result.StaleConnections += stats.StaleConnections
		result.StalledClients += stats.StalledClients
		result.InMessages += stats.Received.Messages
		result.OutMessages += stats.Sent.Messages
		result.InBytes += stats.Received.Bytes
		result.OutBytes += stats.Sent.Bytes
	}
	result.CPUPercent = cpu / float64(len(responses))
	return result
}

func (h *httpTopology) DashboardRuntime(ctx context.Context) (dashboard.Runtime, error) {
	var value varzWire
	if err := h.get(ctx, "/varz", &value); err != nil {
		return dashboard.Runtime{}, err
	}
	goroutines := value.Goroutines
	if goroutines == 0 {
		goroutines = value.NumGoroutines
	}
	return dashboard.Runtime{
		Available: true, Servers: 1, CPUPercent: value.CPU, MemoryBytes: positiveUint64(value.Mem),
		Cores: value.Cores, MaxProcs: value.MaxProcs, Goroutines: goroutines, GoroutinesAvailable: goroutines > 0,
		StartedAt: value.Start, Connections: value.Connections, Subscriptions: value.Subscriptions,
		SlowConsumers: value.SlowConsumers, StaleConnections: value.StaleConnections, StalledClients: value.StalledClients,
		InMessages: value.InMessages, OutMessages: value.OutMessages, InBytes: value.InBytes, OutBytes: value.OutBytes,
	}, nil
}

func positiveUint64(value int64) uint64 {
	if value > 0 {
		return uint64(value)
	}
	return 0
}
