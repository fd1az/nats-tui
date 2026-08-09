package kvstore

import (
	"context"
	"testing"
)

type entryWriterStub struct {
	command  WriteEntry
	revision uint64
}

func (s *entryWriterStub) PutKeyValueEntry(_ context.Context, command WriteEntry) (uint64, error) {
	s.command = command
	return s.revision, nil
}

func TestPutEntryHandlerCopiesValueAndPreservesExpectedRevision(t *testing.T) {
	t.Parallel()

	expected := uint64(12)
	value := []byte(`{"enabled":true}`)
	writer := &entryWriterStub{revision: 13}
	revision, err := NewPutEntryHandler(writer).Handle(context.Background(), WriteEntry{
		Bucket: " CONFIG ", Key: "feature.api", Value: value, ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	value[0] = 'x'
	if revision != 13 || writer.command.Bucket != "CONFIG" || writer.command.Key != "feature.api" || string(writer.command.Value) != `{"enabled":true}` || writer.command.ExpectedRevision == nil || *writer.command.ExpectedRevision != 12 {
		t.Fatalf("revision=%d command=%#v", revision, writer.command)
	}
}

func TestPutEntryHandlerRejectsMissingKey(t *testing.T) {
	t.Parallel()

	_, err := NewPutEntryHandler(&entryWriterStub{}).Handle(context.Background(), WriteEntry{Bucket: "CONFIG", Key: "  "})
	if err == nil {
		t.Fatal("Handle() error = nil")
	}
}
