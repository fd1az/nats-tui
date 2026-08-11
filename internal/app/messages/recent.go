package messages

import (
	"context"
	"fmt"
	"time"
)

type Record struct {
	Sequence         uint64
	ConsumerSequence uint64
	Subject          string
	Time             time.Time
	Headers          map[string][]string
	Payload          []byte
	Stream           string
	Consumer         string
	Domain           string
	Deliveries       uint64
	Pending          uint64
	WatchPending     int
	WatchCapacity    int
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

type HistoryQuery struct {
	Stream            string
	Subject           string
	StartSequence     uint64
	EndSequence       uint64
	StartTime         *time.Time
	Limit             int
	PayloadExpression string
}

func (q HistoryQuery) Validate() error {
	if q.Stream == "" {
		return fmt.Errorf("history stream is required")
	}
	if q.Limit < 1 || q.Limit > 10_000 {
		return fmt.Errorf("history limit must be between 1 and 10000")
	}
	if q.EndSequence > 0 && q.StartSequence > q.EndSequence {
		return fmt.Errorf("history start sequence cannot exceed end sequence")
	}
	return nil
}

type Browser interface {
	BrowseMessages(context.Context, HistoryQuery) ([]Record, error)
}

type BrowseHandler struct{ browser Browser }

func NewBrowseHandler(browser Browser) BrowseHandler {
	if browser == nil {
		panic("nil message browser")
	}
	return BrowseHandler{browser: browser}
}

func (h BrowseHandler) Handle(ctx context.Context, query HistoryQuery) ([]Record, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	filter, err := CompilePayloadFilter(query.PayloadExpression)
	if err != nil {
		return nil, err
	}
	requestedLimit := query.Limit
	if !filter.Empty() {
		query.Limit = min(max(requestedLimit*20, 1000), 10_000)
	}
	records, err := h.browser.BrowseMessages(ctx, query)
	if err != nil {
		return nil, err
	}
	if filter.Empty() {
		return records, nil
	}
	result := make([]Record, 0, min(requestedLimit, len(records)))
	for _, record := range records {
		if filter.Match(record.Payload) {
			result = append(result, record)
			if len(result) == requestedLimit {
				break
			}
		}
	}
	return result, nil
}
