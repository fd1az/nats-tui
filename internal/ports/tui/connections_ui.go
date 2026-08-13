package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	app "github.com/fd1az/nats-tui/internal/app/connections"
	appsubjects "github.com/fd1az/nats-tui/internal/app/subjects"
)

type LoadConnectionsFunc func() tea.Cmd
type CloseConnectionFunc func(string, uint64) tea.Cmd
type ConnectionServices struct {
	Load  LoadConnectionsFunc
	Close CloseConnectionFunc
}
type connectionSample struct {
	at                         time.Time
	connections, subscriptions int
}
type connectionsState struct {
	snapshot                          app.Snapshot
	cursor, subCursor                 int
	query                             string
	search, detail, loading, onlySlow bool
	sort                              app.Sort
	group                             app.Group
	err                               error
	notice                            string
	pending                           *app.Connection
	samples                           []connectionSample
}
type connectionsLoadedMsg struct{ snapshot app.Snapshot }
type connectionsFailedMsg struct{ err error }
type connectionClosedMsg struct {
	server string
	cid    uint64
}
type connectionCloseFailedMsg struct{ err error }

func ConnectionsLoaded(snapshot app.Snapshot) tea.Msg    { return connectionsLoadedMsg{snapshot} }
func ConnectionsFailed(err error) tea.Msg                { return connectionsFailedMsg{err} }
func ConnectionClosed(server string, cid uint64) tea.Msg { return connectionClosedMsg{server, cid} }
func ConnectionCloseFailed(err error) tea.Msg            { return connectionCloseFailedMsg{err} }
func (m Model) WithConnections(services ConnectionServices) Model {
	m.connectionServices = services
	m.connections.sort = app.SortName
	m.connections.group = app.GroupNone
	return m
}
func (m Model) startConnectionsLoad() (tea.Model, tea.Cmd) {
	if m.connectionServices.Load == nil {
		m.connections.err = fmt.Errorf("connection service is unavailable")
		return m, nil
	}
	m.connections.loading = true
	m.connections.err = nil
	return m, m.connectionServices.Load()
}
func (m Model) displayedConnections() []app.Connection {
	items := app.Query{Filter: m.connections.query, Sort: m.connections.sort, OnlySlow: m.connections.onlySlow}.Apply(m.connections.snapshot.Connections)
	if m.connections.group == app.GroupNone {
		return items
	}
	grouped := make([]app.Connection, 0, len(items))
	for _, group := range app.GroupBy(items, m.connections.group) {
		grouped = append(grouped, group.Connections...)
	}
	return grouped
}

func (m Model) updateConnectionsKey(key tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	s := &m.connections
	if s.pending != nil {
		switch key.String() {
		case "y", "enter":
			target := *s.pending
			s.pending = nil
			if m.connectionServices.Close == nil {
				s.err = fmt.Errorf("connection close is unavailable")
				return m, nil, true
			}
			s.loading = true
			return m, m.connectionServices.Close(target.ServerID, target.CID), true
		case "n", "esc":
			s.pending = nil
			return m, nil, true
		default:
			return m, nil, true
		}
	}
	if s.search {
		switch key.String() {
		case "esc", "enter":
			s.search = false
		case "backspace":
			r := []rune(s.query)
			if len(r) > 0 {
				s.query = string(r[:len(r)-1])
			}
		case "ctrl+u":
			s.query = ""
		default:
			if key.Type == tea.KeyRunes {
				s.query += string(key.Runes)
			}
		}
		s.cursor = 0
		return m, nil, true
	}
	if s.loading {
		return m, nil, true
	}
	items := m.displayedConnections()
	switch key.String() {
	case "r":
		model, cmd := m.startConnectionsLoad()
		return model, cmd, true
	case "/":
		s.search = true
		return m, nil, true
	case "up", "k":
		s.cursor = max(s.cursor-1, 0)
		return m, nil, true
	case "down", "j":
		s.cursor = min(s.cursor+1, max(len(items)-1, 0))
		return m, nil, true
	case "enter":
		s.detail = !s.detail
		s.subCursor = 0
		return m, nil, true
	case "esc":
		s.detail = false
		return m, nil, true
	case "o":
		s.sort = nextConnectionSort(s.sort)
		return m, nil, true
	case "G":
		s.group = nextConnectionGroup(s.group)
		return m, nil, true
	case "f":
		s.onlySlow = !s.onlySlow
		s.cursor = 0
		return m, nil, true
	case "[":
		s.subCursor = max(s.subCursor-1, 0)
		return m, nil, true
	case "]":
		if len(items) > 0 {
			s.subCursor = min(s.subCursor+1, max(len(items[s.cursor].Subscriptions)-1, 0))
		}
		return m, nil, true
	case "x":
		if len(items) > 0 {
			target := items[s.cursor]
			s.pending = &target
		}
		return m, nil, true
	case "s":
		if s.detail && len(items) > 0 && len(items[s.cursor].Subscriptions) > 0 {
			sub := items[s.cursor].Subscriptions[s.subCursor]
			m.activeWorkspace = 6
			m.subjectSearchQuery = sub.Subject
			m.applySubjectSearch()
			if len(m.filteredSubjects) == 0 {
				m.subjects = append(m.subjects, appsubjects.Node{Subject: sub.Subject, WatchSubject: sub.Subject, Depth: strings.Count(sub.Subject, "."), Configured: true})
				m.applySubjectSearch()
			}
			return m, nil, true
		}
	}
	return m, nil, false
}
func nextConnectionSort(current app.Sort) app.Sort {
	values := []app.Sort{app.SortName, app.SortUser, app.SortIP, app.SortLanguage, app.SortVersion, app.SortRTT, app.SortTraffic, app.SortPending}
	for i, v := range values {
		if v == current {
			return values[(i+1)%len(values)]
		}
	}
	return values[0]
}
func nextConnectionGroup(current app.Group) app.Group {
	values := []app.Group{app.GroupNone, app.GroupName, app.GroupUser, app.GroupServer, app.GroupLanguage}
	for i, v := range values {
		if v == current {
			return values[(i+1)%len(values)]
		}
	}
	return values[0]
}

func (m Model) connectionsView(width, height int) string {
	s := m.connections
	if s.pending != nil {
		return m.centeredCanvas(width, height, lipgloss.NewStyle().Width(min(max(width-12, 38), 72)).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.red).Render("Close live connection?\n\n"+fmt.Sprintf("%s:%d  %s", s.pending.ServerID, s.pending.CID, valueOr(s.pending.Name, "(unnamed)"))+"\n\nThe client will be disconnected immediately.\n\ny / enter confirm  •  n / esc cancel"))
	}
	if s.loading && len(s.snapshot.Connections) == 0 {
		return m.centeredCanvas(width, height, m.spinner.View()+" loading account connections…")
	}
	if s.err != nil && len(s.snapshot.Connections) == 0 {
		return "Connections & Subscriptions\n\n" + lipgloss.NewStyle().Foreground(m.theme.red).Render(s.err.Error()) + "\n\nUse system credentials or --monitor-url.  •  r retry"
	}
	items := m.displayedConnections()
	heading := lipgloss.NewStyle().Bold(true).Foreground(m.theme.primary).Render("Connections & Subscriptions") + "  " + lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("%d conns  %d subs", len(items), s.snapshot.SubscriptionCount()))
	if s.snapshot.SlowConsumerCount() > 0 {
		heading += "  " + lipgloss.NewStyle().Foreground(m.theme.yellow).Render(fmt.Sprintf("⚠ %d slow", s.snapshot.SlowConsumerCount()))
	}
	if s.notice != "" {
		heading += "  " + lipgloss.NewStyle().Foreground(m.theme.mint).Render("✓ "+s.notice)
	}
	search := ""
	if s.search || s.query != "" {
		caret := ""
		if s.search {
			caret = "█"
		}
		search = "\n/ " + s.query + caret
	}
	body := "No connections match the current filter."
	if len(items) > 0 {
		if s.detail || width < 88 {
			body = m.connectionDetail(width, items[min(s.cursor, len(items)-1)])
		} else {
			body = m.connectionTable(width, items)
		}
	}
	hint := lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("↑/↓ select • enter detail • / filter • o sort:%s • G group:%s • f slow • x close • [/] subscription • s → Subjects", s.sort, s.group))
	return heading + search + "\n" + connectionTrend(s.samples) + "\n\n" + body + "\n\n" + hint
}
func (m Model) connectionTable(width int, items []app.Connection) string {
	groups := app.GroupBy(items, m.connections.group)
	rows := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.muted).Render(fmt.Sprintf("  %-18s %-12s %-14s %-8s %8s %9s %5s", "NAME", "USER", "IP", "LANG", "RTT", "PENDING", "SUBS"))}
	selected := 0
	for _, group := range groups {
		if m.connections.group != app.GroupNone {
			rows = append(rows, lipgloss.NewStyle().Bold(true).Foreground(m.theme.cyan).Render("  "+group.Name+fmt.Sprintf("  (%d)", len(group.Connections))))
		}
		for _, item := range group.Connections {
			marker, color := "  ", m.theme.secondary
			if selected == m.connections.cursor {
				marker, color = "› ", m.theme.accent
			}
			slow := ""
			if item.SlowConsumer() {
				slow = " ⚠"
			}
			rows = append(rows, lipgloss.NewStyle().Foreground(color).Bold(selected == m.connections.cursor).Render(fmt.Sprintf("%s%-18s %-12s %-14s %-8s %8s %9s %5d%s", marker, truncate(valueOr(item.Name, "(unnamed)"), 18), truncate(valueOr(item.User, "-"), 12), truncate(item.IP, 14), truncate(valueOr(item.Lang, "-"), 8), item.RTT, humanBytes(nonNegativeBytes(item.PendingBytes)), len(item.Subscriptions), slow)))
			selected++
		}
	}
	return strings.Join(rows, "\n")
}
func (m Model) connectionDetail(width int, item app.Connection) string {
	label := lipgloss.NewStyle().Bold(true).Foreground(m.theme.muted)
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(valueOr(item.Name, "(unnamed)")) + fmt.Sprintf("  %s:%d", item.ServerID, item.CID), label.Render("USER / ACCOUNT") + "  " + valueOr(item.User, "-") + " / " + valueOr(item.Account, "-"), label.Render("CLIENT") + "  " + valueOr(item.Lang, "-") + " " + valueOr(item.Version, "-"), label.Render("IP / RTT") + "  " + item.IP + " / " + item.RTT.String(), label.Render("TRAFFIC IN / OUT") + "  " + humanBytes(nonNegativeBytes(item.InBytes)) + " / " + humanBytes(nonNegativeBytes(item.OutBytes)), label.Render("PENDING / STALLS") + "  " + humanBytes(nonNegativeBytes(item.PendingBytes)) + fmt.Sprintf(" / %d", item.Stalls), "", label.Render(fmt.Sprintf("SUBSCRIPTIONS  %d", len(item.Subscriptions)))}
	for i, sub := range item.Subscriptions {
		marker := "  "
		if i == m.connections.subCursor {
			marker = "› "
		}
		queue := ""
		if sub.Queue != "" {
			queue = "  queue=" + sub.Queue
		}
		lines = append(lines, marker+sub.Subject+queue+fmt.Sprintf("  delivered=%d", sub.Delivered))
	}
	return lipgloss.NewStyle().Width(max(width-2, 1)).Render(strings.Join(lines, "\n"))
}
func connectionTrend(samples []connectionSample) string {
	if len(samples) == 0 {
		return ""
	}
	start := max(len(samples)-24, 0)
	values := samples[start:]
	maxValue := 1
	for _, sample := range values {
		if sample.connections > maxValue {
			maxValue = sample.connections
		}
	}
	blocks := []rune("▁▂▃▄▅▆▇█")
	var graph strings.Builder
	for _, sample := range values {
		index := sample.connections * (len(blocks) - 1) / maxValue
		graph.WriteRune(blocks[index])
	}
	last := values[len(values)-1]
	return fmt.Sprintf("live %s  connections %d  subscriptions %d", graph.String(), last.connections, last.subscriptions)
}
