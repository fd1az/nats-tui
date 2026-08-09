package consumers

import (
	"context"
	"reflect"
	"testing"
)

type catalogStub struct{ consumers []Summary }

func (s catalogStub) ListConsumers(context.Context) ([]Summary, error) { return s.consumers, nil }

func TestConsumerLagIncludesUndeliveredAndUnacknowledgedMessages(t *testing.T) {
	t.Parallel()

	consumer := Summary{Pending: 40, AckPending: 2}
	if got := consumer.Lag(); got != 42 {
		t.Fatalf("Lag() = %d, want 42", got)
	}
}

func TestConsumerStatusClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		paused, active bool
		pending        uint64
		ackPending     int
		want           Status
	}{
		{name: "paused wins", paused: true, active: true, pending: 4, want: StatusPaused},
		{name: "active worker", active: true, want: StatusActive},
		{name: "lag without worker", pending: 4, want: StatusLagging},
		{name: "idle", want: StatusIdle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.paused, tt.active, tt.pending, tt.ackPending); got != tt.want {
				t.Fatalf("Classify() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestListConsumersReturnsCatalog(t *testing.T) {
	t.Parallel()

	want := []Summary{{Stream: "ORDERS", Name: "worker"}}
	handler := NewListConsumersHandler(catalogStub{consumers: want})
	got, err := handler.Handle(context.Background())
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Handle() = %#v, %v", got, err)
	}
}
