package streams

import (
	"context"
	"reflect"
	"testing"
)

type catalogStub struct {
	streams []Summary
}

func (s catalogStub) ListStreams(context.Context) ([]Summary, error) {
	return s.streams, nil
}

func TestListStreamsReturnsCatalogSummaries(t *testing.T) {
	t.Parallel()

	want := []Summary{{Name: "ORDERS", Messages: 42}}
	handler := NewListStreamsHandler(catalogStub{streams: want})

	got, err := handler.Handle(context.Background())
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Handle() = %#v, want %#v", got, want)
	}
}
