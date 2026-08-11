package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	appoperator "github.com/clcsolutions/nats-tui/internal/app/operator"
	appsubjects "github.com/clcsolutions/nats-tui/internal/app/subjects"
)

const (
	subjectObservationBatchLimit    = 250
	subjectObservationBatchInterval = 250 * time.Millisecond
)

type subjectObservationStartedMsg struct {
	events <-chan appsubjects.TrafficSample
}
type subjectObservationFailedMsg struct{ err error }
type subjectObservationBatchMsg struct{ samples []appsubjects.TrafficSample }
type subjectObservationEndedMsg struct{}
type subjectTrafficTickMsg struct{}

func SubjectObservationStarted(events <-chan appsubjects.TrafficSample) tea.Msg {
	return subjectObservationStartedMsg{events: events}
}
func SubjectObservationFailed(err error) tea.Msg { return subjectObservationFailedMsg{err: err} }

func (m *Model) rebuildSubjects() {
	if m.subjectTraffic == nil {
		m.subjectTraffic = appsubjects.NewTrafficTracker(appsubjects.DefaultTrafficWindow)
	}
	m.subjects = appsubjects.Enrich(m.streams, m.consumers, subjectPermissionInputs(m.operator.catalog.Users()), m.subjectTraffic.Snapshot(m.runtime.Now()))
	m.applySubjectSearch()
}

func subjectPermissionInputs(users []appoperator.User) []appsubjects.PrincipalPermissions {
	result := make([]appsubjects.PrincipalPermissions, 0, len(users))
	for _, user := range users {
		principal := user.Name
		if principal == "" {
			principal = user.PublicKey
		}
		result = append(result, appsubjects.PrincipalPermissions{
			Principal: principal, Account: user.Account,
			PublishAllow: user.PublishAllow, PublishDeny: user.PublishDeny,
			SubscribeAllow: user.SubscribeAllow, SubscribeDeny: user.SubscribeDeny,
		})
	}
	return result
}

func (m Model) ensureSubjectObservation() (tea.Model, tea.Cmd) {
	if m.activeWorkspace != 6 || m.subjectObserving || m.subjectObservationStarting {
		return m, nil
	}
	if m.startSubjectObservation == nil {
		m.subjectObservationErr = errors.New("subject observation is unavailable")
		return m, nil
	}
	m.subjectObservationStarting, m.subjectObservationErr = true, nil
	commands := []tea.Cmd{m.spinner.Tick, m.startSubjectObservation()}
	if m.operatorServices.Load != nil && len(m.operator.catalog.Users()) == 0 && !m.operator.loading {
		m.operator.loading = true
		commands = append(commands, m.operatorServices.Load())
	}
	return m, tea.Batch(commands...)
}

func (m Model) waitForSubjectObservationBatch(events <-chan appsubjects.TrafficSample) tea.Cmd {
	return func() tea.Msg {
		first, ok := <-events
		if !ok {
			return subjectObservationEndedMsg{}
		}
		timer := m.runtime.After(subjectObservationBatchInterval)
		batch := []appsubjects.TrafficSample{first}
		for len(batch) < subjectObservationBatchLimit {
			select {
			case sample, open := <-events:
				if !open {
					return subjectObservationBatchMsg{samples: batch}
				}
				batch = append(batch, sample)
			case <-timer:
				return subjectObservationBatchMsg{samples: batch}
			}
		}
		return subjectObservationBatchMsg{samples: batch}
	}
}

func (m Model) scheduleSubjectTrafficTick() tea.Cmd {
	return m.runtime.Tick(time.Second, func(time.Time) tea.Msg { return subjectTrafficTickMsg{} })
}

func (m Model) composeFromSelectedSubject(request bool) (tea.Model, tea.Cmd) {
	nodes := m.displayedSubjects()
	if m.subjectCursor < 0 || m.subjectCursor >= len(nodes) {
		return m, nil
	}
	subject := nodes[m.subjectCursor].Subject
	if strings.ContainsAny(subject, "*>") {
		subject = strings.TrimSuffix(strings.TrimSuffix(subject, ".>"), ".*")
		m.requestReply.notice = "Complete the concrete subject before sending; wildcards cannot be published."
	} else {
		m.requestReply.notice = "Opened from Subject Explorer"
	}
	m.requestReply.requestMode = request
	m.requestReply.pane = requestCompose
	m.requestReply.focus = 0
	m.requestReply.error = nil
	m.requestReply.subject.SetValue(subject)
	m.requestReply.subject.Focus()
	m.activeWorkspace = 7
	return m, textinput.Blink
}

func (m Model) displayedSubjects() []appsubjects.Node {
	if strings.TrimSpace(m.subjectSearchQuery) == "" {
		return m.subjects
	}
	return m.filteredSubjects
}

func (m Model) updateSubjectSearch(key tea.KeyMsg) Model {
	switch key.String() {
	case "esc", "enter":
		m.subjectSearchMode = false
	case "backspace":
		runes := []rune(m.subjectSearchQuery)
		if len(runes) > 0 {
			m.subjectSearchQuery = string(runes[:len(runes)-1])
			m.applySubjectSearch()
		}
	case "ctrl+u":
		m.subjectSearchQuery = ""
		m.applySubjectSearch()
	default:
		if key.Type == tea.KeyRunes {
			m.subjectSearchQuery += string(key.Runes)
			m.applySubjectSearch()
		}
	}
	return m
}

func (m *Model) applySubjectSearch() {
	m.subjectCursor = 0
	if strings.TrimSpace(m.subjectSearchQuery) == "" {
		m.filteredSubjects = nil
		return
	}
	m.filteredSubjects = appsubjects.Filter(m.subjects, m.subjectSearchQuery)
}

func (m Model) startSelectedSubjectWatch() (tea.Model, tea.Cmd) {
	nodes := m.displayedSubjects()
	if len(nodes) == 0 || m.subjectCursor >= len(nodes) {
		return m, nil
	}
	if m.startWatch == nil {
		m.messageErr = errors.New("live watch is unavailable")
		return m, nil
	}
	if (m.watching || m.watchStarting) && m.stopWatch != nil {
		m.stopWatch()
	}
	pattern := nodes[m.subjectCursor].WatchSubject
	m.setMessages(nil)
	m.searchQuery = ""
	m.searchMode = false
	m.messageStream = pattern
	m.watchSubjects = []string{pattern}
	m.messageCursor = 0
	m.messageErr = nil
	m.showMessageDetail = false
	m.liveReceived = 0
	m.watching = false
	m.watchPaused = false
	m.watchStarting = true
	m.activeWorkspace = 3
	return m, tea.Batch(m.spinner.Tick, m.startWatch(pattern, []string{pattern}))
}

func selectedSubjectName(nodes []appsubjects.Node, cursor int) string {
	if cursor < 0 || cursor >= len(nodes) {
		return ""
	}
	return nodes[cursor].Subject
}

func findSubject(nodes []appsubjects.Node, subject string) int {
	for index, node := range nodes {
		if node.Subject == subject {
			return index
		}
	}
	return 0
}

func (m Model) subjectsView(width, height int) string {
	nodes := m.displayedSubjects()
	heading := lipgloss.NewStyle().Foreground(m.theme.muted).Render("Dashboard  ›  Subject Explorer") + "  " + m.subjectObservationStatus()
	search := m.subjectSearchView(len(nodes))
	if len(m.subjects) == 0 {
		empty := "No configured subjects found. Create a stream to populate the explorer."
		if m.subjectObserving {
			empty = "Watching account traffic… application subjects will appear here as they are observed."
		}
		return heading + "\n" + search + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.secondary).Render(empty)
	}
	if len(nodes) == 0 {
		return heading + "\n" + search + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.secondary).Render("No subjects match this search.")
	}

	paneHeight := max(height-3, 3)
	if width < 100 {
		return heading + "\n" + search + "\n\n" + m.subjectListPane(nodes, width, paneHeight)
	}
	leftWidth := width * 58 / 100
	rightWidth := max(width-leftWidth-3, 24)
	list := m.subjectListPane(nodes, leftWidth, paneHeight)
	detail := m.subjectDetailPane(nodes[m.subjectCursor], rightWidth)
	dividerHeight := max(lipgloss.Height(list), lipgloss.Height(detail))
	divider := lipgloss.NewStyle().Foreground(m.theme.border).Render(strings.TrimSuffix(strings.Repeat("│\n", dividerHeight), "\n"))
	return heading + "\n" + search + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, list, " ", divider, " ", detail)
}

func (m Model) subjectObservationStatus() string {
	switch {
	case m.subjectObservationStarting:
		return lipgloss.NewStyle().Foreground(m.theme.yellow).Render(m.spinner.View() + " STARTING OBSERVER")
	case m.subjectObserving:
		return lipgloss.NewStyle().Foreground(m.theme.mint).Render(fmt.Sprintf("● LIVE  %d events  •  %s window", m.subjectObservationReceived, appsubjects.DefaultTrafficWindow))
	case m.subjectObservationErr != nil:
		return lipgloss.NewStyle().Foreground(m.theme.red).Render("observer: " + m.subjectObservationErr.Error())
	default:
		return lipgloss.NewStyle().Foreground(m.theme.muted).Render("○ observation idle")
	}
}

func (m Model) subjectSearchView(resultCount int) string {
	if m.subjectSearchQuery == "" && !m.subjectSearchMode {
		return lipgloss.NewStyle().Foreground(m.theme.muted).Render("/ search subjects or streams")
	}
	caret := ""
	if m.subjectSearchMode {
		caret = lipgloss.NewStyle().Foreground(m.theme.accent).Render("▏")
	}
	count := lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("  %d matches", resultCount))
	return lipgloss.NewStyle().Foreground(m.theme.accent).Render("/ ") + m.subjectSearchQuery + caret + count
}

func (m Model) subjectListPane(nodes []appsubjects.Node, width, height int) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("SUBJECT TREE")
	count := lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("%d nodes", len(nodes)))
	header := title + strings.Repeat(" ", max(width-lipgloss.Width(title)-lipgloss.Width(count), 1)) + count
	capacity := max(height-3, 1)
	start, end := visibleWindow(len(nodes), m.subjectCursor, capacity)
	rows := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		rows = append(rows, m.subjectRow(nodes[index], index == m.subjectCursor, width))
	}
	hint := lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ select  •  enter/w watch  •  p publish  •  R request  •  / search")
	return strings.Join([]string{header, strings.Join(rows, "\n"), hint}, "\n")
}

func (m Model) subjectRow(node appsubjects.Node, selected bool, width int) string {
	marker := "  "
	if selected {
		marker = "› "
	}
	token := node.Subject
	if dot := strings.LastIndexByte(token, '.'); dot >= 0 {
		token = token[dot+1:]
	}
	branch := "├─ "
	if node.Depth == 0 {
		branch = "◆ "
	}
	configured := ""
	if node.Configured {
		configured = "  ◆ cfg"
	} else if node.Observed {
		configured = "  ● live"
	}
	rate := ""
	if node.MessageRate > 0 {
		rate = fmt.Sprintf("  %s %5.1f/s", subjectHeatGlyph(node.Heat), node.MessageRate)
	}
	line := marker + strings.Repeat("  ", node.Depth) + branch + token + configured + rate
	style := lipgloss.NewStyle().Foreground(m.theme.secondary)
	if selected {
		style = style.Foreground(m.theme.accent).Bold(true).Underline(true)
	} else if node.Heat >= 0.67 {
		style = style.Foreground(m.theme.red).Bold(true)
	} else if node.Heat >= 0.34 {
		style = style.Foreground(m.theme.yellow)
	} else if node.Configured {
		style = style.Foreground(m.theme.cyan)
	}
	return style.Render(truncate(line, width))
}

func subjectHeatGlyph(heat float64) string {
	switch {
	case heat >= 0.85:
		return "████"
	case heat >= 0.60:
		return "███░"
	case heat >= 0.30:
		return "██░░"
	case heat > 0:
		return "█░░░"
	default:
		return "····"
	}
}

func (m Model) subjectDetailPane(node appsubjects.Node, width int) string {
	label := lipgloss.NewStyle().Foreground(m.theme.muted)
	value := lipgloss.NewStyle().Foreground(m.theme.secondary)
	kind := "branch"
	if node.Configured {
		kind = "configured pattern"
	} else if node.Observed {
		kind = "observed live subject"
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("SUBJECT DETAIL"),
		"",
		label.Render("Subject:  ") + value.Render(truncate(node.Subject, max(width-11, 8))),
		label.Render("Type:     ") + value.Render(kind),
		label.Render("Streams:  ") + value.Render(truncate(strings.Join(node.Streams, ", "), max(width-11, 8))),
		label.Render("Consumers:") + " " + value.Render(truncate(valueOr(strings.Join(node.Consumers, ", "), "none"), max(width-12, 8))),
		label.Render("Traffic:  ") + value.Render(fmt.Sprintf("%.1f msg/s  •  %s  •  heat %.0f%%", node.MessageRate, humanBytes(uint64(node.ByteRate))+"/s", node.Heat*100)),
		"",
		label.Render("Live watch pattern"),
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.mint).Render(truncate(node.WatchSubject, width)),
		"",
		lipgloss.NewStyle().Foreground(m.theme.muted).Render("enter/w watch  •  p publish  •  R request"),
	}
	lines = append(lines, "", label.Render("PERMISSIONS AFFECTING THIS SUBJECT"))
	if len(node.Permissions) == 0 {
		lines = append(lines, value.Render("No visible managed-user rules."))
	} else {
		for index, rule := range node.Permissions {
			if index == 5 {
				lines = append(lines, label.Render(fmt.Sprintf("… and %d more", len(node.Permissions)-index)))
				break
			}
			color := m.theme.mint
			if rule.Effect == "deny" {
				color = m.theme.red
			}
			line := fmt.Sprintf("%s  %s %s  %s", rule.Principal, rule.Operation, rule.Effect, rule.Pattern)
			lines = append(lines, lipgloss.NewStyle().Foreground(color).Render(truncate(line, width)))
		}
	}
	return strings.Join(lines, "\n")
}
