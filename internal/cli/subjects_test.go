package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/clcsolutions/nats-tui/internal/app/subjects"
	"github.com/clcsolutions/nats-tui/internal/config"
)

type subjectExplorerStub struct{ nodes []subjects.Node }

func (s subjectExplorerStub) ExploreSubjects(context.Context, config.Settings) ([]subjects.Node, error) {
	return s.nodes, nil
}

func TestSubjectListSupportsTableJSONAndPlainOutput(t *testing.T) {
	t.Parallel()
	service := subjectExplorerStub{nodes: []subjects.Node{{Subject: "events.payment", Depth: 1, Configured: true, Streams: []string{"EVENTS"}, Consumers: []string{"EVENTS/worker"}}}}
	for _, output := range []string{"table", "json", "plain"} {
		root := NewRoot(Dependencies{Subjects: service})
		buffer := new(bytes.Buffer)
		root.SetOut(buffer)
		root.SetArgs([]string{"subject", "list", "--output", output})
		if err := root.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("output %s: %v", output, err)
		}
		if !strings.Contains(buffer.String(), "events.payment") {
			t.Fatalf("output %s missing subject: %s", output, buffer)
		}
	}
}

func TestSubjectInspectReturnsEnrichedNode(t *testing.T) {
	t.Parallel()
	service := subjectExplorerStub{nodes: []subjects.Node{{Subject: "events.payment", Streams: []string{"EVENTS"}, MessageRate: 12.5}}}
	root := NewRoot(Dependencies{Subjects: service})
	buffer := new(bytes.Buffer)
	root.SetOut(buffer)
	root.SetArgs([]string{"subject", "inspect", "events.payment"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buffer.String(), `"MessageRate": 12.5`) || !strings.Contains(buffer.String(), "EVENTS") {
		t.Fatalf("inspect output=%s", buffer)
	}
}
