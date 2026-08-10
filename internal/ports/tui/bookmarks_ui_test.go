package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	app "github.com/clcsolutions/nats-tui/internal/app/bookmarks"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
	"strings"
	"testing"
)

func TestBookmarkShortcutAddsSelectionAndGlobalListIsResponsive(t *testing.T) {
	t.Parallel()
	model := NewModel("local", "nats://localhost:4222", nil)
	model.screen = screenWorkspace
	model.activeWorkspace = 1
	model.streams = []streams.Summary{{Name: "ORDERS"}}
	model = model.WithBookmarks(BookmarkServices{Add: func(command app.AddCommand) tea.Cmd {
		return func() tea.Msg {
			return BookmarkAdded(app.View{ID: "b1", Kind: command.Kind, Primary: command.Primary, Label: command.Label, Status: "available", Available: true})
		}
	}})
	updated, command := model.updateWorkspace(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("B")})
	if command == nil {
		t.Fatal("bookmark command was not emitted")
	}
	updated, _ = updated.Update(command())
	result := updated.(Model)
	if len(result.bookmarks.items) != 1 || result.bookmarks.items[0].Primary != "ORDERS" {
		t.Fatalf("bookmarks=%+v", result.bookmarks.items)
	}
	result.bookmarks.open = true
	for _, width := range []int{64, 140} {
		view := result.bookmarksView(width, 24)
		if !strings.Contains(view, "ORDERS") || strings.Contains(view, "\x1b[48;") {
			t.Fatalf("width=%d view=%q", width, view)
		}
	}
}
func TestStaleBookmarkFailsClosedWithoutLosingEntry(t *testing.T) {
	t.Parallel()
	model := NewModel("local", "nats://localhost:4222", nil)
	model.bookmarks.open = true
	model.bookmarks.items = []app.View{{ID: "b1", Kind: "stream", Primary: "GONE", Label: "Gone", Status: "stale", Available: false}}
	updated, _ := model.updateBookmarks(tea.KeyMsg{Type: tea.KeyEnter})
	result := updated.(Model)
	if !result.bookmarks.open || len(result.bookmarks.items) != 1 || !strings.Contains(result.bookmarks.notice, "unavailable") {
		t.Fatalf("state=%+v", result.bookmarks)
	}
}
