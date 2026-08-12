package natsmonitor

import (
	"testing"
	"time"

	domain "github.com/fd1az/nats-tui/internal/domain/alerts"
)

func TestMapAdvisoryMapsConsumerDeliveryFailure(t *testing.T) {
	t.Parallel()
	received := time.Date(2026, 8, 9, 18, 0, 1, 0, time.UTC)
	payload := []byte(`{"type":"io.nats.jetstream.advisory.v1.max_deliver","id":"evt-42","timestamp":"2026-08-09T18:00:00Z","stream":"ORDERS","consumer":"WORKER","stream_seq":42,"deliveries":5}`)
	alert, err := mapAdvisory("$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.ORDERS.WORKER", payload, received)
	if err != nil {
		t.Fatal(err)
	}
	resource := alert.Resource()
	if alert.ID().String() != "evt-42" || alert.Category() != domain.CategoryConsumer || alert.Severity() != domain.SeverityCritical || resource.Kind() != domain.ResourceConsumer || resource.Primary() != "ORDERS" || resource.Secondary() != "WORKER" {
		t.Fatalf("alert=%#v resource=%#v", alert, resource)
	}
}

func TestMapAdvisoryMapsAPIAuditAndServerError(t *testing.T) {
	t.Parallel()
	received := time.Date(2026, 8, 9, 18, 0, 1, 0, time.UTC)
	payload := []byte(`{"type":"io.nats.jetstream.advisory.v1.api_audit","id":"audit-1","timestamp":"2026-08-09T18:00:00Z","subject":"$JS.API.STREAM.CREATE.ORDERS","response":"{\"error\":{\"code\":400,\"description\":\"bad config\"}}"}`)
	alert, err := mapAdvisory("$JS.EVENT.ADVISORY.API", payload, received)
	if err != nil {
		t.Fatal(err)
	}
	if alert.Category() != domain.CategoryAudit || alert.Severity() != domain.SeverityWarning || alert.Title() != "JetStream API error" || alert.Description() != "bad config" {
		t.Fatalf("alert=%#v description=%q", alert, alert.Description())
	}
}

func TestMapAdvisoryKeepsUnknownEventsForwardCompatible(t *testing.T) {
	t.Parallel()
	received := time.Date(2026, 8, 9, 18, 0, 0, 0, time.UTC)
	payload := []byte(`{"type":"io.nats.jetstream.advisory.v2.future_event","timestamp":"2026-08-09T18:00:01Z","detail":"future"}`)
	alert, err := mapAdvisory("$JS.EVENT.ADVISORY.FUTURE.EVENT", payload, received)
	if err != nil {
		t.Fatal(err)
	}
	if alert.Category() != domain.CategoryUnknown || alert.Severity() != domain.SeverityInfo || alert.ReceivedAt().Before(alert.OccurredAt()) || string(alert.Payload()) != string(payload) {
		t.Fatalf("alert=%#v payload=%s", alert, alert.Payload())
	}
	second, _ := mapAdvisory("$JS.EVENT.ADVISORY.FUTURE.EVENT", payload, received.Add(time.Minute))
	if second.ID() != alert.ID() {
		t.Fatalf("fallback IDs differ: %s != %s", second.ID().String(), alert.ID().String())
	}
}

func TestForwardAdvisoriesDropsMalformedPayloadButKeepsStreamAlive(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	now := time.Date(2026, 8, 9, 18, 0, 0, 0, time.UTC)
	incoming := make(chan advisoryMessage, 2)
	incoming <- advisoryMessage{subject: "$JS.EVENT.ADVISORY.API", payload: []byte("not-json")}
	incoming <- advisoryMessage{subject: "$JS.EVENT.ADVISORY.STREAM.CREATED.ORDERS", payload: []byte(`{"id":"ok","timestamp":"2026-08-09T18:00:00Z"}`)}
	close(incoming)
	events := make(chan *domain.Alert, 2)
	errors := make(chan error, 2)
	forwardAdvisories(ctx, incoming, events, errors, func() time.Time { return now })
	if len(events) != 1 || len(errors) != 1 {
		t.Fatalf("events=%d errors=%d", len(events), len(errors))
	}
}
