package tui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
)

const streamFormFieldCount = 8

type streamForm struct {
	action      streams.Action
	field       int
	name        string
	description string
	subjects    string
	storage     streams.Storage
	retention   streams.Retention
	maxMessages string
	maxBytes    string
	replicas    string
	err         error
}

func newCreateStreamForm() streamForm {
	return streamForm{action: streams.ActionCreate, storage: streams.StorageFile, retention: streams.RetentionLimits, maxMessages: "-1", maxBytes: "-1", replicas: "1"}
}

func newEditStreamForm(stream streams.Summary) streamForm {
	return streamForm{
		action: streams.ActionUpdate, field: 1, name: stream.Name, description: stream.Description,
		subjects: strings.Join(stream.Subjects, ", "), storage: stream.Storage, retention: stream.Retention,
		maxMessages: strconv.FormatInt(stream.MaxMessages, 10), maxBytes: strconv.FormatInt(stream.MaxBytes, 10), replicas: strconv.Itoa(stream.Replicas),
	}
}

func (f streamForm) definition() (streams.Definition, error) {
	maxMessages, err := strconv.ParseInt(strings.TrimSpace(f.maxMessages), 10, 64)
	if err != nil {
		return streams.Definition{}, fmt.Errorf("max messages must be an integer")
	}
	maxBytes, err := strconv.ParseInt(strings.TrimSpace(f.maxBytes), 10, 64)
	if err != nil {
		return streams.Definition{}, fmt.Errorf("max bytes must be an integer")
	}
	replicas, err := strconv.Atoi(strings.TrimSpace(f.replicas))
	if err != nil {
		return streams.Definition{}, fmt.Errorf("replicas must be an integer")
	}
	definition := streams.Definition{
		Name: strings.TrimSpace(f.name), Description: strings.TrimSpace(f.description), Subjects: splitSubjects(f.subjects),
		Storage: f.storage, Retention: f.retention, MaxMessages: maxMessages, MaxBytes: maxBytes, Replicas: replicas,
	}
	if err := definition.Validate(); err != nil {
		return streams.Definition{}, err
	}
	return definition, nil
}

func splitSubjects(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		subject := strings.TrimSpace(part)
		if subject == "" {
			continue
		}
		if _, exists := seen[subject]; exists {
			continue
		}
		seen[subject] = struct{}{}
		result = append(result, subject)
	}
	return result
}

func (m Model) updateStreamForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.streamActionRunning {
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.streamForm = nil
		m.streamActionErr = nil
		return m, nil
	case "ctrl+s":
		return m.submitStreamForm()
	case "tab", "down":
		m.moveStreamFormField(1)
		return m, nil
	case "shift+tab", "up":
		m.moveStreamFormField(-1)
		return m, nil
	case "left", "right", " ":
		if m.streamForm.field == 3 && m.streamForm.action == streams.ActionCreate {
			m.toggleStreamStorage()
			return m, nil
		}
		if m.streamForm.field == 4 {
			m.cycleStreamRetention(key.String() == "left")
			return m, nil
		}
	case "backspace":
		m.editStreamField(true, "")
		return m, nil
	}
	if key.Type == tea.KeyRunes {
		m.editStreamField(false, string(key.Runes))
	}
	return m, nil
}

func (m *Model) moveStreamFormField(step int) {
	for {
		m.streamForm.field = (m.streamForm.field + step + streamFormFieldCount) % streamFormFieldCount
		if m.streamForm.action == streams.ActionCreate || (m.streamForm.field != 0 && m.streamForm.field != 3) {
			return
		}
	}
}

func (m Model) submitStreamForm() (tea.Model, tea.Cmd) {
	if m.streamAction == nil {
		m.streamActionErr = errors.New("stream operations are unavailable")
		return m, nil
	}
	definition, err := m.streamForm.definition()
	if err != nil {
		m.streamActionErr = err
		return m, nil
	}
	input := StreamActionInput{Generation: m.refreshGeneration, Action: m.streamForm.action, Name: definition.Name, Definition: definition}
	m.streamActionRunning = true
	m.streamActionErr = nil
	m.streamActionNotice = ""
	return m, m.streamAction(input)
}

func (m *Model) editStreamField(backspace bool, addition string) {
	field := m.streamTextField()
	if field == nil || (m.streamForm.action == streams.ActionUpdate && m.streamForm.field == 0) {
		return
	}
	if backspace {
		runes := []rune(*field)
		if len(runes) > 0 {
			*field = string(runes[:len(runes)-1])
		}
		return
	}
	*field += addition
}

func (m *Model) streamTextField() *string {
	switch m.streamForm.field {
	case 0:
		return &m.streamForm.name
	case 1:
		return &m.streamForm.description
	case 2:
		return &m.streamForm.subjects
	case 5:
		return &m.streamForm.maxMessages
	case 6:
		return &m.streamForm.maxBytes
	case 7:
		return &m.streamForm.replicas
	default:
		return nil
	}
}

func (m *Model) toggleStreamStorage() {
	if m.streamForm.storage == streams.StorageFile {
		m.streamForm.storage = streams.StorageMemory
		return
	}
	m.streamForm.storage = streams.StorageFile
}

func (m *Model) cycleStreamRetention(reverse bool) {
	values := []streams.Retention{streams.RetentionLimits, streams.RetentionInterest, streams.RetentionWorkQueue}
	index := 0
	for candidate, value := range values {
		if value == m.streamForm.retention {
			index = candidate
		}
	}
	step := 1
	if reverse {
		step = -1
	}
	m.streamForm.retention = values[(index+step+len(values))%len(values)]
}

func (m Model) streamFormView(width, height int) string {
	title := "Create stream"
	if m.streamForm.action == streams.ActionUpdate {
		title = "Edit stream"
	}
	nameLabel := "Name"
	storageLabel := "Storage"
	if m.streamForm.action == streams.ActionUpdate {
		nameLabel = "Name (immutable)"
		storageLabel = "Storage (immutable)"
	}
	values := []struct{ label, value string }{
		{nameLabel, m.streamForm.name}, {"Description", m.streamForm.description}, {"Subjects (comma separated)", m.streamForm.subjects},
		{storageLabel, string(m.streamForm.storage)}, {"Retention", string(m.streamForm.retention)}, {"Max messages (-1 unlimited)", m.streamForm.maxMessages},
		{"Max bytes (-1 unlimited)", m.streamForm.maxBytes}, {"Replicas (1-5)", m.streamForm.replicas},
	}
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(title), ""}
	for index, item := range values {
		label := lipgloss.NewStyle().Foreground(m.theme.muted).Render(item.label)
		value := item.value
		value = tailTruncate(value, max(width-8, 12))
		prefix := "  "
		if index == m.streamForm.field {
			prefix = lipgloss.NewStyle().Foreground(m.theme.accent).Render("› ")
			if index != 3 && index != 4 {
				value += lipgloss.NewStyle().Foreground(m.theme.accent).Render("▏")
			}
		}
		lines = append(lines, label, prefix+value)
	}
	if m.streamActionErr != nil {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.red).Render(m.streamActionErr.Error()))
	}
	if m.streamActionRunning {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.yellow).Render("↻ applying stream configuration"))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("tab next  •  ←/→ choose  •  ctrl+s save  •  esc cancel"))
	return lipgloss.NewStyle().Width(width).Height(height).Padding(1, 2).Render(strings.Join(lines, "\n"))
}

func tailTruncate(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	return "…" + string(runes[len(runes)-width+1:])
}

func (m Model) updateStreamDeleteConfirmation(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "y", "enter":
		m.confirmStreamDelete = false
		return m.runStreamDelete()
	case "n", "esc":
		m.confirmStreamDelete = false
	}
	return m, nil
}

func (m Model) runStreamDelete() (tea.Model, tea.Cmd) {
	if m.streamAction == nil || len(m.streams) == 0 {
		m.streamActionErr = errors.New("stream operations are unavailable")
		return m, nil
	}
	stream := m.streams[m.streamCursor]
	input := StreamActionInput{Generation: m.refreshGeneration, Action: streams.ActionDelete, Name: stream.Name}
	m.streamActionRunning = true
	m.streamActionErr = nil
	return m, m.streamAction(input)
}

func (m Model) streamDeleteConfirmationView(width, height int) string {
	stream := m.streams[m.streamCursor]
	content := strings.Join([]string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.red).Render("Delete stream?"), "",
		lipgloss.NewStyle().Foreground(m.theme.primary).Render(stream.Name),
		lipgloss.NewStyle().Foreground(m.theme.yellow).Render(fmt.Sprintf("%s stored messages  •  %s", compactNumber(stream.Messages), humanBytes(stream.Bytes))), "",
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render("This removes the stream and all stored messages. This cannot be undone."), "",
		lipgloss.NewStyle().Foreground(m.theme.muted).Render("y confirm   n/esc cancel"),
	}, "\n")
	card := lipgloss.NewStyle().Width(min(max(width-12, 34), 76)).Padding(2, 3).BorderStyle(lipgloss.RoundedBorder()).BorderForeground(m.theme.red).Render(content)
	return m.centeredCanvas(width, height, card)
}

func (m Model) streamDetailView(width int, stream streams.Summary) string {
	label := lipgloss.NewStyle().Foreground(m.theme.muted)
	value := lipgloss.NewStyle().Foreground(m.theme.secondary)
	description := stream.Description
	if description == "" {
		description = "—"
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(stream.Name), "",
		label.Render("Description:  ") + value.Render(description),
		label.Render("Subjects:     ") + value.Render(strings.Join(stream.Subjects, ", ")),
		label.Render("Storage:      ") + value.Render(string(stream.Storage)),
		label.Render("Retention:    ") + value.Render(string(stream.Retention)),
		label.Render("Replication:  ") + value.Render(fmt.Sprintf("%d replicas", stream.Replicas)), "",
		label.Render("Messages:     ") + value.Render(compactNumber(stream.Messages)),
		label.Render("Bytes:        ") + value.Render(humanBytes(stream.Bytes)),
		label.Render("Consumers:    ") + value.Render(fmt.Sprintf("%d", stream.Consumers)),
		label.Render("Max messages: ") + value.Render(compactSigned(stream.MaxMessages)),
		label.Render("Max bytes:    ") + value.Render(humanSignedBytes(stream.MaxBytes)), "",
		lipgloss.NewStyle().Foreground(m.theme.muted).Render("m messages  •  e edit  •  d delete  •  esc back"),
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(strings.Join(lines, "\n"))
}

func compactSigned(value int64) string {
	if value < 0 {
		return "unlimited"
	}
	return compactNumber(uint64(value))
}

func humanSignedBytes(value int64) string {
	if value < 0 {
		return "unlimited"
	}
	return humanBytes(uint64(value))
}

func (m *Model) applyStreamAction(input StreamActionInput) {
	summary := streams.Summary{
		Name: input.Definition.Name, Description: input.Definition.Description, Subjects: append([]string(nil), input.Definition.Subjects...),
		Storage: input.Definition.Storage, Retention: input.Definition.Retention, Replicas: input.Definition.Replicas,
		MaxMessages: input.Definition.MaxMessages, MaxBytes: input.Definition.MaxBytes,
	}
	switch input.Action {
	case streams.ActionCreate:
		m.streams = append(m.streams, summary)
		m.streamCursor = len(m.streams) - 1
	case streams.ActionUpdate:
		current := m.streams[m.streamCursor]
		summary.Messages, summary.Bytes, summary.Consumers = current.Messages, current.Bytes, current.Consumers
		m.streams[m.streamCursor] = summary
	case streams.ActionDelete:
		m.streams = append(m.streams[:m.streamCursor], m.streams[m.streamCursor+1:]...)
		m.streamCursor = min(m.streamCursor, max(len(m.streams)-1, 0))
	}
}
