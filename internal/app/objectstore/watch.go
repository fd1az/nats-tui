package objectstore

import (
	"context"
	"fmt"
	"strings"
)

// ObjectEventKind classifies an Object Store catalog mutation.
type ObjectEventKind uint8

const (
	// ObjectUpserted indicates a created or metadata-updated object.
	ObjectUpserted ObjectEventKind = iota + 1
	// ObjectDeleted indicates an object delete marker.
	ObjectDeleted
)

// ObjectEvent is an immutable Object Store catalog mutation.
type ObjectEvent struct {
	Kind   ObjectEventKind
	Object ObjectSummary
}

// ObjectWatchSource streams future object metadata mutations.
type ObjectWatchSource interface {
	WatchObjects(context.Context, string) (<-chan ObjectEvent, error)
}

// WatchObjectsHandler validates and starts a future-only object watch.
type WatchObjectsHandler struct{ source ObjectWatchSource }

// NewWatchObjectsHandler constructs an object watch use case.
func NewWatchObjectsHandler(source ObjectWatchSource) WatchObjectsHandler {
	if source == nil {
		panic("nil object watch source")
	}
	return WatchObjectsHandler{source: source}
}

// Handle watches future mutations in a validated bucket.
func (h WatchObjectsHandler) Handle(ctx context.Context, bucket string) (<-chan ObjectEvent, error) {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return nil, fmt.Errorf("object-store bucket is required")
	}
	return h.source.WatchObjects(ctx, bucket)
}
