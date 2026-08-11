package messages

import (
	"context"
	"testing"
)

type browserStub struct {
	query   HistoryQuery
	records []Record
}

func (s *browserStub) BrowseMessages(_ context.Context, query HistoryQuery) ([]Record, error) {
	s.query = query
	return s.records, nil
}

func TestBrowseHandlerFiltersPayloadAndPreservesRequestedLimit(t *testing.T) {
	browser := &browserStub{records: []Record{
		{Payload: []byte(`{"type":"skip"}`)},
		{Payload: []byte(`{"type":"keep"}`)},
		{Payload: []byte(`{"type":"keep"}`)},
		{Payload: []byte(`{"type":"keep"}`)},
	}}
	handler := NewBrowseHandler(browser)
	got, err := handler.Handle(context.Background(), HistoryQuery{Stream: "EVENTS", Limit: 2, PayloadExpression: "type == 'keep'"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || browser.query.Limit != 1000 {
		t.Fatalf("records=%d scan limit=%d", len(got), browser.query.Limit)
	}
}

func TestHistoryQueryRejectsUnsafeLimits(t *testing.T) {
	for _, limit := range []int{0, 10_001} {
		if err := (HistoryQuery{Stream: "EVENTS", Limit: limit}).Validate(); err == nil {
			t.Fatalf("limit %d accepted", limit)
		}
	}
}

func TestHistoryQueryRejectsInvertedSequenceRange(t *testing.T) {
	t.Parallel()
	if err := (HistoryQuery{Stream: "EVENTS", StartSequence: 42, EndSequence: 41, Limit: 1}).Validate(); err == nil {
		t.Fatal("inverted sequence range accepted")
	}
}
