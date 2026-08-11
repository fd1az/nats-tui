package subjects

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const DefaultTrafficWindow = 10 * time.Second

type TrafficSample struct {
	Subject string
	Bytes   uint64
	At      time.Time
}

func (s TrafficSample) Validate() error {
	if strings.TrimSpace(s.Subject) == "" || s.At.IsZero() {
		return fmt.Errorf("traffic sample requires subject and timestamp")
	}
	return nil
}

type Observer interface {
	ObserveSubjects(context.Context) (<-chan TrafficSample, error)
}

type ObserveHandler struct{ observer Observer }

func NewObserveHandler(observer Observer) ObserveHandler {
	if observer == nil {
		panic("nil subject observer")
	}
	return ObserveHandler{observer: observer}
}

func (h ObserveHandler) Handle(ctx context.Context) (<-chan TrafficSample, error) {
	return h.observer.ObserveSubjects(ctx)
}

type TrafficMetric struct {
	Messages    uint64
	Bytes       uint64
	MessageRate float64
	ByteRate    float64
}

type TrafficSnapshot struct {
	Window   time.Duration
	Subjects map[string]TrafficMetric
	Observed map[string]bool
}

type trafficBucket struct {
	second int64
	values map[string]TrafficMetric
}

// TrafficTracker is a bounded, deterministic rolling projection. Callers pass
// time explicitly so tests and replay never depend on the wall clock.
type TrafficTracker struct {
	window   time.Duration
	buckets  []trafficBucket
	observed map[string]int64
}

func NewTrafficTracker(window time.Duration) *TrafficTracker {
	if window <= 0 {
		panic("traffic window must be positive")
	}
	return &TrafficTracker{window: window, observed: make(map[string]int64)}
}

func (t *TrafficTracker) Observe(samples ...TrafficSample) int {
	accepted := 0
	for _, sample := range samples {
		if sample.Validate() != nil || IsInternalSubject(sample.Subject) {
			continue
		}
		accepted++
		second := sample.At.Unix()
		t.observed[sample.Subject] = second
		index := len(t.buckets) - 1
		if index < 0 || t.buckets[index].second != second {
			t.buckets = append(t.buckets, trafficBucket{second: second, values: make(map[string]TrafficMetric)})
			index++
		}
		for _, subject := range subjectHierarchy(sample.Subject) {
			metric := t.buckets[index].values[subject]
			metric.Messages++
			metric.Bytes += sample.Bytes
			t.buckets[index].values[subject] = metric
		}
	}
	return accepted
}

// IsInternalSubject identifies NATS control-plane and request inbox traffic,
// which would otherwise overwhelm the application subject hierarchy.
func IsInternalSubject(subject string) bool {
	return strings.HasPrefix(subject, "$") || strings.HasPrefix(subject, "_INBOX.") || subject == "_INBOX"
}

func (t *TrafficTracker) Snapshot(now time.Time) TrafficSnapshot {
	cutoff := now.Add(-t.window).Unix()
	first := 0
	for first < len(t.buckets) && t.buckets[first].second < cutoff {
		first++
	}
	if first > 0 {
		t.buckets = append(t.buckets[:0], t.buckets[first:]...)
	}
	result := TrafficSnapshot{Window: t.window, Subjects: make(map[string]TrafficMetric), Observed: make(map[string]bool)}
	seconds := t.window.Seconds()
	if len(t.buckets) > 0 {
		seconds = max(now.Sub(time.Unix(t.buckets[0].second, 0)).Seconds(), 1)
	}
	for _, bucket := range t.buckets {
		for subject, value := range bucket.values {
			metric := result.Subjects[subject]
			metric.Messages += value.Messages
			metric.Bytes += value.Bytes
			result.Subjects[subject] = metric
		}
	}
	for subject, second := range t.observed {
		if second < cutoff {
			delete(t.observed, subject)
			continue
		}
		result.Observed[subject] = true
	}
	for subject, metric := range result.Subjects {
		metric.MessageRate = float64(metric.Messages) / seconds
		metric.ByteRate = float64(metric.Bytes) / seconds
		result.Subjects[subject] = metric
	}
	return result
}

func subjectHierarchy(subject string) []string {
	tokens := strings.Split(subject, ".")
	result := make([]string, 0, len(tokens))
	for index := range tokens {
		result = append(result, strings.Join(tokens[:index+1], "."))
	}
	return result
}
