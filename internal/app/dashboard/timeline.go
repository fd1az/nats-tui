package dashboard

import "time"

const DefaultSampleLimit = 120

type Sample struct {
	At             time.Time
	MessagesPerSec float64
	BytesPerSec    float64
	CPUPercent     float64
	ServerMemory   uint64
	Connections    int
	JetStreamBytes uint64
	MemoryStorage  uint64
	FileStorage    uint64
	Streams        int
	Consumers      int
	SlowConsumers  int64
}

type Timeline struct {
	limit    int
	samples  []Sample
	previous *Snapshot
}

func NewTimeline(limit int) *Timeline {
	if limit < 1 {
		panic("dashboard sample limit must be positive")
	}
	return &Timeline{limit: limit}
}

func (t *Timeline) Record(snapshot Snapshot) Sample {
	sample := Sample{
		At: snapshot.ObservedAt, CPUPercent: snapshot.Runtime.CPUPercent,
		ServerMemory: snapshot.Runtime.MemoryBytes, Connections: snapshot.Runtime.Connections,
		JetStreamBytes: snapshot.Bytes, MemoryStorage: snapshot.MemoryBytes, FileStorage: snapshot.FileBytes,
		Streams: snapshot.Streams, Consumers: snapshot.Consumers, SlowConsumers: snapshot.Runtime.SlowConsumers,
	}
	if t.previous != nil {
		seconds := snapshot.ObservedAt.Sub(t.previous.ObservedAt).Seconds()
		messages, bytes := snapshot.TrafficMessages(), snapshot.TrafficBytes()
		previousMessages, previousBytes := t.previous.TrafficMessages(), t.previous.TrafficBytes()
		if seconds > 0 && messages >= previousMessages {
			sample.MessagesPerSec = float64(messages-previousMessages) / seconds
		}
		if seconds > 0 && bytes >= previousBytes {
			sample.BytesPerSec = float64(bytes-previousBytes) / seconds
		}
	}
	previous := snapshot
	t.previous = &previous
	t.samples = append(t.samples, sample)
	if overflow := len(t.samples) - t.limit; overflow > 0 {
		copy(t.samples, t.samples[overflow:])
		t.samples = t.samples[:t.limit]
	}
	return sample
}

func (t *Timeline) Samples() []Sample { return append([]Sample(nil), t.samples...) }

func (t *Timeline) Reset() {
	t.samples = nil
	t.previous = nil
}
