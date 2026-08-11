package messages

import "testing"

func TestPayloadFilterSupportsJSONPathAndJMESPathSelectors(t *testing.T) {
	payload := []byte(`{"event":{"type":"payment.created"},"items":[{"amount":42}]}`)
	tests := []struct {
		expression string
		want       bool
	}{
		{"jsonpath:$.event.type == \"payment.created\"", true},
		{"jmespath:event.type == 'payment.created'", true},
		{"$.items[0].amount == 42", true},
		{"event.missing", false},
		{"event.type != 'payment.failed'", true},
	}
	for _, test := range tests {
		filter, err := CompilePayloadFilter(test.expression)
		if err != nil {
			t.Fatalf("CompilePayloadFilter(%q): %v", test.expression, err)
		}
		if got := filter.Match(payload); got != test.want {
			t.Errorf("Match(%q) = %v, want %v", test.expression, got, test.want)
		}
	}
}

func TestPayloadFilterRejectsInvalidPathAndNonJSON(t *testing.T) {
	if _, err := CompilePayloadFilter("$.items[nope]"); err == nil {
		t.Fatal("invalid index accepted")
	}
	filter, err := CompilePayloadFilter("$.event")
	if err != nil {
		t.Fatal(err)
	}
	if filter.Match([]byte("plain text")) {
		t.Fatal("non-JSON payload matched JSON filter")
	}
}
