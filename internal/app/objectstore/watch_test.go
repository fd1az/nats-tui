package objectstore

import (
	"context"
	"errors"
	"testing"
)

type objectWatchSourceStub struct {
	bucket string
	events <-chan ObjectEvent
	err    error
}

func (s *objectWatchSourceStub) WatchObjects(_ context.Context, bucket string) (<-chan ObjectEvent, error) {
	s.bucket = bucket
	return s.events, s.err
}

func TestWatchObjectsHandlerValidatesBucketBeforeStarting(t *testing.T) {
	t.Parallel()
	source := &objectWatchSourceStub{}
	if _, err := NewWatchObjectsHandler(source).Handle(context.Background(), " "); err == nil {
		t.Fatal("Handle() accepted an empty bucket")
	}
	if source.bucket != "" {
		t.Fatalf("source called with %q", source.bucket)
	}
}

func TestWatchObjectsHandlerReturnsTypedEvents(t *testing.T) {
	t.Parallel()
	events := make(chan ObjectEvent, 1)
	events <- ObjectEvent{Kind: ObjectUpserted, Object: ObjectSummary{Bucket: "ARTIFACTS", ID: "id-1", Name: "release.tar"}}
	close(events)
	source := &objectWatchSourceStub{events: events}
	result, err := NewWatchObjectsHandler(source).Handle(context.Background(), " ARTIFACTS ")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if source.bucket != "ARTIFACTS" {
		t.Fatalf("bucket = %q", source.bucket)
	}
	event := <-result
	if event.Kind != ObjectUpserted || event.Object.ID != "id-1" {
		t.Fatalf("event = %#v", event)
	}
}

func TestWatchObjectsHandlerPreservesSourceError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("watch unavailable")
	_, err := NewWatchObjectsHandler(&objectWatchSourceStub{err: wantErr}).Handle(context.Background(), "ARTIFACTS")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Handle() error = %v, want %v", err, wantErr)
	}
}
