package kvstore

import (
	"context"
	"testing"
)

type entryRemoverStub struct{ command RemoveEntry }

func (s *entryRemoverStub) RemoveKeyValueEntry(_ context.Context, command RemoveEntry) error {
	s.command = command
	return nil
}

func TestRemoveEntryHandlerPreservesCASAndPurgeIntent(t *testing.T) {
	t.Parallel()

	revision := uint64(9)
	remover := &entryRemoverStub{}
	err := NewRemoveEntryHandler(remover).Handle(context.Background(), RemoveEntry{
		Bucket: " CONFIG ", Key: " feature.api ", Action: RemovePurge, ExpectedRevision: &revision,
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if remover.command.Bucket != "CONFIG" || remover.command.Key != "feature.api" || remover.command.Action != RemovePurge || remover.command.ExpectedRevision == nil || *remover.command.ExpectedRevision != 9 {
		t.Fatalf("command = %#v", remover.command)
	}
}

func TestRemoveEntryHandlerRejectsUnknownAction(t *testing.T) {
	t.Parallel()

	err := NewRemoveEntryHandler(&entryRemoverStub{}).Handle(context.Background(), RemoveEntry{Bucket: "CONFIG", Key: "key", Action: "DROP"})
	if err == nil {
		t.Fatal("Handle() accepted unknown action")
	}
}
