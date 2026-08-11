package tui

import (
	"errors"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clcsolutions/nats-tui/internal/app/messages"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
)

const messageHistoryFieldCount = 6

type messageHistoryForm struct {
	field                                       int
	streamNames                                 []string
	stream, subject, sequence, startTime, limit string
	payloadExpression                           string
}

func newMessageHistoryForm(current string, streamCatalog []streams.Summary, previous messages.HistoryQuery) messageHistoryForm {
	names := make([]string, 0, len(streamCatalog))
	for _, stream := range streamCatalog {
		names = append(names, stream.Name)
	}
	if current == "" && len(names) > 0 {
		current = names[0]
	}
	form := messageHistoryForm{streamNames: names, stream: current, sequence: "0", limit: "50"}
	if previous.Stream == current {
		form.subject = previous.Subject
		form.sequence = strconv.FormatUint(previous.StartSequence, 10)
		if previous.StartTime != nil {
			form.startTime = previous.StartTime.Format(time.RFC3339)
		}
		if previous.Limit > 0 {
			form.limit = strconv.Itoa(previous.Limit)
		}
		form.payloadExpression = previous.PayloadExpression
	}
	return form
}

func (f messageHistoryForm) query() (messages.HistoryQuery, error) {
	sequence, err := strconv.ParseUint(strings.TrimSpace(f.sequence), 10, 64)
	if err != nil {
		return messages.HistoryQuery{}, errors.New("start sequence must be an integer")
	}
	limit, err := strconv.Atoi(strings.TrimSpace(f.limit))
	if err != nil {
		return messages.HistoryQuery{}, errors.New("limit must be an integer")
	}
	var startTime *time.Time
	if strings.TrimSpace(f.startTime) != "" {
		value, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(f.startTime))
		if parseErr != nil {
			return messages.HistoryQuery{}, errors.New("start time must use RFC3339")
		}
		startTime = &value
	}
	query := messages.HistoryQuery{Stream: strings.TrimSpace(f.stream), Subject: strings.TrimSpace(f.subject), StartSequence: sequence, StartTime: startTime, Limit: limit, PayloadExpression: strings.TrimSpace(f.payloadExpression)}
	if err := query.Validate(); err != nil {
		return messages.HistoryQuery{}, err
	}
	if _, err := messages.CompilePayloadFilter(query.PayloadExpression); err != nil {
		return messages.HistoryQuery{}, err
	}
	return query, nil
}

func (m Model) updateMessageHistoryForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.loadingMessages {
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.messageHistoryForm, m.messageErr = nil, nil
		return m, nil
	case "enter", "ctrl+s":
		return m.submitMessageHistory()
	case "tab", "down":
		m.messageHistoryForm.field = (m.messageHistoryForm.field + 1) % messageHistoryFieldCount
		return m, nil
	case "shift+tab", "up":
		m.messageHistoryForm.field = (m.messageHistoryForm.field - 1 + messageHistoryFieldCount) % messageHistoryFieldCount
		return m, nil
	case "left", "right":
		if m.messageHistoryForm.field == 0 && len(m.messageHistoryForm.streamNames) > 0 {
			index := 0
			for i, value := range m.messageHistoryForm.streamNames {
				if value == m.messageHistoryForm.stream {
					index = i
				}
			}
			step := 1
			if key.String() == "left" {
				step = -1
			}
			m.messageHistoryForm.stream = m.messageHistoryForm.streamNames[(index+step+len(m.messageHistoryForm.streamNames))%len(m.messageHistoryForm.streamNames)]
			return m, nil
		}
	case "backspace":
		m.editMessageHistoryField(true, "")
		return m, nil
	}
	if key.Type == tea.KeyRunes {
		m.editMessageHistoryField(false, string(key.Runes))
	}
	return m, nil
}

func (m *Model) editMessageHistoryField(backspace bool, addition string) {
	field := m.messageHistoryTextField()
	if field == nil {
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

func (m *Model) messageHistoryTextField() *string {
	switch m.messageHistoryForm.field {
	case 1:
		return &m.messageHistoryForm.subject
	case 2:
		return &m.messageHistoryForm.sequence
	case 3:
		return &m.messageHistoryForm.startTime
	case 4:
		return &m.messageHistoryForm.limit
	case 5:
		return &m.messageHistoryForm.payloadExpression
	default:
		return nil
	}
}

func (m Model) submitMessageHistory() (tea.Model, tea.Cmd) {
	if m.loadMessageHistory == nil {
		m.messageErr = errors.New("message history browser is unavailable")
		return m, nil
	}
	query, err := m.messageHistoryForm.query()
	if err != nil {
		m.messageErr = err
		return m, nil
	}
	m.messageHistoryQuery, m.messageErr, m.loadingMessages = query, nil, true
	return m, tea.Batch(m.spinner.Tick, m.loadMessageHistory(query))
}

func (m Model) messageHistoryFormView(width, height int) string {
	values := []struct{ label, value, help string }{
		{"Stream", m.messageHistoryForm.stream, "Choose the stream before reading history."},
		{"Subject filter", m.messageHistoryForm.subject, "Applied before loading; wildcards * and > are supported."},
		{"Start sequence", m.messageHistoryForm.sequence, "Lower bound. 0 starts at the stream's first sequence."},
		{"Start time", m.messageHistoryForm.startTime, "Lower bound in RFC3339; empty disables it."},
		{"Result limit", m.messageHistoryForm.limit, "1-10000 messages, newest first."},
		{"Payload filter", m.messageHistoryForm.payloadExpression, "JSONPath $.event.type or JMESPath event.type; optional == value."},
	}
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("Browse message history"), lipgloss.NewStyle().Foreground(m.theme.muted).Render("Server-bounded query · payload filtering is compiled once"), ""}
	for index, item := range values {
		prefix, value := "  ", tailTruncate(item.value, max(width-12, 12))
		if index == m.messageHistoryForm.field {
			prefix = lipgloss.NewStyle().Foreground(m.theme.accent).Render("› ")
			if m.messageHistoryTextField() != nil {
				value += lipgloss.NewStyle().Foreground(m.theme.accent).Render("▏")
			}
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.muted).Render(item.label), prefix+value)
		if index == m.messageHistoryForm.field {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(m.theme.secondary).Render(item.help))
		}
	}
	if m.messageErr != nil {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.red).Render(m.messageErr.Error()))
	}
	if m.loadingMessages {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.yellow).Render(m.spinner.View()+" loading bounded history"))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("tab next  •  ←/→ stream  •  enter load  •  esc cancel"))
	card := lipgloss.NewStyle().Width(min(max(width-12, 54), 100)).Padding(1, 2).BorderStyle(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}
