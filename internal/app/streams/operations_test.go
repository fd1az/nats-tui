package streams

import (
	"context"
	"testing"
)

type operatorStub struct{ command Command }

func (s *operatorStub) OperateStream(_ context.Context, command Command) error {
	s.command = command
	return nil
}

func TestStreamDefinitionValidation(t *testing.T) {
	t.Parallel()

	valid := Definition{Name: "ORDERS", Subjects: []string{"orders.>"}, Storage: StorageFile, Retention: RetentionLimits, MaxMessages: -1, MaxBytes: -1, Replicas: 1}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid definition error = %v", err)
	}
	invalid := valid
	invalid.Subjects = nil
	if err := invalid.Validate(); err == nil {
		t.Fatal("definition without subjects was accepted")
	}
	invalid = valid
	invalid.Replicas = 6
	if err := invalid.Validate(); err == nil {
		t.Fatal("definition with six replicas was accepted")
	}
}

func TestOperateStreamValidatesBeforeDelegating(t *testing.T) {
	t.Parallel()

	operator := &operatorStub{}
	definition := Definition{Name: "ORDERS", Subjects: []string{"orders.>"}, Storage: StorageMemory, Retention: RetentionLimits, MaxMessages: 100, MaxBytes: -1, Replicas: 1}
	command := Command{Action: ActionCreate, Name: "ORDERS", Definition: definition}
	if err := NewOperateHandler(operator).Handle(context.Background(), command); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if operator.command.Name != "ORDERS" {
		t.Fatalf("delegated command = %#v", operator.command)
	}
}
