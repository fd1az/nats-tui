package kvstore

import (
	"context"
	"testing"
)

func TestRestoreHandlerWritesHistoricalValueAgainstLatestRevision(t *testing.T) {
	t.Parallel()

	history := &historyReaderStub{entries: []Entry{
		{Revision: 7, Operation: OperationPut, Value: []byte(`{"enabled":true}`)},
		{Revision: 8, Operation: OperationDelete},
	}}
	writer := &entryWriterStub{revision: 9}
	result, err := NewRestoreHandler(history, writer).Handle(context.Background(), RestoreEntry{Bucket: "CONFIG", Key: "feature.api", Revision: 7})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Revision != 9 || string(result.Value) != `{"enabled":true}` || writer.command.ExpectedRevision == nil || *writer.command.ExpectedRevision != 8 || string(writer.command.Value) != `{"enabled":true}` {
		t.Fatalf("result=%#v write=%#v", result, writer.command)
	}
}

func TestRestoreHandlerRejectsDeleteMarkerAsSource(t *testing.T) {
	t.Parallel()

	history := &historyReaderStub{entries: []Entry{{Revision: 7, Operation: OperationDelete}}}
	_, err := NewRestoreHandler(history, &entryWriterStub{}).Handle(context.Background(), RestoreEntry{Bucket: "CONFIG", Key: "feature.api", Revision: 7})
	if err == nil {
		t.Fatal("Handle() restored a delete marker")
	}
}
