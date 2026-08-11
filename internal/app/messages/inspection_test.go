package messages

import (
	"strings"
	"testing"
)

func TestInspectPayloadModes(t *testing.T) {
	tests := []struct {
		mode    InspectionMode
		payload []byte
		want    string
	}{
		{InspectAuto, []byte(`{"event":"created"}`), "\n"},
		{InspectXML, []byte(`<event><id>42</id></event>`), "  <id>42</id>"},
		{InspectText, []byte("hello"), "hello"},
		{InspectHex, []byte{0, 1, 2}, "00000000"},
		{InspectBinary, []byte("hello"), "aGVsbG8="},
	}
	for _, test := range tests {
		got, _, err := InspectPayload(test.payload, test.mode)
		if err != nil {
			t.Fatalf("InspectPayload(%s): %v", test.mode, err)
		}
		if !strings.Contains(got, test.want) {
			t.Errorf("InspectPayload(%s) = %q, want %q", test.mode, got, test.want)
		}
	}
}

func TestDetectInspectionModeUsesHexForBinary(t *testing.T) {
	if got := DetectInspectionMode([]byte{0xff, 0x00}); got != InspectHex {
		t.Fatalf("mode = %s", got)
	}
}
