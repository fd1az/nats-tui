package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	app "github.com/fd1az/nats-tui/internal/app/alerts"
)

type AlertServices struct {
	Load   func(app.Query) tea.Cmd
	Watch  func() tea.Cmd
	Stop   func()
	Ack    func(string) tea.Cmd
	AckAll func() tea.Cmd
	Scan   func() tea.Cmd
}

type alertState struct {
	records                                   []app.Record
	cursor, detailOffset                      int
	query, severity                           string
	search, detail, loading, watching, paused bool
	events                                    <-chan app.Record
	err                                       error
	notice                                    string
}
type alertsLoadedMsg struct{ records []app.Record }
type alertsFailedMsg struct{ err error }
type alertWatchStartedMsg struct{ events <-chan app.Record }
type alertWatchEventMsg struct {
	record app.Record
	events <-chan app.Record
}
type alertWatchEndedMsg struct{}
type alertAckedMsg struct {
	id        string
	all, scan bool
	count     int
}
type alertAckFailedMsg struct{ err error }

func AlertsLoaded(records []app.Record) tea.Msg          { return alertsLoadedMsg{records} }
func AlertsFailed(err error) tea.Msg                     { return alertsFailedMsg{err} }
func AlertWatchStarted(events <-chan app.Record) tea.Msg { return alertWatchStartedMsg{events} }
func AlertAcked(id string) tea.Msg                       { return alertAckedMsg{id: id} }
func AlertsAcked(count int) tea.Msg                      { return alertAckedMsg{all: true, count: count} }
func AlertsScanned(count int) tea.Msg                    { return alertAckedMsg{scan: true, count: count} }
func AlertActionFailed(err error) tea.Msg                { return alertAckFailedMsg{err} }

func (m Model) WithAlerts(services AlertServices) Model { m.alertServices = services; return m }

func waitAlertEvent(events <-chan app.Record) tea.Cmd {
	return func() tea.Msg {
		record, ok := <-events
		if !ok {
			return alertWatchEndedMsg{}
		}
		return alertWatchEventMsg{record, events}
	}
}

func (m Model) startAlerts() (tea.Model, tea.Cmd) {
	if m.alertServices.Load == nil {
		m.alerts.err = fmt.Errorf("alert inbox is unavailable")
		return m, nil
	}
	m.alerts.loading, m.alerts.err = true, nil
	commands := []tea.Cmd{m.alertServices.Load(app.Query{Limit: 1000})}
	if !m.alerts.watching && m.alertServices.Watch != nil {
		commands = append(commands, m.alertServices.Watch())
	}
	return m, tea.Batch(commands...)
}

func (m Model) displayedAlerts() []app.Record {
	query := strings.ToLower(strings.TrimSpace(m.alerts.query))
	result := make([]app.Record, 0, len(m.alerts.records))
	for _, record := range m.alerts.records {
		if m.alerts.severity != "" && record.Severity != m.alerts.severity {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{record.ID, record.Title, record.Description, record.Source, record.ResourcePrimary, record.ResourceSecondary, string(record.Payload)}, " "))
		if query == "" || strings.Contains(haystack, query) {
			result = append(result, record)
		}
	}
	return result
}

func (m Model) updateAlertsKey(key tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	s := &m.alerts
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
	items := m.displayedAlerts()
	switch key.String() {
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
		if len(items) > 0 {
			s.detail = !s.detail
			s.detailOffset = 0
		}
		return m, nil, true
	case "esc":
		s.detail = false
		return m, nil, true
	case " ":
		s.paused = !s.paused
		return m, nil, true
	case "v":
		switch s.severity {
		case "":
			s.severity = "critical"
		case "critical":
			s.severity = "warning"
		case "warning":
			s.severity = "info"
		default:
			s.severity = ""
		}
		s.cursor = 0
		return m, nil, true
	case "a":
		if len(items) > 0 && m.alertServices.Ack != nil {
			return m, m.alertServices.Ack(items[s.cursor].ID), true
		}
		return m, nil, true
	case "A":
		if m.alertServices.AckAll != nil {
			return m, m.alertServices.AckAll(), true
		}
		return m, nil, true
	case "r":
		model, command := m.startAlerts()
		return model, command, true
	case "s":
		if m.alertServices.Scan != nil {
			s.loading = true
			return m, m.alertServices.Scan(), true
		}
		return m, nil, true
	case "g":
		if len(items) > 0 {
			m.jumpFromAlert(items[s.cursor])
			return m, nil, true
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m *Model) jumpFromAlert(record app.Record) {
	switch record.ResourceKind {
	case "stream":
		for i, stream := range m.streams {
			if stream.Name == record.ResourcePrimary {
				m.streamCursor = i
				m.activeWorkspace = 1
				return
			}
		}
	case "consumer":
		for i, consumer := range m.consumers {
			if consumer.Stream == record.ResourcePrimary && consumer.Name == record.ResourceSecondary {
				m.consumerCursor = i
				m.activeWorkspace = 2
				return
			}
		}
	case "server":
		m.activeWorkspace = 8
		return
	case "connection":
		m.activeWorkspace = 9
		return
	}
	m.alerts.notice = "Affected resource is not present in the current snapshot"
}

func (m Model) alertsView(width, height int) string {
	s, items := m.alerts, m.displayedAlerts()
	heading := lipgloss.NewStyle().Foreground(m.theme.muted).Render("Dashboard  ›  Alerts")
	status := fmt.Sprintf("%d alerts", len(items))
	if s.watching {
		status += "  ● LIVE"
	}
	if s.paused {
		status += "  PAUSED"
	}
	if s.severity != "" {
		status += "  severity:" + s.severity
	}
	if s.query != "" {
		status += "  /" + s.query
	}
	statusLine := lipgloss.NewStyle().Foreground(m.theme.accent).Render(status)
	if s.loading {
		return heading + "\n\n" + m.spinner.View() + " Loading alert inbox…"
	}
	if s.err != nil {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.red).Render(s.err.Error())
	}
	if len(items) == 0 {
		return heading + "\n\n" + statusLine + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.secondary).Render("No alerts match this view. The live watch remains active.") + "\n\n" + alertHints(m)
	}
	s.cursor = min(s.cursor, len(items)-1)
	if s.detail {
		return heading + "\n\n" + m.alertDetail(items[s.cursor], width, height-4)
	}
	listWidth := width
	if width >= 105 {
		listWidth = width * 58 / 100
	}
	list := m.alertList(items, listWidth, height-5)
	if width < 105 {
		return heading + "\n" + statusLine + "\n" + list + "\n" + alertHints(m)
	}
	detail := m.alertDetail(items[s.cursor], max(width-listWidth-3, 30), height-5)
	divider := lipgloss.NewStyle().Foreground(m.theme.border).Render(strings.TrimSuffix(strings.Repeat("│\n", max(lipgloss.Height(list), lipgloss.Height(detail))), "\n"))
	return heading + "\n" + statusLine + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, list, " ", divider, " ", detail) + "\n" + alertHints(m)
}

func (m Model) alertList(items []app.Record, width, height int) string {
	lines := make([]string, 0, min(len(items), height))
	start := max(m.alerts.cursor-height+1, 0)
	end := min(start+height, len(items))
	for i := start; i < end; i++ {
		record := items[i]
		glyph := "●"
		color := m.theme.secondary
		if record.Severity == "critical" {
			color = m.theme.red
		} else if record.Severity == "warning" {
			color = m.theme.yellow
		}
		if record.Acknowledged {
			glyph = "○"
			color = m.theme.muted
		}
		resource := strings.Trim(strings.Join([]string{record.ResourcePrimary, record.ResourceSecondary}, "/"), "/")
		line := fmt.Sprintf("%s %-8s %-18s %s", glyph, strings.ToUpper(record.Severity), alertTruncate(resource, 18), alertTruncate(record.Title, max(width-34, 8)))
		style := lipgloss.NewStyle().Foreground(color)
		if i == m.alerts.cursor {
			style = style.Bold(true).Underline(true).Foreground(m.theme.accent)
		}
		lines = append(lines, style.Render(line))
	}
	return strings.Join(lines, "\n")
}

func (m Model) alertDetail(record app.Record, width, height int) string {
	payload := string(record.Payload)
	var pretty any
	if json.Unmarshal(record.Payload, &pretty) == nil {
		if encoded, err := json.MarshalIndent(pretty, "", "  "); err == nil {
			payload = string(encoded)
		}
	}
	resource := strings.Trim(strings.Join([]string{record.ResourcePrimary, record.ResourceSecondary}, " / "), " / ")
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.primary).Render(record.Title), lipgloss.NewStyle().Foreground(m.theme.secondary).Render(record.Description), "", fmt.Sprintf("Severity  %s", strings.ToUpper(record.Severity)), fmt.Sprintf("Category  %s", record.Category), fmt.Sprintf("Resource  %s", valueOr(resource, "-")), fmt.Sprintf("Subject   %s", record.Source), fmt.Sprintf("Occurred  %s", record.OccurredAt.Format("2006-01-02 15:04:05.000")), "", "Payload", payload}
	all := strings.Split(strings.Join(lines, "\n"), "\n")
	start := min(m.alerts.detailOffset, max(len(all)-height, 0))
	return lipgloss.NewStyle().Width(width).Foreground(m.theme.secondary).Render(strings.Join(all[start:min(start+height, len(all))], "\n"))
}

func alertHints(m Model) string {
	return lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ move  enter detail  / search  v severity  a ack  A ack all  g go to resource  s health scan  space pause")
}
func alertTruncate(value string, width int) string {
	if lipgloss.Width(value) <= width {
		return value
	}
	runes := []rune(value)
	if width <= 1 {
		return "…"
	}
	return string(runes[:min(len(runes), width-1)]) + "…"
}
