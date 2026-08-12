package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fd1az/nats-tui/internal/app/messages"
	"github.com/fd1az/nats-tui/internal/app/streams"
	domain "github.com/fd1az/nats-tui/internal/domain/requestreply"
)

type messageExportForm struct {
	path   textinput.Model
	field  int
	format messages.ExportFormat
	all    bool
	force  bool
}

type messageRepublishForm struct {
	subject textinput.Model
	record  messages.Record
}

type messageExportSucceededMsg struct{ result messages.ExportResult }
type messageExportFailedMsg struct{ err error }
type messageDeleteSucceededMsg struct{ sequence uint64 }
type messageDeleteFailedMsg struct{ err error }

func MessageExportSucceeded(result messages.ExportResult) tea.Msg {
	return messageExportSucceededMsg{result}
}
func MessageExportFailed(err error) tea.Msg          { return messageExportFailedMsg{err} }
func MessageDeleteSucceeded(sequence uint64) tea.Msg { return messageDeleteSucceededMsg{sequence} }
func MessageDeleteFailed(err error) tea.Msg          { return messageDeleteFailedMsg{err} }

func newMessageInput(placeholder string) textinput.Model {
	input := textinput.New()
	input.Placeholder = placeholder
	input.CharLimit = 4096
	return input
}

func newMessageExportForm(stream string) messageExportForm {
	path := newMessageInput("./messages.jsonl")
	path.SetValue("./" + strings.ToLower(stream) + "-messages.jsonl")
	path.Focus()
	return messageExportForm{path: path, format: messages.ExportJSONL, all: true}
}

func newMessageRepublishForm(record messages.Record) messageRepublishForm {
	subject := newMessageInput("events.replayed")
	subject.SetValue(record.Subject)
	subject.Focus()
	return messageRepublishForm{subject: subject, record: record}
}

func (m Model) updateMessageExportForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	form := m.messageExportForm
	switch key.String() {
	case "esc":
		m.messageExportForm = nil
		return m, nil
	case "tab", "down":
		form.field = (form.field + 1) % 4
	case "shift+tab", "up":
		form.field = (form.field + 3) % 4
	case "left", "right", " ":
		switch form.field {
		case 1:
			formats := []messages.ExportFormat{messages.ExportJSONL, messages.ExportJSON, messages.ExportRaw}
			for index, format := range formats {
				if form.format == format {
					step := 1
					if key.String() == "left" {
						step = -1
					}
					form.format = formats[(index+step+len(formats))%len(formats)]
					break
				}
			}
		case 2:
			form.all = !form.all
		case 3:
			form.force = !form.force
		}
	case "enter", "ctrl+s":
		if form.field == 0 && key.String() == "enter" {
			form.field++
			break
		}
		return m.submitMessageExport()
	default:
		if form.field == 0 {
			var command tea.Cmd
			form.path, command = form.path.Update(key)
			return m, command
		}
	}
	if form.field == 0 {
		form.path.Focus()
	} else {
		form.path.Blur()
	}
	return m, nil
}

func (m Model) submitMessageExport() (tea.Model, tea.Cmd) {
	if m.exportMessages == nil {
		m.messageErr = errors.New("message export service is unavailable")
		return m, nil
	}
	records := m.displayedMessages()
	if !m.messageExportForm.all && len(records) > 0 {
		records = []messages.Record{records[m.messageCursor]}
	}
	if m.messageExportForm.format == messages.ExportRaw && len(records) != 1 {
		m.messageErr = errors.New("raw export requires Selected message scope")
		return m, nil
	}
	request := messages.ExportRequest{Path: strings.TrimSpace(m.messageExportForm.path.Value()), Format: m.messageExportForm.format, Force: m.messageExportForm.force, Records: append([]messages.Record(nil), records...)}
	m.messageOperationRunning, m.messageErr = true, nil
	return m, m.exportMessages(request)
}

func (m Model) updateMessageRepublishForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.messageRepublishForm = nil
		return m, nil
	case "enter", "ctrl+s":
		if m.requestReplyServices.Publish == nil {
			m.messageErr = errors.New("publish service is unavailable")
			return m, nil
		}
		record := m.messageRepublishForm.record
		message, err := domain.NewMessage(strings.TrimSpace(m.messageRepublishForm.subject.Value()), record.Payload, record.Headers)
		if err != nil {
			m.messageErr = err
			return m, nil
		}
		m.messageOperationRunning, m.messageErr = true, nil
		return m, m.requestReplyServices.Publish(message)
	default:
		var command tea.Cmd
		m.messageRepublishForm.subject, command = m.messageRepublishForm.subject.Update(key)
		return m, command
	}
}

func (m Model) updateMessageDeleteConfirmation(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "n":
		m.confirmMessageDelete = false
		return m, nil
	case "enter", "y":
		if m.deleteMessage == nil {
			m.messageErr = errors.New("message delete service is unavailable")
			return m, nil
		}
		records := m.displayedMessages()
		if m.messageCursor >= len(records) || records[m.messageCursor].Sequence == 0 {
			m.messageErr = errors.New("only stored JetStream messages can be deleted")
			return m, nil
		}
		command := streams.MessageDeleteCommand{Stream: m.messageStream, Sequence: records[m.messageCursor].Sequence, Secure: true}
		m.messageOperationRunning, m.confirmMessageDelete = true, false
		return m, m.deleteMessage(command)
	}
	return m, nil
}

func (m Model) messageExportFormView(width, height int) string {
	form := m.messageExportForm
	scope := "All visible messages"
	if !form.all {
		scope = "Selected message"
	}
	values := []string{form.path.View(), strings.ToUpper(string(form.format)), scope, enabledLabel(form.force)}
	help := []string{"Destination path; parent directories are created securely.", "JSONL streams well, JSON is an array, RAW requires one selected message.", "Visible respects the current history query and reactive search.", "OFF protects existing files; ON replaces the destination atomically."}
	labels := []string{"Path", "Format", "Scope", "Overwrite"}
	cardWidth := min(max(width-14, 54), 90)
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("Export messages"), lipgloss.NewStyle().Foreground(m.theme.secondary).Render("Write a stable message envelope without blocking the live subscription."), ""}
	for index := range labels {
		marker := "  "
		style := lipgloss.NewStyle().Foreground(m.theme.secondary)
		if index == form.field {
			marker, style = "› ", style.Foreground(m.theme.accent).Bold(true)
		}
		lines = append(lines, marker+lipgloss.NewStyle().Foreground(m.theme.muted).Width(16).Render(labels[index])+style.Render(values[index]))
	}
	lines = append(lines, "", lipgloss.NewStyle().Bold(true).Foreground(m.theme.cyan).Render("WHAT TO PUT HERE"), lipgloss.NewStyle().Width(cardWidth).Foreground(m.theme.secondary).Render(help[form.field]), "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("tab field  •  ←/→ choose  •  ctrl+s export  •  esc cancel"))
	card := lipgloss.NewStyle().Width(cardWidth).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}

func (m Model) messageRepublishFormView(width, height int) string {
	form := m.messageRepublishForm
	cardWidth := min(max(width-14, 54), 90)
	content := strings.Join([]string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("Republish message"),
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render(fmt.Sprintf("Replay #%d with its original payload and complete headers.", form.record.Sequence)), "",
		lipgloss.NewStyle().Foreground(m.theme.muted).Render("Destination subject"), "› " + form.subject.View(), "",
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.cyan).Render("WHAT TO PUT HERE"),
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render("Use a concrete Core NATS subject. Wildcards and whitespace are rejected."), "",
		lipgloss.NewStyle().Foreground(m.theme.yellow).Render("This publishes a new live message."),
		lipgloss.NewStyle().Foreground(m.theme.muted).Render("enter publish  •  esc cancel"),
	}, "\n")
	card := lipgloss.NewStyle().Width(cardWidth).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(content)
	return m.centeredCanvas(width, height, card)
}

func (m Model) messageDeleteConfirmationView(width, height int) string {
	records := m.displayedMessages()
	if m.messageCursor >= len(records) {
		return ""
	}
	record := records[m.messageCursor]
	content := lipgloss.NewStyle().Bold(true).Foreground(m.theme.red).Render("Permanently delete stored message?") + "\n\n" +
		fmt.Sprintf("Stream: %s\nSequence: %d\nSubject: %s", m.messageStream, record.Sequence, record.Subject) + "\n\n" +
		lipgloss.NewStyle().Foreground(m.theme.yellow).Render("This cannot be undone and may create a sequence gap.") + "\n\ny / enter confirm  •  n / esc cancel"
	card := lipgloss.NewStyle().Width(min(max(width-14, 54), 82)).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.red).Render(content)
	return m.centeredCanvas(width, height, card)
}
