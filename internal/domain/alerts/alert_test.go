package alerts

import (
	"errors"
	"testing"
	"time"
)

func TestNewAlertKeepsValidImmutableState(t *testing.T) {
	t.Parallel()
	occurred := time.Date(2026, 8, 9, 15, 0, 0, 0, time.UTC)
	received := occurred.Add(time.Second)
	resource, err := NewResource(ResourceStream, "ORDERS", "")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"type":"stream_action"}`)
	alert, err := NewAlert(AlertInput{ID: "evt-1", Kind: "io.nats.jetstream.advisory.v1.stream_action", Category: CategoryStream, Severity: SeverityInfo, Title: "Stream created", Description: "ORDERS was created", Source: "$JS.EVENT.ADVISORY.STREAM.CREATED.ORDERS", Resource: resource, OccurredAt: occurred, ReceivedAt: received, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	payload[0] = 'X'
	if alert.ID().String() != "evt-1" || alert.Category() != CategoryStream || alert.Severity() != SeverityInfo || string(alert.Payload()) != `{"type":"stream_action"}` {
		t.Fatalf("alert=%#v payload=%q", alert, alert.Payload())
	}
	if alert.Acknowledged() {
		t.Fatal("new alert is acknowledged")
	}
}

func TestNewAlertRejectsInvalidState(t *testing.T) {
	t.Parallel()
	now := time.Now()
	_, err := NewAlert(AlertInput{Category: CategoryStream, Severity: SeverityInfo, Title: "x", Source: "subject", OccurredAt: now, ReceivedAt: now})
	if !errors.Is(err, ErrAlertIDRequired) {
		t.Fatalf("error=%v", err)
	}
	_, err = NewAlert(AlertInput{ID: "id", Category: CategoryStream, Severity: SeverityInfo, Title: "x", Source: "subject", OccurredAt: now.Add(time.Second), ReceivedAt: now})
	if !errors.Is(err, ErrReceivedBeforeOccurrence) {
		t.Fatalf("error=%v", err)
	}
}

func TestAlertAcknowledgementIsExplicitAndIdempotent(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 15, 0, 0, 0, time.UTC)
	alert := testAlert(t, now)
	if err := alert.Acknowledge(now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if !alert.Acknowledged() || !alert.AcknowledgedAt().Equal(now.Add(time.Minute)) {
		t.Fatalf("acknowledged=%t at=%s", alert.Acknowledged(), alert.AcknowledgedAt())
	}
	if err := alert.Acknowledge(now.Add(2 * time.Minute)); !errors.Is(err, ErrAlreadyAcknowledged) {
		t.Fatalf("error=%v", err)
	}
}

func testAlert(t *testing.T, now time.Time) *Alert {
	t.Helper()
	alert, err := NewAlert(AlertInput{ID: "evt-1", Kind: "kind", Category: CategoryConsumer, Severity: SeverityWarning, Title: "Delivery failed", Source: "$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.ORDERS.WORKER", OccurredAt: now, ReceivedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	return alert
}
