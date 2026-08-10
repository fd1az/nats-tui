package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	app "github.com/clcsolutions/nats-tui/internal/app/bookmarks"
	"strings"
)

type BookmarkServices struct {
	Load       func([]app.TargetView) tea.Cmd
	Add        func(app.AddCommand) tea.Cmd
	Remove     func(string) tea.Cmd
	SaveSearch func(string, string) tea.Cmd
}
type bookmarkState struct {
	open, loading bool
	cursor        int
	items         []app.View
	err           error
	notice        string
	pending       *app.View
}
type bookmarksLoadedMsg struct{ items []app.View }
type bookmarksFailedMsg struct{ err error }
type bookmarkAddedMsg struct{ item app.View }
type bookmarkRemovedMsg struct{ id string }
type recentSearchSavedMsg struct{}

func BookmarksLoaded(items []app.View) tea.Msg { return bookmarksLoadedMsg{items} }
func BookmarksFailed(err error) tea.Msg        { return bookmarksFailedMsg{err} }
func BookmarkAdded(item app.View) tea.Msg      { return bookmarkAddedMsg{item} }
func BookmarkRemoved(id string) tea.Msg        { return bookmarkRemovedMsg{id} }
func RecentSearchSaved() tea.Msg               { return recentSearchSavedMsg{} }
func (m Model) WithBookmarks(services BookmarkServices) Model {
	m.bookmarkServices = services
	return m
}
func (m Model) bookmarkInventory() []app.TargetView {
	result := make([]app.TargetView, 0, len(m.streams)+len(m.consumers)+len(m.subjects)+len(m.keyValueBuckets)+len(m.objectBuckets))
	for _, v := range m.streams {
		result = append(result, app.TargetView{Kind: "stream", Primary: v.Name})
	}
	for _, v := range m.consumers {
		result = append(result, app.TargetView{Kind: "consumer", Primary: v.Stream, Secondary: v.Name})
	}
	for _, v := range m.subjects {
		result = append(result, app.TargetView{Kind: "subject", Primary: v.Subject})
	}
	for _, v := range m.keyValueBuckets {
		result = append(result, app.TargetView{Kind: "kv-bucket", Primary: v.Name})
	}
	for _, v := range m.objectBuckets {
		result = append(result, app.TargetView{Kind: "object-bucket", Primary: v.Name})
	}
	if m.browsingKeyValue {
		for _, key := range m.keyValueKeys {
			result = append(result, app.TargetView{Kind: "kv-key", Primary: m.keyValueBucket, Secondary: key})
		}
	}
	if m.browsingObjects {
		for _, object := range m.objects {
			result = append(result, app.TargetView{Kind: "object", Primary: m.objectBucket, Secondary: object.Name})
		}
	}
	return result
}
func (m Model) selectedBookmarkTarget() (app.AddCommand, bool) {
	switch m.activeWorkspace {
	case 1:
		if len(m.streams) > 0 {
			v := m.streams[m.streamCursor]
			return app.AddCommand{Kind: "stream", Primary: v.Name, Label: v.Name}, true
		}
	case 2:
		if len(m.consumers) > 0 {
			v := m.consumers[m.consumerCursor]
			return app.AddCommand{Kind: "consumer", Primary: v.Stream, Secondary: v.Name, Label: v.Name}, true
		}
	case 4:
		if m.browsingKeyValue && len(m.displayedKeyValueKeys()) > 0 {
			return app.AddCommand{Kind: "kv-key", Primary: m.keyValueBucket, Secondary: m.displayedKeyValueKeys()[m.keyValueKeyCursor], Label: m.displayedKeyValueKeys()[m.keyValueKeyCursor]}, true
		}
		if len(m.keyValueBuckets) > 0 {
			v := m.keyValueBuckets[m.keyValueCursor]
			return app.AddCommand{Kind: "kv-bucket", Primary: v.Name, Label: v.Name}, true
		}
	case 5:
		if m.browsingObjects && len(m.displayedObjects()) > 0 {
			v := m.displayedObjects()[m.objectObjectCursor]
			return app.AddCommand{Kind: "object", Primary: m.objectBucket, Secondary: v.Name, Label: v.Name}, true
		}
		if len(m.objectBuckets) > 0 {
			v := m.objectBuckets[m.objectCursor]
			return app.AddCommand{Kind: "object-bucket", Primary: v.Name, Label: v.Name}, true
		}
	case 6:
		nodes := m.displayedSubjects()
		if len(nodes) > 0 {
			v := nodes[m.subjectCursor]
			return app.AddCommand{Kind: "subject", Primary: v.Subject, Label: v.Subject}, true
		}
	}
	return app.AddCommand{}, false
}
func (m Model) toggleBookmark() (tea.Model, tea.Cmd) {
	target, ok := m.selectedBookmarkTarget()
	if !ok {
		m.bookmarks.notice = "This view has no bookmarkable selection"
		return m, nil
	}
	for _, item := range m.bookmarks.items {
		if item.Kind == target.Kind && item.Primary == target.Primary && item.Secondary == target.Secondary {
			if m.bookmarkServices.Remove == nil {
				m.bookmarks.err = fmt.Errorf("bookmark removal is unavailable")
				return m, nil
			}
			return m, m.bookmarkServices.Remove(item.ID)
		}
	}
	if m.bookmarkServices.Add == nil {
		m.bookmarks.err = fmt.Errorf("bookmark creation is unavailable")
		return m, nil
	}
	return m, m.bookmarkServices.Add(target)
}
func (m Model) updateBookmarks(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.bookmarks
	switch key.String() {
	case "esc", "b":
		s.open = false
		return m, nil
	case "up", "k":
		s.cursor = max(s.cursor-1, 0)
		return m, nil
	case "down", "j":
		s.cursor = min(s.cursor+1, max(len(s.items)-1, 0))
		return m, nil
	case "d", "B":
		if len(s.items) > 0 && m.bookmarkServices.Remove != nil {
			return m, m.bookmarkServices.Remove(s.items[s.cursor].ID)
		}
		return m, nil
	case "enter":
		if len(s.items) > 0 {
			return m.jumpBookmark(s.items[s.cursor])
		}
		return m, nil
	case "r":
		if m.bookmarkServices.Load != nil {
			s.loading = true
			return m, m.bookmarkServices.Load(m.bookmarkInventory())
		}
	}
	return m, nil
}
func (m Model) jumpBookmark(item app.View) (tea.Model, tea.Cmd) {
	if !item.Available {
		m.bookmarks.notice = "Resource is unavailable. Keep, retarget, or delete this bookmark."
		return m, nil
	}
	m.bookmarks.open = false
	switch item.Kind {
	case "stream":
		if err := m.selectStream(item.Primary); err != nil {
			return m.markBookmarkStale(item, err)
		}
	case "consumer":
		if err := m.selectConsumer(item.Primary, item.Secondary); err != nil {
			return m.markBookmarkStale(item, err)
		}
	case "subject":
		for i, node := range m.subjects {
			if node.Subject == item.Primary {
				m.subjectCursor = i
				m.activeWorkspace = 6
				return m, nil
			}
		}
		return m.markBookmarkStale(item, fmt.Errorf("subject no longer exists"))
	case "kv-bucket":
		for i, bucket := range m.keyValueBuckets {
			if bucket.Name == item.Primary {
				m.keyValueCursor = i
				m.activeWorkspace = 4
				return m, nil
			}
		}
		return m.markBookmarkStale(item, fmt.Errorf("KV bucket no longer exists"))
	case "kv-key":
		for _, bucket := range m.keyValueBuckets {
			if bucket.Name == item.Primary {
				if m.loadKeyValueKeys == nil {
					m.bookmarks.open = true
					m.bookmarks.notice = "KV key loading is unavailable"
					return m, nil
				}
				m.activeWorkspace = 4
				m.browsingKeyValue = true
				m.keyValueBucket = item.Primary
				m.bookmarks.pending = &item
				m.loadingKeyValueKeys = true
				return m, m.loadKeyValueKeys(item.Primary)
			}
		}
		return m.markBookmarkStale(item, fmt.Errorf("KV key parent bucket no longer exists"))
	case "object-bucket":
		for i, bucket := range m.objectBuckets {
			if bucket.Name == item.Primary {
				m.objectCursor = i
				m.activeWorkspace = 5
				return m, nil
			}
		}
		return m.markBookmarkStale(item, fmt.Errorf("object bucket no longer exists"))
	case "object":
		for _, bucket := range m.objectBuckets {
			if bucket.Name == item.Primary {
				if m.loadObjects == nil {
					m.bookmarks.open = true
					m.bookmarks.notice = "object loading is unavailable"
					return m, nil
				}
				m.activeWorkspace = 5
				m.browsingObjects = true
				m.objectBucket = item.Primary
				m.bookmarks.pending = &item
				m.loadingObjects = true
				return m, m.loadObjects(item.Primary)
			}
		}
		return m.markBookmarkStale(item, fmt.Errorf("object parent bucket no longer exists"))
	}
	return m, nil
}
func (m Model) markBookmarkStale(item app.View, err error) (tea.Model, tea.Cmd) {
	for i := range m.bookmarks.items {
		if m.bookmarks.items[i].ID == item.ID {
			m.bookmarks.items[i].Available = false
			m.bookmarks.items[i].Status = "stale"
		}
	}
	m.bookmarks.open = true
	m.bookmarks.notice = err.Error()
	return m, nil
}
func (m *Model) resolvePendingBookmark() {
	if m.bookmarks.pending == nil {
		return
	}
	item := *m.bookmarks.pending
	m.bookmarks.pending = nil
	switch item.Kind {
	case "kv-key":
		for i, key := range m.displayedKeyValueKeys() {
			if key == item.Secondary {
				m.keyValueKeyCursor = i
				return
			}
		}
		m.markPendingBookmarkStale(item, "Bookmarked key no longer exists")
	case "object":
		for i, object := range m.displayedObjects() {
			if object.Name == item.Secondary {
				m.objectObjectCursor = i
				return
			}
		}
		m.markPendingBookmarkStale(item, "Bookmarked object no longer exists")
	}
}

func (m *Model) markPendingBookmarkStale(item app.View, notice string) {
	for i := range m.bookmarks.items {
		if m.bookmarks.items[i].ID == item.ID {
			m.bookmarks.items[i].Available = false
			m.bookmarks.items[i].Status = "stale"
			break
		}
	}
	m.bookmarks.open = true
	m.bookmarks.notice = notice
}
func (m Model) bookmarksView(width, height int) string {
	cardWidth := min(max(width-14, 44), 100)
	lines := []string{}
	if m.bookmarks.loading {
		lines = append(lines, m.spinner.View()+" Loading bookmarks…")
	} else if len(m.bookmarks.items) == 0 {
		lines = append(lines, "No bookmarks yet.", "Select a resource and press B.")
	} else {
		for i, item := range m.bookmarks.items {
			marker := "  "
			style := lipgloss.NewStyle().Foreground(m.theme.secondary)
			if i == m.bookmarks.cursor {
				marker = "▸ "
				style = style.Foreground(m.theme.accent).Bold(true)
			}
			status := "●"
			if item.Status == "stale" {
				status = "×"
				style = style.Foreground(m.theme.red)
			} else if item.Status == "unchecked" {
				status = "?"
			}
			target := strings.Trim(strings.Join([]string{item.Primary, item.Secondary}, " / "), " / ")
			lines = append(lines, style.Render(fmt.Sprintf("%s%s  %-13s %-24s %s", marker, status, item.Kind, alertTruncate(item.Label, 24), target)))
		}
	}
	if m.bookmarks.err != nil {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.red).Render(m.bookmarks.err.Error()))
	}
	if m.bookmarks.notice != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.yellow).Render(m.bookmarks.notice))
	}
	content := lipgloss.NewStyle().Bold(true).Foreground(m.theme.primary).Render("Bookmarks") + "\n\n" + strings.Join(lines, "\n") + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑↓ select   enter jump   d delete   r resolve   b/esc close")
	card := lipgloss.NewStyle().Width(max(cardWidth-2, 1)).MaxHeight(max(height-6, 8)).Padding(1, 2).BorderStyle(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(content)
	return m.centeredCanvas(width, height, card)
}
