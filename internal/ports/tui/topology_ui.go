package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	app "github.com/fd1az/nats-tui/internal/app/topology"
	domain "github.com/fd1az/nats-tui/internal/domain/topology"
)

type LoadTopologyFunc func() tea.Cmd
type OperateTopologyFunc func(app.Command) tea.Cmd
type TopologyServices struct {
	Load    LoadTopologyFunc
	Operate OperateTopologyFunc
}
type topologyState struct {
	snapshot               domain.Snapshot
	cursor                 int
	graph, detail, loading bool
	err                    error
	notice                 string
	pending                *app.Command
}
type topologyLoadedMsg struct{ snapshot domain.Snapshot }
type topologyFailedMsg struct{ err error }
type topologyOperationSucceededMsg struct{ command app.Command }
type topologyOperationFailedMsg struct{ err error }

func TopologyLoaded(snapshot domain.Snapshot) tea.Msg { return topologyLoadedMsg{snapshot} }
func TopologyFailed(err error) tea.Msg                { return topologyFailedMsg{err} }
func TopologyOperationSucceeded(command app.Command) tea.Msg {
	return topologyOperationSucceededMsg{command}
}
func TopologyOperationFailed(err error) tea.Msg              { return topologyOperationFailedMsg{err} }
func (m Model) WithTopology(services TopologyServices) Model { m.topologyServices = services; return m }

func (m Model) startTopologyLoad() (tea.Model, tea.Cmd) {
	if m.topologyServices.Load == nil {
		m.topology.err = fmt.Errorf("topology service is unavailable")
		return m, nil
	}
	m.topology.loading, m.topology.err, m.topology.notice = true, nil, ""
	return m, m.topologyServices.Load()
}

func (m Model) updateTopologyKey(key tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	state := &m.topology
	if state.pending != nil {
		switch key.String() {
		case "y", "enter":
			command := *state.pending
			state.pending = nil
			if m.topologyServices.Operate == nil {
				state.err = fmt.Errorf("topology operations are unavailable")
				return m, nil, true
			}
			state.loading = true
			return m, m.topologyServices.Operate(command), true
		case "n", "esc":
			state.pending = nil
			return m, nil, true
		default:
			return m, nil, true
		}
	}
	if state.loading {
		return m, nil, true
	}
	servers := state.snapshot.Servers()
	switch key.String() {
	case "r":
		model, command := m.startTopologyLoad()
		return model, command, true
	case "up", "k":
		state.cursor = max(state.cursor-1, 0)
		return m, nil, true
	case "down", "j":
		state.cursor = min(state.cursor+1, max(len(servers)-1, 0))
		return m, nil, true
	case "g":
		state.graph = !state.graph
		return m, nil, true
	case "enter":
		state.detail = !state.detail
		return m, nil, true
	case "esc":
		state.detail = false
		return m, nil, true
	case "R":
		if len(servers) > 0 {
			command := app.Command{Action: app.ActionReload, ServerID: servers[state.cursor].ID().String()}
			state.pending = &command
		}
		return m, nil, true
	case "S":
		command := app.Command{Action: app.ActionStepDownMeta}
		state.pending = &command
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) topologyView(width, height int) string {
	state := m.topology
	if state.pending != nil {
		target := "JetStream meta leader"
		if state.pending.Action == app.ActionReload {
			target = "server " + state.pending.ServerID
		}
		return m.centeredCanvas(width, height, lipgloss.NewStyle().Width(min(max(width-12, 36), 72)).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.yellow).Render(lipgloss.NewStyle().Bold(true).Foreground(m.theme.yellow).Render("Confirm "+string(state.pending.Action))+"\n\nTarget: "+target+"\n\nThis operation changes live server state.\n\ny / enter confirm  •  n / esc cancel"))
	}
	if state.loading {
		return m.centeredCanvas(width, height, lipgloss.NewStyle().Foreground(m.theme.yellow).Render(m.spinner.View()+" discovering server topology…"))
	}
	if state.err != nil {
		return "Server Topology\n\n" + lipgloss.NewStyle().Foreground(m.theme.red).Render(state.err.Error()) + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render("Use system-account credentials or configure --monitor-url.  •  r retry")
	}
	servers := state.snapshot.Servers()
	if len(servers) == 0 {
		return "Server Topology\n\nNo topology loaded. Press r to discover servers."
	}
	heading := lipgloss.NewStyle().Bold(true).Foreground(m.theme.primary).Render("Server Topology") + "  " + lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("%d servers", len(servers)))
	meta := state.snapshot.Meta()
	if meta.Leader != "" {
		heading += "  " + lipgloss.NewStyle().Foreground(m.theme.mint).Render("META "+meta.Leader)
	}
	if state.notice != "" {
		heading += "  " + lipgloss.NewStyle().Foreground(m.theme.mint).Render("✓ "+state.notice)
	}
	body := m.topologyTable(width, height-4, servers)
	if state.graph {
		body = m.topologyGraph(width, servers, meta)
	}
	hint := lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ select  •  enter detail  •  g graph/table  •  r refresh  •  R reload server  •  S step down meta")
	return heading + "\n\n" + body + "\n\n" + hint
}

func (m Model) topologyTable(width, height int, servers []domain.Server) string {
	state := m.topology
	cursor := min(state.cursor, len(servers)-1)
	if width < 92 || state.detail {
		return m.topologyDetail(width, servers[cursor])
	}
	rows := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.muted).Render(fmt.Sprintf("  %-22s %-12s %-10s %7s %7s %10s", "SERVER", "CLUSTER", "VERSION", "CONNS", "SUBS", "JS USED"))}
	for index, server := range servers {
		marker, color := "  ", m.theme.secondary
		if index == cursor {
			marker, color = "› ", m.theme.accent
		}
		health := serverHealth(server)
		rows = append(rows, lipgloss.NewStyle().Foreground(color).Bold(index == cursor).Render(fmt.Sprintf("%s%-22s %-12s %-10s %7d %7d %10s  %s", marker, truncate(topologyDisplayName(server), 22), truncate(valueOr(server.Cluster(), "-"), 12), truncate(server.Version(), 10), server.Connections(), server.Subscriptions(), humanBytes(server.JetStream().Memory()+server.JetStream().Storage()), health)))
	}
	return strings.Join(rows, "\n")
}

func (m Model) topologyDetail(width int, server domain.Server) string {
	label := lipgloss.NewStyle().Bold(true).Foreground(m.theme.muted)
	value := lipgloss.NewStyle().Foreground(m.theme.primary)
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(topologyDisplayName(server)) + "  " + healthBadge(m, serverHealth(server)), label.Render("ID") + "  " + value.Render(server.ID().String()), label.Render("VERSION") + "  " + value.Render(server.Version()), label.Render("HOST") + "  " + value.Render(server.Host()), label.Render("CLUSTER / DOMAIN") + "  " + value.Render(valueOr(server.Cluster(), "-")+" / "+valueOr(server.Domain(), "-")), label.Render("UPTIME") + "  " + value.Render(server.Uptime().Round(1e9).String()), label.Render("CONNECTIONS / SUBSCRIPTIONS") + "  " + value.Render(fmt.Sprintf("%d / %d", server.Connections(), server.Subscriptions())), label.Render("CPU / MEMORY") + "  " + value.Render(fmt.Sprintf("%.1f%% / %s", server.CPU(), humanBytes(uint64(server.Memory())))), label.Render("JETSTREAM MEMORY / STORAGE") + "  " + value.Render(humanBytes(server.JetStream().Memory())+" / "+humanBytes(server.JetStream().Storage())), label.Render("ENDPOINTS") + "  " + value.Render(strings.Join(server.Endpoints(), ", ")), label.Render("TAGS") + "  " + value.Render(strings.Join(server.Tags(), ", ")), "", label.Render(fmt.Sprintf("LINKS  %d", len(server.Links())))}
	for _, link := range server.Links() {
		lines = append(lines, fmt.Sprintf("  %s %-10s → %-20s  rtt %-8s  pending %-8s  %s", linkGlyph(link.Kind()), link.Kind(), valueOr(link.RemoteName(), link.Remote().String()), link.RTT(), humanBytes(uint64(link.PendingBytes())), link.Health()))
	}
	return lipgloss.NewStyle().Width(max(width-2, 1)).MaxHeight(max(1, 24)).Render(strings.Join(lines, "\n"))
}

func (m Model) topologyGraph(width int, servers []domain.Server, meta domain.MetaGroup) string {
	lines := []string{lipgloss.NewStyle().Foreground(m.theme.muted).Render("Topology graph")}
	for _, server := range servers {
		leader := ""
		if server.Name() == meta.Leader {
			leader = " ★ META"
		}
		lines = append(lines, "", lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("● "+topologyDisplayName(server)+leader))
		if len(server.Links()) == 0 {
			lines = append(lines, "  └─ no routes, gateways, or leaves")
		}
		for _, link := range server.Links() {
			lines = append(lines, fmt.Sprintf("  ├─%s %s → %s  [%s]", linkGlyph(link.Kind()), link.Kind(), valueOr(link.RemoteName(), link.Remote().String()), link.Health()))
		}
	}
	return lipgloss.NewStyle().Width(max(width-2, 1)).Render(strings.Join(lines, "\n"))
}
func linkGlyph(kind domain.LinkKind) string {
	if kind == domain.LinkGateway {
		return "⇄"
	}
	if kind == domain.LinkLeaf {
		return "↳"
	}
	return "─"
}
func serverHealth(server domain.Server) domain.Health {
	result := domain.HealthHealthy
	for _, link := range server.Links() {
		if link.Health() == domain.HealthDown {
			return domain.HealthDown
		}
		if link.Health() == domain.HealthDegraded {
			result = domain.HealthDegraded
		}
	}
	return result
}

func topologyDisplayName(server domain.Server) string {
	name, id := server.Name(), server.ID().String()
	if name == id && len(name) > 12 {
		return "server-" + name[:8]
	}
	return name
}
func healthBadge(m Model, health domain.Health) string {
	color := m.theme.mint
	if health == domain.HealthDegraded {
		color = m.theme.yellow
	}
	if health == domain.HealthDown {
		color = m.theme.red
	}
	return lipgloss.NewStyle().Bold(true).Foreground(color).Render("● " + strings.ToUpper(string(health)))
}
