// Package kvstore contains application read models and use cases for NATS KV stores.
package kvstore

import (
	"context"
	"time"
)

// Storage identifies the bucket's JetStream storage medium.
type Storage string

const (
	StorageFile   Storage = "FILE"
	StorageMemory Storage = "MEMORY"
)

// BucketSummary contains the operational fields needed by the bucket catalog.
type BucketSummary struct {
	Name         string
	Description  string
	Values       uint64
	Bytes        uint64
	History      int64
	TTL          time.Duration
	Storage      Storage
	Replicas     int
	Compressed   bool
	MaxBytes     int64
	MaxValueSize int32
	Mirror       string
	Sources      []string
}

// BucketCatalog lists KV bucket summaries.
type BucketCatalog interface {
	ListKeyValueBuckets(context.Context) ([]BucketSummary, error)
}

// ListBucketsHandler loads the KV bucket catalog.
type ListBucketsHandler struct{ catalog BucketCatalog }

// NewListBucketsHandler constructs a bucket catalog query handler.
func NewListBucketsHandler(catalog BucketCatalog) ListBucketsHandler {
	if catalog == nil {
		panic("nil key-value bucket catalog")
	}
	return ListBucketsHandler{catalog: catalog}
}

// Handle returns all visible KV buckets.
func (h ListBucketsHandler) Handle(ctx context.Context) ([]BucketSummary, error) {
	return h.catalog.ListKeyValueBuckets(ctx)
}
