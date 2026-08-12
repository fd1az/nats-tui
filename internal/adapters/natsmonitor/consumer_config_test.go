package natsmonitor

import (
	"reflect"
	"testing"
	"time"

	"github.com/fd1az/nats-tui/internal/app/consumers"
)

func TestConsumerConfigurationRoundTripPreservesAdvancedFields(t *testing.T) {
	start := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	want := consumers.DefaultDefinition("EVENTS", "worker")
	want.Description = "payments"
	want.DeliverPolicy, want.StartTime = consumers.DeliverByStartTime, &start
	want.AckWait, want.MaxDeliver, want.Backoff = 30*time.Second, 3, []time.Duration{time.Second, 5 * time.Second}
	want.FilterSubjects = []string{"events.us.*", "events.eu.*"}
	want.ReplayPolicy, want.RateLimit, want.SampleFrequency = consumers.ReplayOriginal, 1024, "25%"
	want.HeadersOnly, want.MemoryStorage = true, true
	want.Metadata = map[string]string{"team": "payments"}
	want.DeliverSubject, want.DeliverGroup, want.FlowControl, want.IdleHeartbeat = "deliver.worker", "workers", true, 5*time.Second
	got := consumerConfigFromNATS(want.Stream, want.Name, consumerConfigToNATS(want))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip\n got: %#v\nwant: %#v", got, want)
	}
}
