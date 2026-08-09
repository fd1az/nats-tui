package connections

import (
	"testing"
	"time"
)

func TestQueryFiltersSortsAndDetectsSlowConsumers(t *testing.T) {
	t.Parallel()
	items := []Connection{{ServerID: "A", CID: 1, Name: "worker", User: "svc", IP: "10.0.0.2", Lang: "go", RTT: 4 * time.Millisecond, OutBytes: 10}, {ServerID: "B", CID: 2, Name: "api", User: "web", IP: "10.0.0.1", Lang: "rust", RTT: time.Millisecond, PendingBytes: 2 << 20}}
	got := Query{Filter: "web", Sort: SortPending, OnlySlow: true}.Apply(items)
	if len(got) != 1 || got[0].Name != "api" || !got[0].SlowConsumer() {
		t.Fatalf("got=%#v", got)
	}
	byRTT := Query{Filter: "1ms"}.Apply(items)
	if len(byRTT) != 1 || byRTT[0].Name != "api" {
		t.Fatalf("RTT filter=%#v", byRTT)
	}
}

func TestGroupUsesStableOperatorLabels(t *testing.T) {
	t.Parallel()
	items := []Connection{{Name: "", ServerName: "nats-a", Lang: "go"}, {Name: "api", ServerName: "nats-a", Lang: "go"}}
	groups := GroupBy(items, GroupName)
	if len(groups) != 2 || groups[0].Name != "(unnamed)" || groups[1].Name != "api" {
		t.Fatalf("groups=%#v", groups)
	}
}
