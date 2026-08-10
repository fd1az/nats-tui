package commandbar

import (
	"testing"
	"time"
)

func TestDefinitionResolvesRequiredParametersWithoutShellParsing(t *testing.T) {
	t.Parallel()
	definition, err := NewDefinition(DefinitionInput{ID: "publish-order", Title: "Publish order", Action: "publish.core", Risk: RiskMutation, Parameters: []ParameterInput{{Name: "subject", Required: true}, {Name: "payload", Required: true}}, Arguments: map[string]string{"subject": "{{ subject }}", "payload": "{{payload}}"}, Equivalent: []string{"nats", "pub", "{{subject}}", "{{payload}}"}})
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := definition.Resolve(map[string]string{"subject": "orders.created; rm -rf nope", "payload": `{"id":42}`})
	if err != nil {
		t.Fatal(err)
	}
	if invocation.Argument("subject") != "orders.created; rm -rf nope" || len(invocation.Equivalent()) != 4 || invocation.Equivalent()[2] != "orders.created; rm -rf nope" {
		t.Fatalf("invocation=%#v", invocation)
	}
}

func TestDefinitionRejectsUnknownPlaceholderAndMissingRequiredValue(t *testing.T) {
	t.Parallel()
	_, err := NewDefinition(DefinitionInput{ID: "bad", Title: "Bad", Action: "navigate.stream", Arguments: map[string]string{"stream": "{{missing}}"}})
	if err == nil {
		t.Fatal("unknown placeholder accepted")
	}
	definition, err := NewDefinition(DefinitionInput{ID: "go-stream", Title: "Go stream", Action: "navigate.stream", Parameters: []ParameterInput{{Name: "stream", Required: true}}, Arguments: map[string]string{"stream": "{{stream}}"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = definition.Resolve(nil); err == nil {
		t.Fatal("missing required value accepted")
	}
}

func TestHistoryEntryProtectsStateTransition(t *testing.T) {
	t.Parallel()
	entry, err := NewHistoryEntry("h1", "go-stream", "Go stream", time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := entry.Complete(time.Date(2026, 8, 9, 12, 0, 1, 0, time.UTC), ""); err != nil {
		t.Fatal(err)
	}
	if entry.Status() != StatusSucceeded || entry.Duration() != time.Second {
		t.Fatalf("status=%s duration=%s", entry.Status(), entry.Duration())
	}
	if err := entry.Complete(time.Now(), ""); err == nil {
		t.Fatal("completed history transitioned twice")
	}
}
