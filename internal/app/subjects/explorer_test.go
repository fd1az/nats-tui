package subjects

import (
	"reflect"
	"testing"

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
