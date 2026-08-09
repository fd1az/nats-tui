package objectstore

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ObjectSummary contains object metadata without loading its contents.
type ObjectSummary struct {
	Bucket      string
	ID          string
	Name        string
	Description string
	Size        uint64
	Chunks      uint32
	ModifiedAt  time.Time
	Digest      string
	Deleted     bool
	Headers     map[string][]string
	Metadata    map[string]string
	Link        *LinkTarget
}

// ObjectCatalog lists object metadata for a bucket.
type ObjectCatalog interface {
	ListObjects(context.Context, string) ([]ObjectSummary, error)
}

// ListObjectsHandler loads and normalizes an object's metadata catalog.
type ListObjectsHandler struct{ catalog ObjectCatalog }

// NewListObjectsHandler constructs the object catalog query.
func NewListObjectsHandler(catalog ObjectCatalog) ListObjectsHandler {
	if catalog == nil {
		panic("nil object catalog")
	}
	return ListObjectsHandler{catalog: catalog}
}

// Handle returns objects sorted by name for stable keyboard navigation.
func (h ListObjectsHandler) Handle(ctx context.Context, bucket string) ([]ObjectSummary, error) {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return nil, fmt.Errorf("object-store bucket is required")
	}
	objects, err := h.catalog.ListObjects(ctx, bucket)
	if err != nil {
		return nil, err
	}
	sort.Slice(objects, func(left, right int) bool { return objects[left].Name < objects[right].Name })
	return objects, nil
}
