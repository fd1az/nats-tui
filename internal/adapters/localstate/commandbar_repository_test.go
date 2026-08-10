package localstate

import (
	"context"
	"testing"
	"time"

	domain "github.com/clcsolutions/nats-tui/internal/domain/commandbar"
)

func TestCommandBarRepositoryRoundTripAndBoundsHistory(t *testing.T) {
	t.Parallel()
	file := new(memoryStateFile)
	repository := NewCommandBarRepository(file, 2)
	definition, err := domain.NewDefinition(domain.DefinitionInput{ID: "watch-orders", Title: "Watch orders", Action: "navigate.messages", Custom: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveDefinition(context.Background(), definition); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"h1", "h2", "h3"} {
		entry, _ := domain.NewHistoryEntry(id, "watch-orders", "Watch orders", time.Date(2026, 8, 9, 12, 0, i, 0, time.UTC))
		_ = entry.Complete(entry.StartedAt().Add(time.Second), "")
		if err := repository.AppendHistory(context.Background(), entry); err != nil {
			t.Fatal(err)
		}
	}
	definitions, err := repository.ListDefinitions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	history, err := repository.ListHistory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || definitions[0].ID() != "watch-orders" || len(history) != 2 || history[0].ID() != "h2" {
		t.Fatalf("definitions=%v history=%v", definitions, history)
	}
}
