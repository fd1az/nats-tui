package messages

import (
	"context"
	"time"
)

type Record struct {
	Sequence uint64
	Subject  string
	Time     time.Time
	Headers  map[string][]string
	Payload  []byte
}

type Reader interface {
	RecentMessages(context.Context, string, int) ([]Record, error)
}

type LoadRecentHandler struct {
	reader Reader
}

func NewLoadRecentHandler(reader Reader) LoadRecentHandler {
	if reader == nil {
		panic("nil message reader")
	}
	return LoadRecentHandler{reader: reader}
}

func (h LoadRecentHandler) Handle(ctx context.Context, stream string, limit int) ([]Record, error) {
	return h.reader.RecentMessages(ctx, stream, limit)
}
