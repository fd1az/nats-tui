package natsmonitor

import (
	"context"
	"testing"
	"time"
)

func TestAggregateDashboardRuntimeCombinesServers(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	got := aggregateDashboardRuntime([]statsResponse{
		{Stats: serverStatsWire{Start: start, CPU: 20, Mem: 100, Connections: 3, SlowConsumers: 1, Sent: dataStatsWire{Messages: 10, Bytes: 100}}},
		{Stats: serverStatsWire{Start: start.Add(time.Minute), CPU: 40, Mem: 200, Connections: 4, StaleConnections: 2, Received: dataStatsWire{Messages: 20, Bytes: 300}}},
	})
	if got.Servers != 2 || got.CPUPercent != 30 || got.MemoryBytes != 300 || got.Connections != 7 {
		t.Fatalf("runtime = %#v", got)
	}
	if got.StartedAt != start || got.InMessages != 20 || got.OutMessages != 10 || got.SlowConsumers != 1 || got.StaleConnections != 2 {
		t.Fatalf("runtime aggregate = %#v", got)
	}
}

func TestHTTPDashboardRuntimeMapsOptionalMetrics(t *testing.T) {
	t.Parallel()
	source, err := newHTTPTopology("http://127.0.0.1:8222", monitoringDoer{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := source.DashboardRuntime(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Available || !got.GoroutinesAvailable || got.Goroutines != 42 || got.InMessages != 100 || got.OutBytes != 500 {
		t.Fatalf("runtime = %#v", got)
	}
}
