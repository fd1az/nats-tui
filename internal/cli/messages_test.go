package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/clcsolutions/nats-tui/internal/app/messages"
	"github.com/clcsolutions/nats-tui/internal/config"
	domain "github.com/clcsolutions/nats-tui/internal/domain/requestreply"
)

type messageManagerStub struct {
	query   messages.HistoryQuery
	export  messages.ExportRequest
	records []messages.Record
}

func (s *messageManagerStub) BrowseStoredMessages(_ context.Context, _ config.Settings, query messages.HistoryQuery) ([]messages.Record, error) {
	s.query = query
	return s.records, nil
}

func (s *messageManagerStub) ExportStoredMessages(_ context.Context, _ config.Settings, request messages.ExportRequest) (messages.ExportResult, error) {
	s.export = request
	return messages.ExportResult{Path: request.Path, Format: string(request.Format), Messages: 3, Bytes: 99}, nil
}

type messagePublisherStub struct{ message domain.Message }

func (s *messagePublisherStub) PublishCoreMessage(_ context.Context, _ config.Settings, message domain.Message) error {
	s.message = message
	return nil
}

func TestMessageHistoryParsesBoundedPayloadQuery(t *testing.T) {
	t.Parallel()
	service := &messageManagerStub{records: []messages.Record{{Stream: "EVENTS", Sequence: 42, Subject: "events.created", Payload: []byte(`{"type":"created"}`)}}}
	root := NewRoot(Dependencies{Messages: service})
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetArgs([]string{"message", "history", "EVENTS", "--subject", "events.*", "--start-sequence", "40", "--limit", "25", "--payload-filter", "$.type == created"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.query.Stream != "EVENTS" || service.query.Subject != "events.*" || service.query.StartSequence != 40 || service.query.Limit != 25 || service.query.PayloadExpression == "" {
		t.Fatalf("query=%+v", service.query)
	}
	if !strings.Contains(output.String(), `"Sequence": 42`) {
		t.Fatalf("output=%s", output)
	}
}

func TestMessageRepublishPreservesPayloadAndHeaders(t *testing.T) {
	t.Parallel()
	manager := &messageManagerStub{records: []messages.Record{{Sequence: 7, Subject: "events.old", Headers: map[string][]string{"X-Trace": {"abc"}}, Payload: []byte("payload")}}}
	publisher := new(messagePublisherStub)
	root := NewRoot(Dependencies{Messages: manager, CorePublisher: publisher})
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"message", "republish", "EVENTS", "7", "events.replayed"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if publisher.message.Subject().String() != "events.replayed" || string(publisher.message.Payload()) != "payload" || publisher.message.Headers()["X-Trace"][0] != "abc" {
		t.Fatalf("message subject=%s payload=%q headers=%v", publisher.message.Subject(), publisher.message.Payload(), publisher.message.Headers())
	}
}
