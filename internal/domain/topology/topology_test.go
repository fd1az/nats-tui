package topology

import (
	"errors"
	"testing"
	"time"
)

func TestNewServerRejectsMissingIdentity(t *testing.T) {
	t.Parallel()
	_, err := NewServer(ServerInput{Name: "nats-a"})
	if !errors.Is(err, ErrServerIDRequired) {
		t.Fatalf("error = %v", err)
	}
}

func TestNewLinkDerivesHealthFromConnectionAndPendingBytes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		connected bool
		pending   int64
		want      Health
	}{
		{name: "healthy", connected: true, want: HealthHealthy},
		{name: "backlogged", connected: true, pending: 12, want: HealthDegraded},
		{name: "disconnected", connected: false, want: HealthDown},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			link, err := NewLink(LinkRoute, "A", "B", "nats-b", test.connected, test.pending, 3*time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			if link.Health() != test.want {
				t.Fatalf("health = %q, want %q", link.Health(), test.want)
			}
		})
	}
}

func TestNewSnapshotOwnsServersAndRejectsDuplicateIdentities(t *testing.T) {
	t.Parallel()
	server, _ := NewServer(ServerInput{ID: "A", Name: "nats-a", ObservedAt: time.Now()})
	servers := []Server{server}
	snapshot, err := NewSnapshot(servers, MetaGroup{})
	if err != nil {
		t.Fatal(err)
	}
	servers[0] = Server{}
	if snapshot.Servers()[0].ID().String() != "A" {
		t.Fatal("snapshot did not own the server collection")
	}
	_, err = NewSnapshot([]Server{server, server}, MetaGroup{})
	if !errors.Is(err, ErrDuplicateServer) {
		t.Fatalf("duplicate error = %v", err)
	}
}
