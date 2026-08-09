package messages

import (
	"fmt"
	"testing"
)

func TestSearchIndexMatchesSubjectPayloadHeadersAndMultipleTerms(t *testing.T) {
	t.Parallel()

	index := NewSearchIndex([]Record{
		{Sequence: 1, Subject: "orders.created", Payload: []byte(`{"tenant":"CLC","status":"pending"}`)},
		{Sequence: 2, Subject: "payments.settled", Headers: map[string][]string{"Trace-ID": {"abc-123"}}},
	})

	tests := []struct {
		query string
		want  uint64
	}{
		{query: "ORDERS", want: 1},
		{query: "clc pending", want: 1},
		{query: "trace-id abc-123", want: 2},
	}
	for _, tt := range tests {
		result := index.Search(tt.query)
		if len(result) != 1 || result[0].Sequence != tt.want {
			t.Errorf("Search(%q) = %#v, want sequence %d", tt.query, result, tt.want)
		}
	}
}

func TestSearchIndexPrependsLiveMessageAndRespectsBufferLimit(t *testing.T) {
	t.Parallel()

	index := NewSearchIndex([]Record{{Sequence: 2}, {Sequence: 1}})
	index.Prepend(Record{Sequence: 3, Subject: "events.live"}, 2)

	result := index.Search("")
	if len(result) != 2 || result[0].Sequence != 3 || result[1].Sequence != 2 {
		t.Fatalf("Search() = %#v, want sequences 3, 2", result)
	}
}

func BenchmarkSearchIndexFiveThousandMessages(b *testing.B) {
	records := make([]Record, 5_000)
	for index := range records {
		records[index] = Record{Subject: fmt.Sprintf("events.tenant.%d", index%20), Payload: []byte(fmt.Sprintf(`{"id":%d,"status":"pending"}`, index))}
	}
	search := NewSearchIndex(records)
	result := make([]Record, 0, len(records))
	b.ResetTimer()
	for range b.N {
		result = search.SearchInto(result, "tenant.7 pending")
	}
}
