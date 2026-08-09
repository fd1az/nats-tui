package cli

import (
	"bytes"
	"context"
	"testing"
	"time"

	app "github.com/clcsolutions/nats-tui/internal/app/topology"
	"github.com/clcsolutions/nats-tui/internal/config"
	domain "github.com/clcsolutions/nats-tui/internal/domain/topology"
)

type stubTopology struct {
	snapshot domain.Snapshot
	command  app.Command
}

func (s *stubTopology) GetTopology(context.Context, config.Settings) (domain.Snapshot, error) {
	return s.snapshot, nil
}
func (s *stubTopology) OperateTopology(context.Context, config.Settings, app.Command) error {
	s.command = app.Command{}
	return nil
}

type recordingTopology struct{ stubTopology }

func (s *recordingTopology) OperateTopology(_ context.Context, _ config.Settings, command app.Command) error {
	s.command = command
	return nil
}

func TestTopologyListPrintsServerAndHealth(t *testing.T) {
	t.Parallel()
	link, _ := domain.NewLink(domain.LinkRoute, "A", "B", "nats-b", true, 0, time.Millisecond)
	server, _ := domain.NewServer(domain.ServerInput{ID: "A", Name: "nats-a", Version: "2.14.4", Cluster: "C1", Links: []domain.Link{link}})
	snapshot, _ := domain.NewSnapshot([]domain.Server{server}, domain.MetaGroup{Leader: "nats-a"})
	service := &stubTopology{snapshot: snapshot}
	output := new(bytes.Buffer)
	root := NewRoot(Dependencies{TopologyReader: service})
	root.SetOut(output)
	root.SetArgs([]string{"topology"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"nats-a", "2.14.4", "C1", "healthy", "META LEADER"} {
		if !bytes.Contains(output.Bytes(), []byte(want)) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestTopologyMutationsRequireConfirmation(t *testing.T) {
	t.Parallel()
	service := &recordingTopology{}
	unsafe := NewRoot(Dependencies{TopologyOperator: service})
	unsafe.SetArgs([]string{"topology", "reload", "A"})
	if err := unsafe.ExecuteContext(context.Background()); err == nil {
		t.Fatal("reload without --yes succeeded")
	}
	confirmed := NewRoot(Dependencies{TopologyOperator: service})
	confirmed.SetArgs([]string{"topology", "reload", "A", "--yes"})
	if err := confirmed.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.command.Action != app.ActionReload || service.command.ServerID != "A" {
		t.Fatalf("command=%#v", service.command)
	}
}
