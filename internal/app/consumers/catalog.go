package consumers

import (
	"context"
	"time"
)

type Status string

const (
	StatusPaused  Status = "PAUSED"
	StatusActive  Status = "ACTIVE"
	StatusLagging Status = "LAGGING"
	StatusIdle    Status = "IDLE"
)

type Summary struct {
	Stream          string
	Name            string
	Status          Status
	Pending         uint64
	AckPending      int
	Redelivered     int
	Waiting         int
	DeliveredStream uint64
	AckFloorStream  uint64
	FilterSubjects  []string
	Durable         bool
	Push            bool
	CreatedAt       time.Time
	LastActiveAt    time.Time
	Config          Definition
}

func (s Summary) Definition() Definition {
	definition := s.Config.Clone()
	definition.Stream, definition.Name = s.Stream, s.Name
	return definition
}

func (s Summary) Lag() uint64 {
	return s.Pending + uint64(max(s.AckPending, 0))
}

func Classify(paused, workerActive bool, pending uint64, ackPending int) Status {
	switch {
	case paused:
		return StatusPaused
	case workerActive:
		return StatusActive
	case pending > 0 || ackPending > 0:
		return StatusLagging
	default:
		return StatusIdle
	}
}

type Catalog interface {
	ListConsumers(context.Context) ([]Summary, error)
}

type ListConsumersHandler struct{ catalog Catalog }

func NewListConsumersHandler(catalog Catalog) ListConsumersHandler {
	if catalog == nil {
		panic("nil consumer catalog")
	}
	return ListConsumersHandler{catalog: catalog}
}

func (h ListConsumersHandler) Handle(ctx context.Context) ([]Summary, error) {
	return h.catalog.ListConsumers(ctx)
}
