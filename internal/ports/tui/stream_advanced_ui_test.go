package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fd1az/nats-tui/internal/app/streams"
)

func TestAdvancedStreamFormPreservesTopologyAndFitsViewport(t *testing.T) {
	t.Parallel()
	summary := streams.Summary{Name: "ORDERS", Subjects: []string{"orders.>"}, Storage: streams.StorageFile, Retention: streams.RetentionLimits, MaxMessages: -1, MaxBytes: -1, MaxMessagesPerSubject: -1, MaxMessageSize: -1, MaxConsumers: -1, Replicas: 1, AllowDirect: true, Sources: []streams.Source{{Name: "EU", FilterSubject: "orders.eu.>"}}}
	form := newEditStreamForm(summary)
	definition, err := form.definition()
	if err != nil {
		t.Fatal(err)
	}
	if !definition.AllowDirect || len(definition.Sources) != 1 || definition.Sources[0].FilterSubject != "orders.eu.>" {
		t.Fatalf("definition=%+v", definition)
	}
	model := NewModel("local", "nats://localhost:4222", nil)
	model.streamForm = &form
	for _, size := range [][2]int{{58, 22}, {140, 40}} {
		view := model.streamFormView(size[0], size[1])
		if strings.Contains(view, "\x1b[48;") || lipglossHeight(view) > size[1] {
			t.Fatalf("size=%v height=%d", size, lipglossHeight(view))
		}
	}
}

func TestStreamPurgeRequiresDoubleConfirmation(t *testing.T) {
	t.Parallel()
	called := false
	model := NewModel("local", "nats://localhost:4222", nil).WithStreamPurge(func(StreamPurgeInput) tea.Cmd { called = true; return func() tea.Msg { return nil } })
	form := streamPurgeForm{stream: "ORDERS", sequence: "0", keep: "10"}
	model.streamPurgeForm = &form
	updated, command := model.submitStreamPurge()
	first := updated.(Model)
	if command != nil || called || !first.streamPurgeForm.confirmed {
		t.Fatal("first submit did not stop for confirmation")
	}
	updated, command = first.submitStreamPurge()
	if command == nil || !called {
		t.Fatal("confirmed purge did not start")
	}
	_ = updated
}

func lipglossHeight(value string) int { return strings.Count(value, "\n") + 1 }
