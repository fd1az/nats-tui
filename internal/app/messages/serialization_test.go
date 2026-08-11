package messages

import (
	"strings"
	"testing"
)

func TestHeadersTextIsStableAndCompleteMessageKeepsMetadata(t *testing.T) {
	record := Record{Stream: "EVENTS", Subject: "events.created", Sequence: 42, Headers: map[string][]string{"Z": {"last"}, "A": {"first", "second"}}, Payload: []byte(`{"id":42}`)}
	if got := HeadersText(record.Headers); got != "A: first\nA: second\nZ: last" {
		t.Fatalf("headers = %q", got)
	}
	complete, err := CompleteMessage(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"Stream": "EVENTS"`, `"Sequence": 42`, `"id": 42`} {
		if !strings.Contains(complete, want) {
			t.Fatalf("complete message missing %q:\n%s", want, complete)
		}
	}
}
