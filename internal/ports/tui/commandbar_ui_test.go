package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
)

func TestCommandBarFuzzyNavigationAndDestructivePreview(t *testing.T) {
	t.Parallel()
	model := NewModel("local", "nats://localhost:4222", nil)
	model.screen = screenWorkspace
	model.streams = []streams.Summary{{Name: "ORDERS", Messages: 42}}
	updated, _ := model.updateWorkspace(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	opened := updated.(Model)
	if !opened.commandBar.open {
		t.Fatal("command bar did not open")
	}
	for _, r := range "delete orders" {
		next, _ := opened.updateCommandBar(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		opened = next.(Model)
	}
	next, _ := opened.updateCommandBar(tea.KeyMsg{Type: tea.KeyEnter})
	opened = next.(Model)
	if opened.commandBar.stage != commandPreview || !opened.commandBar.preview.RequiresConfirmation || !strings.Contains(opened.commandBar.preview.Command, "nats stream rm ORDERS") {
		t.Fatalf("state=%+v", opened.commandBar)
	}
	view := opened.commandBarView(120, 32)
	if strings.Contains(view, "\x1b[48;") {
		t.Fatal("command bar uses ANSI background")
	}
}
func TestCommandBarDoesNotStealColonFromRequestPayload(t *testing.T) {
	t.Parallel()
	model := NewModel("local", "nats://localhost:4222", nil)
	model.screen = screenWorkspace
	model.activeWorkspace = 7
	model.requestReply.focus = 2
	model.focusRequestField()
	before := model.requestReply.payload.Value()
	updated, _ := model.updateWorkspace(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	result := updated.(Model)
	if result.commandBar.open || result.requestReply.payload.Value() == before {
		t.Fatal("colon was stolen from request editor")
	}
}
