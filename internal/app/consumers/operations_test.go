package consumers

import (
	"context"
	"testing"
)

type operatorStub struct{ command Command }

func (s *operatorStub) OperateConsumer(_ context.Context, command Command) error {
	s.command = command
	return nil
}

func TestConsumerReferenceRequiresStreamAndName(t *testing.T) {
	t.Parallel()

	for _, input := range [][2]string{{"", "worker"}, {"ORDERS", ""}} {
		if _, err := NewRef(input[0], input[1]); err == nil {
			t.Fatalf("NewRef(%q, %q) error = nil", input[0], input[1])
		}
	}
}

func TestOperateHandlerDelegatesTypedCommand(t *testing.T) {
	t.Parallel()

	ref, err := NewRef("ORDERS", "worker")
	if err != nil {
		t.Fatalf("NewRef() error = %v", err)
	}
	operator := &operatorStub{}
	command := Command{Consumer: ref, Action: ActionPause}
	if err := NewOperateHandler(operator).Handle(context.Background(), command); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if operator.command.Consumer.Name() != "worker" || operator.command.Action != ActionPause {
		t.Fatalf("command = %#v", operator.command)
	}
}
