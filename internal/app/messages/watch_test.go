package messages

import (
	"context"
	"reflect"
	"testing"
)

type watcherStub struct {
	subjects []string
	events   chan Record
}

func (s *watcherStub) Watch(_ context.Context, subjects []string) (<-chan Record, error) {
	s.subjects = subjects
	return s.events, nil
}

func TestWatchHandlerPreservesSubjectsAndEventChannel(t *testing.T) {
	t.Parallel()

	events := make(chan Record)
	watcher := &watcherStub{events: events}
	handler := NewWatchHandler(watcher)
	got, err := handler.Handle(context.Background(), []string{"orders.>"})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if got != events || !reflect.DeepEqual(watcher.subjects, []string{"orders.>"}) {
		t.Fatalf("channel or subjects were not preserved")
	}
}
