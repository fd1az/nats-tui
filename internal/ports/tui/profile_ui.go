package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	app "github.com/clcsolutions/nats-tui/internal/app/connections"
)

type ProfileServices struct{ Load func() tea.Cmd }
type connectionProfilesLoadedMsg struct{ items []app.ProfileSummary }
type connectionProfilesFailedMsg struct{ err error }

func ConnectionProfilesLoaded(items []app.ProfileSummary) tea.Msg {
	return connectionProfilesLoadedMsg{items}
}
func ConnectionProfilesFailed(err error) tea.Msg { return connectionProfilesFailedMsg{err} }
func (m Model) WithConnectionProfiles(services ProfileServices) Model {
	m.profileServices = services
	return m
}

func (m *Model) selectConnectionProfile(cursor int) {
	if cursor < 0 || cursor >= len(m.connectionProfiles) {
		return
	}
	m.connectionProfileCursor = cursor
	selected := m.connectionProfiles[cursor]
	m.profile = selected.Name
	m.serverInput.SetValue(strings.Join(selected.Servers, ","))
	m.serverInput.CursorEnd()
	m.useSelectedProfile = true
	m.connectionInputEdited = false
}

func (m Model) connectionProfilesView(width int) string {
	if m.connectionProfilesLoading {
		return m.spinner.View() + " loading saved profiles…"
	}
	if len(m.connectionProfiles) == 0 {
		return lipgloss.NewStyle().Foreground(m.theme.muted).Render("No saved profiles yet · use `nats-tui profile save` or connect directly")
	}
	limit := min(len(m.connectionProfiles), 4)
	rows := make([]string, 0, limit+1)
	rows = append(rows, lipgloss.NewStyle().Foreground(m.theme.muted).Render("RECENT PROFILES"))
	for i := 0; i < limit; i++ {
		item := m.connectionProfiles[i]
		marker := "  "
		color := m.theme.secondary
		if m.useSelectedProfile && i == m.connectionProfileCursor {
			marker = "› "
			color = m.theme.accent
		}
		server := ""
		if len(item.Servers) > 0 {
			server = item.Servers[0]
		}
		security := string(item.Auth)
		if item.TLS {
			security += "+tls"
		}
		if item.InsecureTLS {
			security += "!"
		}
		line := fmt.Sprintf("%s%-16s %-18s %s", marker, truncate(item.Name, 16), truncate(server, max(width-42, 12)), security)
		rows = append(rows, lipgloss.NewStyle().Foreground(color).Bold(m.useSelectedProfile && i == m.connectionProfileCursor).Render(line))
	}
	return strings.Join(rows, "\n")
}
