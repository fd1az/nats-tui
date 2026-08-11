package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/clcsolutions/nats-tui/internal/app/consumers"
	"github.com/clcsolutions/nats-tui/internal/app/dashboard"
	"github.com/clcsolutions/nats-tui/internal/app/messages"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
)

func TestConsumerCreateFormIsCenteredHelpfulAndSubmits(t *testing.T) {
	var submitted ConsumerConfigInput
	model := dashboardModel(t, 120, 38).WithConsumerConfiguration(func(input ConsumerConfigInput) tea.Cmd {
		submitted = input
		return func() tea.Msg { return ConsumerConfigSucceeded(input) }
	})
	model.activeWorkspace = 2
	model.streams = []streams.Summary{{Name: "EVENTS"}}
	opened, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	form := opened.(Model)
	if form.consumerForm == nil {
		t.Fatal("consumer form did not open")
	}
	view := form.View()
	for _, want := range []string{"Create consumer", "Existing stream", "ctrl+s save"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	form.consumerForm.name = "worker"
	_, command := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if command == nil || submitted.Definition.Stream != "EVENTS" || submitted.Definition.Name != "worker" || submitted.Action != consumers.ConfigCreate {
		t.Fatalf("submitted=%+v command=%v", submitted, command)
	}
}

func TestConsumerEditFormPreservesAdvancedConfiguration(t *testing.T) {
	definition := consumers.DefaultDefinition("EVENTS", "worker")
	definition.FilterSubjects = []string{"events.>"}
	definition.Metadata = map[string]string{"team": "payments"}
	model := dashboardModel(t, 120, 38)
	model.activeWorkspace = 2
	model.consumers = []consumers.Summary{{Stream: "EVENTS", Name: "worker", Config: definition}}
	opened, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	form := opened.(Model)
	got, err := form.consumerForm.definition()
	if err != nil {
		t.Fatal(err)
	}
	if got.FilterSubjects[0] != "events.>" || got.Metadata["team"] != "payments" {
		t.Fatalf("definition=%+v", got)
	}
}

func TestConsumerFormAcceptsColonWithoutOpeningCommandBar(t *testing.T) {
	model := dashboardModel(t, 120, 38)
	model.activeWorkspace = 2
	form := newCreateConsumerForm([]streams.Summary{{Name: "EVENTS"}})
	form.field = 2
	model.consumerForm = &form
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	got := updated.(Model)
	if got.commandBar.open || got.consumerForm.description != ":" {
		t.Fatalf("command bar open=%v description=%q", got.commandBar.open, got.consumerForm.description)
	}
}

func TestConsumerMetricsAreBoundedAndComputeThroughput(t *testing.T) {
	model := dashboardModel(t, 120, 38)
	for index := 0; index < consumerHistoryLimit+10; index++ {
		model.applyRefresh(refreshedMsg{snapshot: dashboard.Snapshot{ObservedAt: time.Unix(int64(index+1), 0)}, consumers: []consumers.Summary{{Stream: "EVENTS", Name: "worker", Pending: uint64(index), DeliveredStream: uint64(index * 3)}}})
	}
	history := model.consumerHistory[consumerID{stream: "EVENTS", name: "worker"}]
	if len(history) != consumerHistoryLimit || history[len(history)-1].Rate != 3 {
		t.Fatalf("history length=%d last=%+v", len(history), history[len(history)-1])
	}
	trend, rate := consumerTrend(history, 12)
	if len([]rune(trend)) != 12 || rate != 3 {
		t.Fatalf("trend=%q rate=%f", trend, rate)
	}
}

func TestMessageMonitorCanSelectOrderedJetStreamWatch(t *testing.T) {
	called := false
	model := dashboardModel(t, 120, 38).WithOrderedMessageWatch(func(stream string, subjects []string) tea.Cmd {
		called = stream == "EVENTS" && len(subjects) == 1 && subjects[0] == "events.>"
		return func() tea.Msg { return WatchStarted(make(chan messages.Record)) }
	})
	model.activeWorkspace = 3
	model.messageStream = "EVENTS"
	model.streams = []streams.Summary{{Name: "EVENTS", Subjects: []string{"events.>"}}}
	selected, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	started, command := selected.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	if !started.(Model).orderedWatch || !started.(Model).watchStarting || !called || command == nil {
		t.Fatalf("ordered=%v starting=%v called=%v command=%v", started.(Model).orderedWatch, started.(Model).watchStarting, called, command)
	}
}
