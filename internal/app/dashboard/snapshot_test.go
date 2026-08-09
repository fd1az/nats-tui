package dashboard

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGetSnapshotReturnsMonitorSnapshot(t *testing.T) {
	t.Parallel()

	want := Snapshot{
		ServerName: "nats-local",
		Cluster:    "development",
		Version:    "2.12.0",
		Streams:    3,
		Consumers:  7,
		Messages:   42_310,
		Bytes:      8_388_608,
		ObservedAt: time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC),
	}
	handler := NewGetSnapshotHandler(stubMonitor{snapshot: want})

	got, err := handler.Handle(context.Background())

	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if got != want {
		t.Fatalf("Handle() = %#v, want %#v", got, want)
	}
}

func TestGetSnapshotPropagatesMonitorFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("connection lost")
	handler := NewGetSnapshotHandler(stubMonitor{err: wantErr})

	_, err := handler.Handle(context.Background())

	if !errors.Is(err, wantErr) {
		t.Fatalf("Handle() error = %v, want %v", err, wantErr)
	}
}

type stubMonitor struct {
	snapshot Snapshot
	err      error
}

func (m stubMonitor) Snapshot(context.Context) (Snapshot, error) {
	return m.snapshot, m.err
}
