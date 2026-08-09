package kvstore

import (
	"context"
	"reflect"
	"testing"
)

type historyReaderStub struct {
	bucket  string
	key     string
	entries []Entry
}

func (s *historyReaderStub) ListKeyValueHistory(_ context.Context, bucket, key string) ([]Entry, error) {
	s.bucket, s.key = bucket, key
	return s.entries, nil
}

func TestListHistoryHandlerValidatesAndReturnsRevisions(t *testing.T) {
	t.Parallel()

	want := []Entry{{Bucket: "CONFIG", Key: "feature.api", Revision: 1}, {Bucket: "CONFIG", Key: "feature.api", Revision: 2}}
	reader := &historyReaderStub{entries: want}
	got, err := NewListHistoryHandler(reader).Handle(context.Background(), " CONFIG ", " feature.api ")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if reader.bucket != "CONFIG" || reader.key != "feature.api" || !reflect.DeepEqual(got, want) {
		t.Fatalf("reader=(%q,%q) entries=%#v", reader.bucket, reader.key, got)
	}
}

func TestListHistoryHandlerRejectsEmptyKey(t *testing.T) {
	t.Parallel()

	_, err := NewListHistoryHandler(&historyReaderStub{}).Handle(context.Background(), "CONFIG", " ")
	if err == nil {
		t.Fatal("Handle() accepted empty key")
	}
}
