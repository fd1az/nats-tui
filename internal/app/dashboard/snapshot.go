package dashboard

import (
	"context"
	"time"
)

type Snapshot struct {
	ServerName string
	Cluster    string
	Version    string
	Streams    int
	Consumers  int
	Messages   uint64
	Bytes      uint64
	ObservedAt time.Time
}

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
