package natsmonitor

import (
	"testing"
	"time"
)

func TestMapConnectionsPreservesSubscriptionsAndSlowSignals(t *testing.T) {
	t.Parallel()
	wire := connectionWire{CID: 42, Name: "worker", User: "svc", RTT: "3ms", Pending: 2 << 20, Subs: []subscriptionWire{{Subject: "orders.*", Queue: "workers", SID: "7", Messages: 9}}}
	got := mapConnections("A", "nats-a", []connectionWire{wire})
	if len(got) != 1 || got[0].CID != 42 || got[0].RTT != 3*time.Millisecond || !got[0].SlowConsumer() || len(got[0].Subscriptions) != 1 || got[0].Subscriptions[0].Subject != "orders.*" {
		t.Fatalf("got=%#v", got)
	}
}
