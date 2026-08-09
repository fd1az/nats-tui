package requestreply

import (
	"testing"
	"time"
)

func TestNewSubjectAcceptsPublishableSubjectAndRejectsWildcards(t *testing.T) {
	t.Parallel()
	subject, err := NewSubject(" events.payment.created ")
	if err != nil || subject.String() != "events.payment.created" {
		t.Fatalf("subject=%q error=%v", subject, err)
	}
	for _, invalid := range []string{"", "events.*", "events.>", "events..created", "events created"} {
		if _, err := NewSubject(invalid); err == nil {
			t.Fatalf("invalid subject %q accepted", invalid)
		}
	}
}

func TestNewMessageOwnsPayloadAndHeaders(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"ok":true}`)
	headers := map[string][]string{"Content-Type": {"application/json"}}
	message, err := NewMessage("events.created", payload, headers)
	if err != nil {
		t.Fatalf("NewMessage() error=%v", err)
	}
	payload[0] = 'x'
	headers["Content-Type"][0] = "broken"
	if string(message.Payload()) != `{"ok":true}` || message.Headers()["Content-Type"][0] != "application/json" {
		t.Fatalf("message did not own input: payload=%q headers=%#v", message.Payload(), message.Headers())
	}
	returned := message.Payload()
	returned[0] = 'x'
	if string(message.Payload()) != `{"ok":true}` {
		t.Fatal("message exposed mutable payload")
	}
}

func TestNewMessageRejectsUnsafeHeaders(t *testing.T) {
	t.Parallel()
	for _, headers := range []map[string][]string{
		{"": {"value"}}, {"Bad Header": {"value"}}, {"X-Test": {"ok\r\ninjected"}},
	} {
		if _, err := NewMessage("events.created", nil, headers); err == nil {
			t.Fatalf("invalid headers accepted: %#v", headers)
		}
	}
}

func TestNewRequestRequiresBoundedTimeoutAndReplyCount(t *testing.T) {
	t.Parallel()
	message, err := NewMessage("rpc.lookup", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	request, err := NewRequest(message, 2*time.Second, 3)
	if err != nil || request.Timeout() != 2*time.Second || request.MaxReplies() != 3 {
		t.Fatalf("request=%#v error=%v", request, err)
	}
	for _, input := range []struct {
		timeout time.Duration
		max     int
	}{{0, 1}, {-time.Second, 1}, {time.Second, 0}, {time.Second, 1001}} {
		if _, err := NewRequest(message, input.timeout, input.max); err == nil {
			t.Fatalf("invalid request accepted: %#v", input)
		}
	}
}

func TestNewTemplateValidatesNameAndOwnsRequest(t *testing.T) {
	t.Parallel()
	message, _ := NewMessage("rpc.lookup", []byte(`{"id":42}`), nil)
	request, _ := NewRequest(message, time.Second, 2)
	template, err := NewTemplate(" lookup-user ", request)
	if err != nil || template.Name() != "lookup-user" || template.Request().Message().Subject().String() != "rpc.lookup" {
		t.Fatalf("template=%#v error=%v", template, err)
	}
	for _, invalid := range []string{"", "bad/name", "bad name"} {
		if _, err := NewTemplate(invalid, request); err == nil {
			t.Fatalf("invalid template name %q accepted", invalid)
		}
	}
}
