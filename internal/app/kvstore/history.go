package kvstore

import (
	"context"
	"fmt"
	"strings"
)

// HistoryReader loads every retained revision for a KV key.
type HistoryReader interface {
	ListKeyValueHistory(context.Context, string, string) ([]Entry, error)
}

// ListHistoryHandler validates and loads retained KV revisions.
type ListHistoryHandler struct{ reader HistoryReader }

// NewListHistoryHandler constructs the history query handler.
func NewListHistoryHandler(reader HistoryReader) ListHistoryHandler {
	if reader == nil {
		panic("nil key-value history reader")
	}
	return ListHistoryHandler{reader: reader}
}

// Handle returns all retained revisions for a key.
func (h ListHistoryHandler) Handle(ctx context.Context, bucket, key string) ([]Entry, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if !bucketNamePattern.MatchString(bucket) {
		return nil, fmt.Errorf("bucket name must contain only letters, digits, dashes, or underscores")
	}
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}
	return h.reader.ListKeyValueHistory(ctx, bucket, key)
}
