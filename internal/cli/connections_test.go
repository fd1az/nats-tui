package cli

import (
	"bytes"
	"context"
	app "github.com/clcsolutions/nats-tui/internal/app/connections"
	"github.com/clcsolutions/nats-tui/internal/config"
	"testing"
)

type stubConnections struct {
	snapshot app.Snapshot
	server   string
	cid      uint64
}

func (s *stubConnections) ListNATSConnections(context.Context, config.Settings) (app.Snapshot, error) {
	return s.snapshot, nil
}
func (s *stubConnections) CloseNATSConnection(_ context.Context, _ config.Settings, server string, cid uint64) error {
	s.server, s.cid = server, cid
	return nil
}
func TestConnectionsFiltersSlowConsumersAndConfirmsClose(t *testing.T) {
	t.Parallel()
	service := &stubConnections{snapshot: app.Snapshot{Connections: []app.Connection{{ServerID: "A", CID: 7, Name: "worker", User: "svc"}, {ServerID: "B", CID: 8, Name: "api", PendingBytes: 2 << 20}}}}
	output := new(bytes.Buffer)
	list := NewRoot(Dependencies{ConnectionReader: service})
	list.SetOut(output)
	list.SetArgs([]string{"connections", "--slow"})
	if err := list.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(output.Bytes(), []byte("worker")) || !bytes.Contains(output.Bytes(), []byte("api")) {
		t.Fatalf("output=%s", output)
	}
	unsafe := NewRoot(Dependencies{ConnectionCloser: service})
	unsafe.SetArgs([]string{"connections", "close", "B", "8"})
	if err := unsafe.ExecuteContext(context.Background()); err == nil {
		t.Fatal("unsafe close succeeded")
	}
	safe := NewRoot(Dependencies{ConnectionCloser: service})
	safe.SetArgs([]string{"connections", "close", "B", "8", "--yes"})
	if err := safe.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.server != "B" || service.cid != 8 {
		t.Fatalf("target=%s:%d", service.server, service.cid)
	}
}
