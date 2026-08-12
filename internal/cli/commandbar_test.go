package cli

import (
	"bytes"
	"context"
	"testing"

	app "github.com/fd1az/nats-tui/internal/app/commandbar"
	"github.com/fd1az/nats-tui/internal/config"
	domain "github.com/fd1az/nats-tui/internal/domain/commandbar"
)

type stubCommandBar struct {
	saved  domain.DefinitionInput
	values map[string]string
}

func (s *stubCommandBar) ListCustomCommands(context.Context, config.Settings) ([]app.Entry, error) {
	return nil, nil
}
func (s *stubCommandBar) SaveCustomCommand(_ context.Context, _ config.Settings, input domain.DefinitionInput) error {
	s.saved = input
	return nil
}
func (s *stubCommandBar) DeleteCustomCommand(context.Context, config.Settings, string) error {
	return nil
}
func (s *stubCommandBar) ListCommandHistory(context.Context, config.Settings, int) ([]app.HistoryView, error) {
	return nil, nil
}
func (s *stubCommandBar) PreviewCustomCommand(_ context.Context, _ config.Settings, _ string, values map[string]string) (app.PreviewView, error) {
	s.values = values
	return app.PreviewView{Title: "Watch orders", Command: "nats sub 'orders.>'"}, nil
}
func TestCommandsAddPreservesArgumentBoundariesAndPreviewValues(t *testing.T) {
	t.Parallel()
	service := new(stubCommandBar)
	root := NewRoot(Dependencies{Commands: service})
	root.SetArgs([]string{"commands", "add", "watch-orders", "--title", "Watch orders", "--action", "navigate.messages", "--parameter", "subject", "--required", "subject", "--argument", "subject={{subject}}", "--equivalent-part", "nats", "--equivalent-part", "sub", "--equivalent-part", "{{subject}}"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(service.saved.Equivalent) != 3 || service.saved.Equivalent[2] != "{{subject}}" || !service.saved.Parameters[0].Required {
		t.Fatalf("saved=%+v", service.saved)
	}
	output := new(bytes.Buffer)
	preview := NewRoot(Dependencies{Commands: service})
	preview.SetOut(output)
	preview.SetArgs([]string{"commands", "preview", "watch-orders", "--value", "subject=orders.>"})
	if err := preview.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.values["subject"] != "orders.>" || !bytes.Contains(output.Bytes(), []byte("nats sub")) {
		t.Fatalf("values=%v output=%s", service.values, output)
	}
}
