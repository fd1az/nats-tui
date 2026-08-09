// Package objectstore contains application use cases for JetStream Object Stores.
package objectstore

import (
	"context"
	"time"
)

type BucketSummary struct {
	Name        string
	Description string
	Size        uint64
	TTL         time.Duration
	Storage     string
	Replicas    int
	Compressed  bool
	Sealed      bool
	MaxBytes    int64
	Metadata    map[string]string
	Mirror      string
	Sources     []string
}

type BucketCatalog interface {
	ListObjectStoreBuckets(context.Context) ([]BucketSummary, error)
}

type ListBucketsHandler struct{ catalog BucketCatalog }

func NewListBucketsHandler(catalog BucketCatalog) ListBucketsHandler {
	if catalog == nil {
		panic("nil object-store bucket catalog")
	}
	return ListBucketsHandler{catalog: catalog}
}

func (h ListBucketsHandler) Handle(ctx context.Context) ([]BucketSummary, error) {
	return h.catalog.ListObjectStoreBuckets(ctx)
}
