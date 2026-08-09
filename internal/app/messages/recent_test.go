package messages

import (
	"context"
	"reflect"
	"testing"
)

type readerStub struct{ records []Record }

func (s readerStub) RecentMessages(context.Context, string, int) ([]Record, error) {
	return s.records, nil
}

func TestLoadRecentMessagesDelegatesToReader(t *testing.T) {
	t.Parallel()

	want := []Record{{Sequence: 7, Subject: "orders.created"}}
	handler := NewLoadRecentHandler(readerStub{records: want})
	got, err := handler.Handle(context.Background(), "ORDERS", 50)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Handle() = %#v, want %#v", got, want)
	}
}
