package natsmonitor

import (
	"testing"
)

func TestDecodeTopologyServerMapsIdentityResourcesAndLinks(t *testing.T) {
	t.Parallel()
	stats := statsResponse{}
	stats.Server.ID, stats.Server.Name, stats.Server.Cluster, stats.Server.Version = "A", "nats-a", "C1", "2.14.4"
	stats.Stats.ActiveServers = 2
	stats.Stats.JetStream = &jetStreamWire{Memory: 10, Store: 20}
	details := serverDetails{
		Varz:   varzWire{ID: "A", Name: "nats-a", Host: "127.0.0.1", Port: 4222, Connections: 4, Subscriptions: 9},
		Routes: routezWire{Routes: []routeWire{{RemoteID: "B", RemoteName: "nats-b", RTT: "2ms", Pending: 3}}},
		Leafs:  leafzWire{Leafs: []leafWire{{ID: 7, Name: "edge", RTT: "4ms"}}},
	}
	server, err := mapTopologyServer(stats, details)
	if err != nil {
		t.Fatal(err)
	}
	if server.ID().String() != "A" || server.Connections() != 4 || server.JetStream().Storage() != 20 {
		t.Fatalf("server=%#v", server)
	}
	links := server.Links()
	if len(links) != 2 || links[0].PendingBytes() != 3 || links[1].RemoteName() != "edge" {
		t.Fatalf("links=%#v", links)
	}
}
