package tui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/clcsolutions/nats-tui/internal/app/dashboard"
)

type DashboardServices struct{ Load func(string) tea.Cmd }
type dashboardPanelsLoadedMsg struct{ panels []dashboard.Panel }
type dashboardPanelsFailedMsg struct{ err error }

func DashboardPanelsLoaded(panels []dashboard.Panel) tea.Msg {
	return dashboardPanelsLoadedMsg{panels: append([]dashboard.Panel(nil), panels...)}
}
func DashboardPanelsFailed(err error) tea.Msg {
	if err == nil {
		err = errors.New("load dashboard panels")
	}
	return dashboardPanelsFailedMsg{err: err}
}
func (m Model) WithDashboard(services DashboardServices) Model {
	m.dashboardServices = services
	return m
}
