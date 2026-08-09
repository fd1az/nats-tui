package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	appsubjects "github.com/clcsolutions/nats-tui/internal/app/subjects"
)

func (m *Model) rebuildSubjects() {
	m.subjects = appsubjects.Build(m.streams)
	m.applySubjectSearch()
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
	heading := lipgloss.NewStyle().Foreground(m.theme.muted).Render("Dashboard  ›  Subject Explorer")
	search := m.subjectSearchView(len(nodes))
	if len(m.subjects) == 0 {
		return heading + "\n" + search + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.secondary).Render("No configured subjects found. Create a stream to populate the explorer.")
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
	hint := lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ select  •  enter/w watch live  •  / search")
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
		configured = "  ● configured"
	}
	line := marker + strings.Repeat("  ", node.Depth) + branch + token + configured
	style := lipgloss.NewStyle().Foreground(m.theme.secondary)
	if selected {
		style = style.Foreground(m.theme.accent).Bold(true).Underline(true)
	} else if node.Configured {
		style = style.Foreground(m.theme.cyan)
	}
	return style.Render(truncate(line, width))
}

func (m Model) subjectDetailPane(node appsubjects.Node, width int) string {
	label := lipgloss.NewStyle().Foreground(m.theme.muted)
	value := lipgloss.NewStyle().Foreground(m.theme.secondary)
	kind := "branch"
	if node.Configured {
		kind = "configured pattern"
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("SUBJECT DETAIL"),
		"",
		label.Render("Subject:  ") + value.Render(truncate(node.Subject, max(width-11, 8))),
		label.Render("Type:     ") + value.Render(kind),
		label.Render("Streams:  ") + value.Render(truncate(strings.Join(node.Streams, ", "), max(width-11, 8))),
		"",
		label.Render("Live watch pattern"),
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.mint).Render(truncate(node.WatchSubject, width)),
		"",
		lipgloss.NewStyle().Foreground(m.theme.muted).Render("enter or w  →  open Live Watch"),
	}
	return strings.Join(lines, "\n")
}
