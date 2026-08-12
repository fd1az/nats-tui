package dashboard

import (
	"context"
	"time"
)

type Snapshot struct {
	ServerName  string
	Cluster     string
	Version     string
	Streams     int
	Consumers   int
	Messages    uint64
	Bytes       uint64
	MemoryBytes uint64
	FileBytes   uint64
	Limits      Limits
	Runtime     Runtime
	ObservedAt  time.Time
}

type Limits struct {
	MaxMemory    int64
	MaxFile      int64
	MaxStreams   int
	MaxConsumers int
}

type Runtime struct {
	Available           bool
	Servers             int
	CPUPercent          float64
	MemoryBytes         uint64
	Cores               int
	MaxProcs            int
	Goroutines          int
	GoroutinesAvailable bool
	StartedAt           time.Time
	Connections         int
	Subscriptions       int
	SlowConsumers       int64
	StaleConnections    int64
	StalledClients      int64
	InMessages          uint64
	OutMessages         uint64
	InBytes             uint64
	OutBytes            uint64
}

func (s Snapshot) TrafficMessages() uint64 { return s.Runtime.InMessages + s.Runtime.OutMessages }
func (s Snapshot) TrafficBytes() uint64    { return s.Runtime.InBytes + s.Runtime.OutBytes }

type Monitor interface {
	Snapshot(context.Context) (Snapshot, error)
}

type GetSnapshotHandler struct {
	monitor Monitor
}

func NewGetSnapshotHandler(monitor Monitor) GetSnapshotHandler {
	if monitor == nil {
		panic("nil dashboard monitor")
	}
	return GetSnapshotHandler{monitor: monitor}
}

func (h GetSnapshotHandler) Handle(ctx context.Context) (Snapshot, error) {
	return h.monitor.Snapshot(ctx)
}
