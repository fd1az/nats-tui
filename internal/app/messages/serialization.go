package messages

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

func HeadersText(headers map[string][]string) string {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	lines := make([]string, 0, len(names))
	for _, name := range names {
		for _, value := range headers[name] {
			lines = append(lines, name+": "+value)
		}
	}
	return strings.Join(lines, "\n")
}

func CompleteMessage(record Record) (string, error) {
	var payload any
	encoding := "json"
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		if utf8.Valid(record.Payload) {
			payload, encoding = string(record.Payload), "text"
		} else {
			payload, encoding = base64.StdEncoding.EncodeToString(record.Payload), "base64"
		}
	}
	envelope := struct {
		Stream, Consumer, Domain, Subject, Time, PayloadEncoding string
		Sequence, ConsumerSequence, Deliveries, Pending          uint64
		Headers                                                  map[string][]string
		Payload                                                  any
	}{
		Stream: record.Stream, Consumer: record.Consumer, Domain: record.Domain, Subject: record.Subject, Time: record.Time.Format("2006-01-02T15:04:05.999999999Z07:00"),
		PayloadEncoding: encoding, Sequence: record.Sequence, ConsumerSequence: record.ConsumerSequence, Deliveries: record.Deliveries, Pending: record.Pending,
		Headers: cloneHeaders(record.Headers), Payload: payload,
	}
	encoded, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return "", fmt.Errorf("serialize complete message: %w", err)
	}
	return string(encoded), nil
}

func cloneHeaders(source map[string][]string) map[string][]string {
	result := make(map[string][]string, len(source))
	for name, values := range source {
		result[name] = append([]string(nil), values...)
	}
	return result
}
