package topology

import (
	"context"
	"errors"
	"testing"

	domain "github.com/clcsolutions/nats-tui/internal/domain/topology"
)

type stubSource struct {
	snapshot domain.Snapshot
	err      error
}

func (s stubSource) Topology(context.Context) (domain.Snapshot, error) { return s.snapshot, s.err }

type stubOperator struct {
	command Command
	err     error
}

func (s *stubOperator) OperateTopology(_ context.Context, command Command) error {
	s.command = command
	return s.err
}

func TestGetTopologyReturnsCollectorSnapshot(t *testing.T) {
	t.Parallel()
	want, _ := domain.NewSnapshot(nil, domain.MetaGroup{Leader: "nats-a"})
	got, err := NewGetHandler(stubSource{snapshot: want}).Handle(context.Background())
	if err != nil || got.Meta().Leader != "nats-a" {
		t.Fatalf("snapshot=%#v err=%v", got, err)
	}
}

func TestOperateTopologyValidatesTargetBeforeCallingAdapter(t *testing.T) {
	t.Parallel()
	operator := &stubOperator{}
	handler := NewOperateHandler(operator)
	if err := handler.Handle(context.Background(), Command{Action: ActionReload}); !errors.Is(err, ErrServerRequired) {
		t.Fatalf("error=%v", err)
	}
	if err := handler.Handle(context.Background(), Command{Action: ActionStepDownMeta}); err != nil {
		t.Fatal(err)
	}
	if operator.command.Action != ActionStepDownMeta {
		t.Fatalf("command=%#v", operator.command)
	}
}
