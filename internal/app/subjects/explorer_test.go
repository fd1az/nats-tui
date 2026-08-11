package subjects

import (
	"reflect"
	"testing"
	"time"

	"github.com/clcsolutions/nats-tui/internal/app/consumers"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
)

func TestBuildCreatesSortedSubjectTreeWithWatchPatterns(t *testing.T) {
	t.Parallel()

	got := Build([]streams.Summary{
		{Name: "ORDERS", Subjects: []string{"orders.created", "orders.>"}, Messages: 42},
		{Name: "AUDIT", Subjects: []string{"audit.*"}, Messages: 7},
	})

	want := []Node{
		{Subject: "audit", WatchSubject: "audit.>", Depth: 0, Streams: []string{"AUDIT"}},
		{Subject: "audit.*", WatchSubject: "audit.*", Depth: 1, Configured: true, Streams: []string{"AUDIT"}},
		{Subject: "orders", WatchSubject: "orders.>", Depth: 0, Streams: []string{"ORDERS"}},
		{Subject: "orders.>", WatchSubject: "orders.>", Depth: 1, Configured: true, Streams: []string{"ORDERS"}},
		{Subject: "orders.created", WatchSubject: "orders.created", Depth: 1, Configured: true, Streams: []string{"ORDERS"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Build() = %#v, want %#v", got, want)
	}
}

func TestEnrichMergesObservedSubjectsOwnershipConsumersPermissionsAndHeat(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 11, 12, 0, 10, 0, time.UTC)
	tracker := NewTrafficTracker(10 * time.Second)
	tracker.Observe(
		TrafficSample{Subject: "events.payment.created", Bytes: 100, At: now.Add(-time.Second)},
		TrafficSample{Subject: "events.payment.created", Bytes: 50, At: now},
		TrafficSample{Subject: "events.user.login", Bytes: 20, At: now},
	)
	nodes := Enrich(
		[]streams.Summary{{Name: "EVENTS", Subjects: []string{"events.>"}}},
		[]consumers.Summary{{Stream: "EVENTS", Name: "payments", FilterSubjects: []string{"events.payment.*"}}},
		[]PrincipalPermissions{{Principal: "worker", Account: "A", PublishAllow: []string{"events.payment.>"}, SubscribeDeny: []string{"events.>"}}},
		tracker.Snapshot(now),
	)
	payment := nodeBySubject(t, nodes, "events.payment.created")
	if !payment.Observed || payment.MessageRate <= 0 || payment.ByteRate <= 0 || payment.Heat <= 0.5 {
		t.Fatalf("payment node=%+v", payment)
	}
	if !reflect.DeepEqual(payment.Streams, []string{"EVENTS"}) || !reflect.DeepEqual(payment.Consumers, []string{"EVENTS/payments"}) {
		t.Fatalf("ownership streams=%v consumers=%v", payment.Streams, payment.Consumers)
	}
	if len(payment.Permissions) != 2 {
		t.Fatalf("permissions=%+v", payment.Permissions)
	}
	branch := nodeBySubject(t, nodes, "events.payment")
	if branch.MessageRate != payment.MessageRate || branch.Observed {
		t.Fatalf("branch=%+v", branch)
	}
}

func TestTrafficTrackerExpiresWindowDeterministically(t *testing.T) {
	t.Parallel()
	tracker := NewTrafficTracker(10 * time.Second)
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	tracker.Observe(TrafficSample{Subject: "events.old", Bytes: 10, At: now})
	if got := tracker.Snapshot(now.Add(11 * time.Second)); len(got.Subjects) != 0 || len(got.Observed) != 0 {
		t.Fatalf("expired snapshot=%+v", got)
	}
}

func TestTrafficTrackerExcludesControlPlaneAndInboxNoise(t *testing.T) {
	t.Parallel()
	tracker := NewTrafficTracker(10 * time.Second)
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	accepted := tracker.Observe(
		TrafficSample{Subject: "$SYS.REQ.SERVER.PING", Bytes: 10, At: now},
		TrafficSample{Subject: "$JS.API.STREAM.LIST", Bytes: 10, At: now},
		TrafficSample{Subject: "_INBOX.random.reply", Bytes: 10, At: now},
		TrafficSample{Subject: "events.created", Bytes: 10, At: now},
	)
	if accepted != 1 {
		t.Fatalf("accepted=%d, want 1", accepted)
	}
	snapshot := tracker.Snapshot(now)
	if len(snapshot.Observed) != 1 || !snapshot.Observed["events.created"] {
		t.Fatalf("observed=%v", snapshot.Observed)
	}
}

func TestPatternsOverlap(t *testing.T) {
	t.Parallel()
	for _, pair := range [][2]string{{"events.*", "events.created"}, {"events.>", "events.eu.created"}, {"events.*.created", "events.eu.*"}} {
		if !PatternsOverlap(pair[0], pair[1]) {
			t.Fatalf("%q and %q should overlap", pair[0], pair[1])
		}
	}
	if PatternsOverlap("orders.*", "events.*") {
		t.Fatal("disjoint patterns overlap")
	}
}

func nodeBySubject(t *testing.T, nodes []Node, subject string) Node {
	t.Helper()
	for _, node := range nodes {
		if node.Subject == subject {
			return node
		}
	}
	t.Fatalf("subject %q not found", subject)
	return Node{}
}

func TestFilterMatchesSubjectAndOwningStreamCaseInsensitively(t *testing.T) {
	t.Parallel()

	nodes := Build([]streams.Summary{
		{Name: "PAYMENTS", Subjects: []string{"events.payment.success"}},
		{Name: "IDENTITY", Subjects: []string{"events.user.signup"}},
	})

	bySubject := Filter(nodes, "PAYMENT success")
	if len(bySubject) != 1 || bySubject[0].Subject != "events.payment.success" {
		t.Fatalf("subject filter = %#v", bySubject)
	}

	byStream := Filter(nodes, "identity signup")
	if len(byStream) != 1 || byStream[0].Subject != "events.user.signup" {
		t.Fatalf("stream filter = %#v", byStream)
	}
}
