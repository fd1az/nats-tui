package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/clcsolutions/nats-tui/internal/app/activity"
	"github.com/clcsolutions/nats-tui/internal/app/preferences"
)

type controlledRuntime struct{ now time.Time }

func (r *controlledRuntime) Now() time.Time                       { return r.now }
func (r *controlledRuntime) After(time.Duration) <-chan time.Time { return make(chan time.Time) }
func (r *controlledRuntime) Tick(_ time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	return func() tea.Msg { return fn(r.now) }
}

func TestFailureCreatesToastAndInspectableActivity(t *testing.T) {
	t.Parallel()
	runtime := &controlledRuntime{now: time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)}
	model := dashboardModel(t, 100, 30).WithRuntime(runtime)
	updated, _ := model.Update(MessagesFailed(errors.New("permission denied")))
	failed := updated.(Model)
	if view := failed.View(); !strings.Contains(view, "Operation failed") || !strings.Contains(view, "permission denied") {
		t.Fatalf("toast missing:\n%s", view)
	}
	opened, _ := failed.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
	activityModel := opened.(Model)
	if view := activityModel.View(); !strings.Contains(view, "Activity & errors") || !strings.Contains(view, "permission denied") {
		t.Fatalf("activity missing error:\n%s", view)
	}
	runtime.now = runtime.now.Add(toastLifetime + time.Second)
	activityModel.ux.open = false
	if strings.Contains(activityModel.View(), "Operation failed") {
		t.Fatal("expired toast still visible")
	}
}

func TestCustomGlobalBindingsAndAccessibleThemes(t *testing.T) {
	t.Parallel()
	keys := preferences.DefaultBindings()
	keys.Help = "h"
	keys.Refresh = "R"
	prefs, err := preferences.New(false, "accessible", keys)
	if err != nil {
		t.Fatal(err)
	}
	model := dashboardModel(t, 100, 30).WithUX(prefs)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if !updated.(Model).showHelp {
		t.Fatal("custom help binding did not open help")
	}
	if model.theme.accent == midnight().accent {
		t.Fatal("accessible theme was not applied")
	}
	mono, _ := preferences.New(false, "mono", preferences.Bindings{})
	model = model.WithUX(mono)
	if model.theme.accent != model.theme.primary || model.theme.red != model.theme.primary {
		t.Fatal("mono theme uses semantic colors")
	}
}

func TestWelcomeTimerCannotReplaceActiveWorkspace(t *testing.T) {
	t.Parallel()
	model := dashboardModel(t, 100, 30)
	updated, _ := model.Update(welcomeDoneMsg{})
	if updated.(Model).screen != screenWorkspace {
		t.Fatalf("screen = %v", updated.(Model).screen)
	}
}

func TestActivityLogCanBeClearedAndClosed(t *testing.T) {
	t.Parallel()
	model := dashboardModel(t, 100, 30)
	model.notify(activity.Error, "failed", "detail")
	model.ux.open = true
	cleared, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if len(cleared.(Model).ux.activity.Events()) != 0 {
		t.Fatal("activity was not cleared")
	}
	closed, _ := cleared.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if closed.(Model).ux.open {
		t.Fatal("activity was not closed")
	}
}

func TestSynchronousUIErrorIsRecorded(t *testing.T) {
	t.Parallel()
	model := NewModel("local", "nats://127.0.0.1:4222", nil)
	model.screen = screenConnection
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	failed := updated.(Model)
	events := failed.ux.activity.Events()
	if len(events) != 1 || !strings.Contains(events[0].Detail, "connection service") {
		t.Fatalf("events = %#v", events)
	}
}
