package streams

import (
	"context"
	"time"
)

type Summary struct {
	Name        string
	Description string
	Subjects    []string
	Messages    uint64
	Bytes       uint64
	Consumers   int
	Storage     Storage
	Retention   Retention
	Replicas    int
	MaxMessages int64
	MaxBytes    int64
	FirstAt     time.Time
	LastAt      time.Time
}

type Catalog interface {
	ListStreams(context.Context) ([]Summary, error)
}

type ListStreamsHandler struct {
	catalog Catalog
}

func NewListStreamsHandler(catalog Catalog) ListStreamsHandler {
	if catalog == nil {
		panic("nil stream catalog")
	}
	return ListStreamsHandler{catalog: catalog}
}

func (h ListStreamsHandler) Handle(ctx context.Context) ([]Summary, error) {
	return h.catalog.ListStreams(ctx)
}
