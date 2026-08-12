package commandbar

import (
	"testing"

	domain "github.com/fd1az/nats-tui/internal/domain/commandbar"
)

func TestSearchRanksContiguousTitleBeforeLooseSubsequence(t *testing.T) {
	t.Parallel()
	values := []Entry{{ID: "consumer-reset", Title: "Reset consumer", Group: "Operate"}, {ID: "stream-events", Title: "Go to stream EVENTS", Group: "Navigate"}, {ID: "server-reload", Title: "Reload server", Group: "Operate"}}
	result := Search(values, "stream", 10)
	if len(result) < 1 || result[0].ID != "stream-events" {
		t.Fatalf("result=%+v", result)
	}
}

func TestPreviewExposesRiskAndLiteralEquivalentArguments(t *testing.T) {
	t.Parallel()
	definition, err := domain.NewDefinition(domain.DefinitionInput{ID: "delete", Title: "Delete stream", Action: "operate.stream.delete", Risk: domain.RiskDestructive, Parameters: []domain.ParameterInput{{Name: "stream", Required: true}}, Arguments: map[string]string{"stream": "{{stream}}"}, Equivalent: []string{"nats", "stream", "rm", "{{stream}}"}})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := Preview(definition, map[string]string{"stream": "ORDERS"})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.RequiresConfirmation || preview.Command != "nats stream rm ORDERS" {
		t.Fatalf("preview=%+v", preview)
	}
}
