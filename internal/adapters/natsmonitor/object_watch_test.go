package natsmonitor

import (
	"context"
	"testing"

	"github.com/fd1az/nats-tui/internal/app/objectstore"
	"github.com/nats-io/nats.go/jetstream"
)

type objectWatcherStub struct {
	updates chan *jetstream.ObjectInfo
	stopped bool
}

func (w *objectWatcherStub) Updates() <-chan *jetstream.ObjectInfo { return w.updates }
func (w *objectWatcherStub) Stop() error {
	w.stopped = true
	return nil
}

func TestForwardObjectEventsMapsDeletesAndStopsWatcher(t *testing.T) {
	t.Parallel()
	watcher := &objectWatcherStub{updates: make(chan *jetstream.ObjectInfo, 1)}
	watcher.updates <- &jetstream.ObjectInfo{ObjectMeta: jetstream.ObjectMeta{Name: "release.tar"}, NUID: "id-1", Deleted: true}
	close(watcher.updates)
	events := make(chan objectstore.ObjectEvent)
	go forwardObjectEvents(context.Background(), "ARTIFACTS", watcher, events)
	event := <-events
	if event.Kind != objectstore.ObjectDeleted || event.Object.Bucket != "ARTIFACTS" || event.Object.ID != "id-1" {
		t.Fatalf("event = %#v", event)
	}
	if _, open := <-events; open {
		t.Fatal("event channel remained open after watcher ended")
	}
	if !watcher.stopped {
		t.Fatal("watcher was not stopped")
	}
}

func TestForwardObjectEventsCancellationUnblocksPendingSend(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	watcher := &objectWatcherStub{updates: make(chan *jetstream.ObjectInfo, 1)}
	watcher.updates <- &jetstream.ObjectInfo{ObjectMeta: jetstream.ObjectMeta{Name: "release.tar"}}
	events := make(chan objectstore.ObjectEvent)
	go forwardObjectEvents(ctx, "ARTIFACTS", watcher, events)
	cancel()
	for range events {
	}
	if !watcher.stopped {
		t.Fatal("watcher was not stopped after cancellation")
	}
}
