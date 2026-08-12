package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/clcsolutions/nats-tui/internal/app/preferences"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
)

func mousePreferences(t *testing.T, enabled bool) preferences.Preferences {
	t.Helper()
	value, err := preferences.New(enabled, "midnight", preferences.Bindings{})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestMouseClickSwitchesWorkspaceOnlyWhenEnabled(t *testing.T) {
	t.Parallel()
	region := workspaceTabRegions(116)[2]
	click := tea.MouseMsg{X: 2 + region.start, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	disabled := dashboardModel(t, 120, 32).WithUX(mousePreferences(t, false))
	updated, _ := disabled.Update(click)
	if updated.(Model).activeWorkspace != 0 {
		t.Fatal("disabled mouse changed workspace")
	}
	enabled := dashboardModel(t, 120, 32).WithUX(mousePreferences(t, true))
	updated, _ = enabled.Update(click)
	if updated.(Model).activeWorkspace != 2 {
		t.Fatalf("workspace = %d", updated.(Model).activeWorkspace)
	}
}

func TestMouseWheelAndDoubleClickNavigateRows(t *testing.T) {
	t.Parallel()
	runtime := &controlledRuntime{now: time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)}
	model := dashboardModel(t, 120, 32).WithRuntime(runtime).WithUX(mousePreferences(t, true))
	model.activeWorkspace = 1
	model.streams = []streams.Summary{{Name: "ONE"}, {Name: "TWO"}, {Name: "THREE"}}
	updated, _ := model.Update(tea.MouseMsg{X: 10, Y: 10, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	scrolled := updated.(Model)
	if scrolled.streamCursor != 1 {
		t.Fatalf("cursor after wheel = %d", scrolled.streamCursor)
	}
	click := tea.MouseMsg{X: 10, Y: 8, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	selected, _ := scrolled.Update(click)
	opened, _ := selected.(Model).Update(click)
	if !opened.(Model).showStreamDetail {
		t.Fatal("double click did not open selected stream")
	}
}

func TestMouseClickSelectsOperationalRows(t *testing.T) {
	t.Parallel()
	model := dashboardModel(t, 120, 32).WithUX(mousePreferences(t, true))
	model.streams = []streams.Summary{{Name: "ONE"}, {Name: "TWO"}}
	model.activeWorkspace = 1
	updated, _ := model.Update(tea.MouseMsg{X: 10, Y: 9, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if updated.(Model).streamCursor != 1 {
		t.Fatalf("stream cursor = %d", updated.(Model).streamCursor)
	}
}
