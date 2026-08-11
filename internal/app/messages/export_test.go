package messages

import (
	"context"
	"strings"
	"testing"
)

type exportWriterStub struct {
	path    string
	content []byte
	force   bool
}

func (w *exportWriterStub) WriteMessageFile(_ context.Context, path string, content []byte, force bool) error {
	w.path, w.content, w.force = path, append([]byte(nil), content...), force
	return nil
}

func TestExportHandlerUsesSnapshotAndStableJSONL(t *testing.T) {
	t.Parallel()
	writer := new(exportWriterStub)
	records := []Record{{Stream: "EVENTS", Sequence: 42, Subject: "events.created", Payload: []byte(`{"id":42}`)}}
	result, err := NewExportHandler(nil, writer).Handle(context.Background(), ExportRequest{Path: "events.jsonl", Format: ExportJSONL, Force: true, Records: records})
	if err != nil {
		t.Fatal(err)
	}
	if result.Messages != 1 || writer.path != "events.jsonl" || !writer.force || !strings.Contains(string(writer.content), `"Sequence":42`) {
		t.Fatalf("result=%+v writer=%+v", result, writer)
	}
}

func TestRawExportRequiresExactlyOneMessage(t *testing.T) {
	t.Parallel()
	_, err := NewExportHandler(nil, new(exportWriterStub)).Handle(context.Background(), ExportRequest{Path: "payload.bin", Format: ExportRaw, Records: []Record{{}, {}}})
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("error=%v", err)
	}
}
