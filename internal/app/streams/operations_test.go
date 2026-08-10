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

type maintenanceStub struct {
	purge   PurgeCommand
	deleted MessageDeleteCommand
}

func (s *maintenanceStub) PurgeStream(_ context.Context, command PurgeCommand) error {
	s.purge = command
	return nil
}
func (s *maintenanceStub) DeleteStreamMessage(_ context.Context, command MessageDeleteCommand) error {
	s.deleted = command
	return nil
}

func TestStreamDefinitionValidation(t *testing.T) {
	t.Parallel()

	valid := Definition{Name: "ORDERS", Subjects: []string{"orders.>"}, Storage: StorageFile, Retention: RetentionLimits, MaxMessages: -1, MaxBytes: -1, Replicas: 1}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid definition error = %v", err)
	}
	sourced := valid
	sourced.Name, sourced.Mirror, sourced.Sources = "AGGREGATE", nil, []Source{{Name: "ORDERS"}}
	if err := sourced.Validate(); err != nil {
		t.Fatalf("source-only stream: %v", err)
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

func TestStreamDefinitionValidatesTopologyAndAdvancedLimits(t *testing.T) {
	t.Parallel()
	valid := Definition{Name: "MIRROR", Storage: StorageFile, Retention: RetentionLimits, MaxMessages: -1, MaxBytes: -1, MaxMessagesPerSubject: -1, MaxMessageSize: -1, MaxConsumers: -1, Replicas: 1, Mirror: &Source{Name: "ORDERS", StartSequence: 10}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid mirror: %v", err)
	}
	invalid := valid
	invalid.Subjects = []string{"orders.>"}
	if err := invalid.Validate(); err == nil {
		t.Fatal("mirror with subjects was accepted")
	}
	invalid = valid
	invalid.Sources = []Source{{Name: "EVENTS"}}
	if err := invalid.Validate(); err == nil {
		t.Fatal("mirror with sources was accepted")
	}
	invalid = Definition{Name: "ORDERS", Subjects: []string{"orders.>"}, Storage: StorageFile, Retention: RetentionLimits, MaxMessages: -1, MaxBytes: -1, Replicas: 1, Discard: DiscardNew, DiscardNewPerSubject: true, MaxMessagesPerSubject: -1}
	if err := invalid.Validate(); err == nil {
		t.Fatal("discard-new-per-subject without limit was accepted")
	}
}

func TestStreamMaintenanceValidatesBeforeDelegating(t *testing.T) {
	t.Parallel()
	maintenance := new(maintenanceStub)
	handler := NewMaintenanceHandler(maintenance)
	if err := handler.Purge(context.Background(), PurgeCommand{Stream: "ORDERS", Keep: 10}); err != nil {
		t.Fatal(err)
	}
	if maintenance.purge.Keep != 10 {
		t.Fatalf("purge=%+v", maintenance.purge)
	}
	if err := handler.Purge(context.Background(), PurgeCommand{Stream: "ORDERS", Keep: 1, Sequence: 2}); err == nil {
		t.Fatal("incompatible purge options were accepted")
	}
	if err := handler.DeleteMessage(context.Background(), MessageDeleteCommand{Stream: "ORDERS", Sequence: 42, Secure: true}); err != nil {
		t.Fatal(err)
	}
	if maintenance.deleted.Sequence != 42 || !maintenance.deleted.Secure {
		t.Fatalf("deleted=%+v", maintenance.deleted)
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
	definition.Sealed = true
	if err := NewOperateHandler(operator).Handle(context.Background(), Command{Action: ActionCreate, Name: "ORDERS", Definition: definition}); err == nil {
		t.Fatal("sealed stream creation was accepted")
	}
}
