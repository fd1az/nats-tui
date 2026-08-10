package streams

import (
	"context"
	"time"
)

type Summary struct {
	Name                  string
	Description           string
	Subjects              []string
	Messages              uint64
	Bytes                 uint64
	Consumers             int
	Storage               Storage
	Retention             Retention
	Replicas              int
	MaxMessages           int64
	MaxBytes              int64
	MaxAge                time.Duration
	MaxMessageSize        int32
	MaxMessagesPerSubject int64
	DuplicateWindow       time.Duration
	MaxConsumers          int
	Discard               Discard
	DiscardNewPerSubject  bool
	Compression           bool
	AllowDirect           bool
	MirrorDirect          bool
	AllowRollup           bool
	DenyDelete            bool
	DenyPurge             bool
	Sealed                bool
	NoAck                 bool
	AllowMessageTTL       bool
	PlacementCluster      string
	PlacementTags         []string
	Metadata              map[string]string
	SubjectTransform      *SubjectTransform
	Republish             *Republish
	Mirror                *Source
	Sources               []Source
	Cluster               string
	Leader                string
	ReplicaHealth         []ReplicaHealth
	MirrorHealth          *SourceHealth
	SourceHealth          []SourceHealth
	FirstAt               time.Time
	LastAt                time.Time
}

type ReplicaHealth struct {
	Name             string
	Current, Offline bool
	Active           time.Duration
	Lag              uint64
}
type SourceHealth struct {
	Name   string
	Lag    uint64
	Active time.Duration
	Error  string
}

func (s Summary) Definition() Definition {
	return Definition{
		Name: s.Name, Description: s.Description, Subjects: append([]string(nil), s.Subjects...), Storage: s.Storage,
		Retention: s.Retention, MaxMessages: s.MaxMessages, MaxBytes: s.MaxBytes, Replicas: s.Replicas,
		MaxAge: s.MaxAge, MaxMessageSize: s.MaxMessageSize, MaxMessagesPerSubject: s.MaxMessagesPerSubject,
		DuplicateWindow: s.DuplicateWindow, MaxConsumers: s.MaxConsumers, Discard: s.Discard,
		DiscardNewPerSubject: s.DiscardNewPerSubject, Compression: s.Compression, AllowDirect: s.AllowDirect,
		MirrorDirect: s.MirrorDirect, AllowRollup: s.AllowRollup, DenyDelete: s.DenyDelete, DenyPurge: s.DenyPurge,
		Sealed: s.Sealed, NoAck: s.NoAck, AllowMessageTTL: s.AllowMessageTTL, PlacementCluster: s.PlacementCluster,
		PlacementTags: append([]string(nil), s.PlacementTags...), Metadata: cloneMetadata(s.Metadata),
		SubjectTransform: cloneSubjectTransform(s.SubjectTransform), Republish: cloneRepublish(s.Republish),
		Mirror: cloneSource(s.Mirror), Sources: cloneSources(s.Sources),
	}
}

func cloneMetadata(value map[string]string) map[string]string {
	if value == nil {
		return nil
	}
	result := make(map[string]string, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func cloneSubjectTransform(value *SubjectTransform) *SubjectTransform {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func cloneRepublish(value *Republish) *Republish {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func cloneSource(value *Source) *Source {
	if value == nil {
		return nil
	}
	result := *value
	result.SubjectTransforms = append([]SubjectTransform(nil), value.SubjectTransforms...)
	if value.StartTime != nil {
		started := *value.StartTime
		result.StartTime = &started
	}
	if value.External != nil {
		external := *value.External
		result.External = &external
	}
	return &result
}

func cloneSources(values []Source) []Source {
	result := make([]Source, 0, len(values))
	for i := range values {
		result = append(result, *cloneSource(&values[i]))
	}
	return result
}

type Catalog interface {
	ListStreams(context.Context) ([]Summary, error)
}

type ListStreamsHandler struct {
	catalog Catalog
}

func NewListStreamsHandler(catalog Catalog) ListStreamsHandler {
	if catalog == nil {
		panic("nil stream catalog")
	}
	return ListStreamsHandler{catalog: catalog}
}

func (h ListStreamsHandler) Handle(ctx context.Context) ([]Summary, error) {
	return h.catalog.ListStreams(ctx)
}
