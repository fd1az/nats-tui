package kvstore

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type keyCatalogStub struct {
	keys   []string
	err    error
	bucket string
}

func (s *keyCatalogStub) ListKeyValueKeys(_ context.Context, bucket string) ([]string, error) {
	s.bucket = bucket
	return s.keys, s.err
}

func TestListKeysValidatesBucketAndReturnsCatalog(t *testing.T) {
	t.Parallel()

	catalog := &keyCatalogStub{keys: []string{"session.1", "session.2"}}
	got, err := NewListKeysHandler(catalog).Handle(context.Background(), "SESSIONS")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if catalog.bucket != "SESSIONS" || !reflect.DeepEqual(got, catalog.keys) {
		t.Fatalf("keys = %#v", got)
	}
	if _, err := NewListKeysHandler(catalog).Handle(context.Background(), "bad.bucket"); err == nil {
		t.Fatal("invalid bucket accepted")
	}
}

func TestListKeysPreservesCatalogError(t *testing.T) {
	t.Parallel()
	want := errors.New("unavailable")
	_, got := NewListKeysHandler(&keyCatalogStub{err: want}).Handle(context.Background(), "SESSIONS")
	if !errors.Is(got, want) {
		t.Fatalf("error = %v", got)
	}
}

type entryReaderStub struct {
	bucket string
	key    string
	entry  Entry
}

func (s *entryReaderStub) GetKeyValueEntry(_ context.Context, bucket, key string) (Entry, error) {
	s.bucket, s.key = bucket, key
	return s.entry, nil
}

func TestGetEntryHandlerReturnsTransportIndependentEntry(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	reader := &entryReaderStub{entry: Entry{Bucket: "SESSIONS", Key: "session.42", Value: []byte(`{"active":true}`), Operation: OperationPut, Revision: 7, Created: created, Delta: 2}}
	entry, err := NewGetEntryHandler(reader).Handle(context.Background(), " SESSIONS ", " session.42 ")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if reader.bucket != "SESSIONS" || reader.key != "session.42" || entry.Revision != 7 || entry.Delta != 2 {
		t.Fatalf("reader=(%q,%q), entry=%#v", reader.bucket, reader.key, entry)
	}
}

func TestGetEntryHandlerRejectsEmptyKey(t *testing.T) {
	t.Parallel()

	_, err := NewGetEntryHandler(&entryReaderStub{}).Handle(context.Background(), "SESSIONS", "  ")
	if err == nil {
		t.Fatal("Handle() error = nil")
	}
}
