package dashboard

import (
	"testing"
	"time"
)

func TestTimelineCalculatesRatesAndBoundsSamples(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	timeline := NewTimeline(2)
	timeline.Record(Snapshot{ObservedAt: start, Runtime: Runtime{InMessages: 10, OutMessages: 10, InBytes: 100, OutBytes: 100}})
	sample := timeline.Record(Snapshot{ObservedAt: start.Add(2 * time.Second), Runtime: Runtime{InMessages: 14, OutMessages: 16, InBytes: 160, OutBytes: 180}})
	if sample.MessagesPerSec != 5 || sample.BytesPerSec != 70 {
		t.Fatalf("rates = %.1f msg/s %.1f B/s", sample.MessagesPerSec, sample.BytesPerSec)
	}
	timeline.Record(Snapshot{ObservedAt: start.Add(4 * time.Second)}) // counter reset
	if samples := timeline.Samples(); len(samples) != 2 || samples[1].MessagesPerSec != 0 {
		t.Fatalf("samples = %#v", samples)
	}
}

func TestNewPanelValidatesMetricAndDefaultsTitle(t *testing.T) {
	t.Parallel()
	panel, err := NewPanel("traffic", "", MetricMessageRate)
	if err != nil {
		t.Fatal(err)
	}
	if panel.Title != "MESSAGE RATE" {
		t.Fatalf("title = %q", panel.Title)
	}
	if _, err := NewPanel("bad id", "", MetricCPU); err == nil {
		t.Fatal("expected invalid ID error")
	}
	if _, err := NewPanel("bad", "", Metric("unknown")); err == nil {
		t.Fatal("expected invalid metric error")
	}
}
