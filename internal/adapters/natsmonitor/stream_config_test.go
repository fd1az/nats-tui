package natsmonitor

import (
	"reflect"
	"testing"
	"time"

	"github.com/clcsolutions/nats-tui/internal/app/streams"
	"github.com/nats-io/nats.go/jetstream"
)

func TestStreamConfigMapsAdvancedTopologyWithoutLeakingNATSTypes(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	definition := streams.Definition{
		Name: "AGGREGATE", Storage: streams.StorageFile, Retention: streams.RetentionInterest,
		MaxMessages: 100, MaxBytes: 2048, MaxMessagesPerSubject: 10, MaxMessageSize: 512, MaxConsumers: 20,
		Replicas: 3, MaxAge: time.Hour, DuplicateWindow: time.Minute, Discard: streams.DiscardNew,
		Compression: true, AllowDirect: true, AllowRollup: true, DenyDelete: true, DenyPurge: true,
		PlacementCluster: "C1", PlacementTags: []string{"ssd"}, Metadata: map[string]string{"owner": "platform"},
		SubjectTransform: &streams.SubjectTransform{Source: "incoming.*", Destination: "stored.$1"},
		Republish:        &streams.Republish{Source: "stored.>", Destination: "audit.>", HeadersOnly: true},
		Sources:          []streams.Source{{Name: "ORDERS", StartTime: &started, FilterSubject: "orders.eu.>", External: &streams.External{APIPrefix: "$JS.EU.API", DeliverPrefix: "deliver.eu"}, SubjectTransforms: []streams.SubjectTransform{{Source: "orders.*", Destination: "eu.$1"}}}},
	}
	config := streamConfig(definition)
	if config.Compression != jetstream.S2Compression || config.Discard != jetstream.DiscardNew || config.Placement.Cluster != "C1" || !reflect.DeepEqual(config.Placement.Tags, []string{"ssd"}) {
		t.Fatalf("config=%+v", config)
	}
	if len(config.Sources) != 1 || config.Sources[0].Name != "ORDERS" || config.Sources[0].External.APIPrefix != "$JS.EU.API" || len(config.Sources[0].SubjectTransforms) != 1 {
		t.Fatalf("sources=%+v", config.Sources)
	}
	if config.SubjectTransform == nil || config.SubjectTransform.Destination != "stored.$1" || config.RePublish == nil || !config.RePublish.HeadersOnly {
		t.Fatalf("transform=%+v republish=%+v", config.SubjectTransform, config.RePublish)
	}
}
