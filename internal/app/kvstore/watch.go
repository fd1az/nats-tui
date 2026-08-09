package kvstore

import (
	"context"
	"fmt"
	"strings"
)

// WatchSource streams KV mutations until the context is canceled.
type WatchSource interface {
	WatchKeyValue(context.Context, string, string) (<-chan Entry, error)
}

// WatchHandler validates and starts a KV watch.
type WatchHandler struct{ source WatchSource }

// NewWatchHandler constructs the watch query handler.
func NewWatchHandler(source WatchSource) WatchHandler {
	if source == nil {
		panic("nil key-value watch source")
	}
	return WatchHandler{source: source}
}

// Handle starts watching key or wildcard in bucket.
func (h WatchHandler) Handle(ctx context.Context, bucket, key string) (<-chan Entry, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if !bucketNamePattern.MatchString(bucket) {
		return nil, fmt.Errorf("bucket name must contain only letters, digits, dashes, or underscores")
	}
	if key == "" {
		key = ">"
	}
	return h.source.WatchKeyValue(ctx, bucket, key)
}
