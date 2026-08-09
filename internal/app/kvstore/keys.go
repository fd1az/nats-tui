package kvstore

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Operation describes the mutation represented by a KV entry.
type Operation string

const (
	OperationPut    Operation = "PUT"
	OperationDelete Operation = "DELETE"
	OperationPurge  Operation = "PURGE"
)

// Entry is a transport-independent view of one KV revision.
type Entry struct {
	Bucket    string
	Key       string
	Value     []byte
	Operation Operation
	Revision  uint64
	Created   time.Time
	Delta     uint64
}

// KeyCatalog lists the current keys in a KV bucket.
type KeyCatalog interface {
	ListKeyValueKeys(context.Context, string) ([]string, error)
}

// EntryReader loads the latest revision of one KV key.
type EntryReader interface {
	GetKeyValueEntry(context.Context, string, string) (Entry, error)
}

// ListKeysHandler validates and lists KV keys.
type ListKeysHandler struct{ catalog KeyCatalog }

// NewListKeysHandler constructs the list-keys query.
func NewListKeysHandler(catalog KeyCatalog) ListKeysHandler {
	if catalog == nil {
		panic("nil key-value key catalog")
	}
	return ListKeysHandler{catalog: catalog}
}

// Handle lists all current keys in a bucket.
func (h ListKeysHandler) Handle(ctx context.Context, bucket string) ([]string, error) {
	bucket = strings.TrimSpace(bucket)
	if !bucketNamePattern.MatchString(bucket) {
		return nil, fmt.Errorf("bucket name must contain only letters, digits, dashes, or underscores")
	}
	return h.catalog.ListKeyValueKeys(ctx, bucket)
}

// GetEntryHandler validates and loads the latest KV entry.
type GetEntryHandler struct{ reader EntryReader }

// NewGetEntryHandler constructs the get-entry query.
func NewGetEntryHandler(reader EntryReader) GetEntryHandler {
	if reader == nil {
		panic("nil key-value entry reader")
	}
	return GetEntryHandler{reader: reader}
}

// Handle loads the latest revision for key in bucket.
func (h GetEntryHandler) Handle(ctx context.Context, bucket, key string) (Entry, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if !bucketNamePattern.MatchString(bucket) {
		return Entry{}, fmt.Errorf("bucket name must contain only letters, digits, dashes, or underscores")
	}
	if key == "" {
		return Entry{}, fmt.Errorf("key is required")
	}
	return h.reader.GetKeyValueEntry(ctx, bucket, key)
}
