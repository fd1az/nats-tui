package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
)

type streamTransferMode uint8

const (
	streamBackup streamTransferMode = iota + 1
	streamRestore
)

type streamTransferForm struct {
	mode             streamTransferMode
	field            int
	stream           string
	directory        string
	includeConsumers bool
	checkMessages    bool
	force            bool
	restoreConfirmed bool
}

func newStreamBackupForm(name string) streamTransferForm {
	return streamTransferForm{mode: streamBackup, stream: name, directory: filepath.Join("backups", name), includeConsumers: true}
}

func newStreamRestoreForm(name string) streamTransferForm {
	directory := "backups"
	if name != "" {
		directory = filepath.Join(directory, name)
	}
	return streamTransferForm{mode: streamRestore, stream: name, directory: directory}
}

func (f streamTransferForm) fieldCount() int {
	if f.mode == streamRestore {
		return 3
	}
	return 4
}

func (m Model) updateStreamTransferForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	form := m.streamTransferForm
	switch key.String() {
	case "esc":
		m.streamTransferForm = nil
		m.streamTransferErr = nil
	case "ctrl+s", "enter":
		return m.submitStreamTransfer()
	case "tab", "down":
		form.field = (form.field + 1) % form.fieldCount()
	case "shift+tab", "up":
		form.field = (form.field - 1 + form.fieldCount()) % form.fieldCount()
	case "left", "right", " ":
		if form.mode == streamBackup {
			switch form.field {
			case 1:
				form.includeConsumers = !form.includeConsumers
			case 2:
				form.checkMessages = !form.checkMessages
			case 3:
				form.force = !form.force
			}
		} else if form.field == 2 {
			form.restoreConfirmed = !form.restoreConfirmed
		}
	case "backspace":
		m.editStreamTransferField(true, "")
	default:
		if key.Type == tea.KeyRunes {
			m.editStreamTransferField(false, string(key.Runes))
		}
	}
	return m, nil
}

func (m *Model) editStreamTransferField(backspace bool, addition string) {
	form := m.streamTransferForm
	var field *string
	if form.mode == streamBackup {
		if form.field == 0 {
			field = &form.directory
		}
	} else {
		switch form.field {
		case 0:
			field = &form.stream
		case 1:
			field = &form.directory
		}
	}
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

func (m Model) submitStreamTransfer() (tea.Model, tea.Cmd) {
	form := *m.streamTransferForm
	if strings.TrimSpace(form.stream) == "" {
		m.streamTransferErr = fmt.Errorf("stream name is required")
		return m, nil
	}
	if strings.TrimSpace(form.directory) == "" {
		m.streamTransferErr = fmt.Errorf("backup directory is required")
		return m, nil
	}
	if form.mode == streamRestore && !form.restoreConfirmed {
		m.streamTransferErr = fmt.Errorf("enable the restore confirmation first")
		return m, nil
	}
	if form.mode == streamBackup && m.startStreamBackup == nil {
		m.streamTransferErr = fmt.Errorf("stream backup is unavailable")
		return m, nil
	}
	if form.mode == streamRestore && m.startStreamRestore == nil {
		m.streamTransferErr = fmt.Errorf("stream restore is unavailable")
		return m, nil
	}
	m.streamTransferErr = nil
	m.streamTransferNotice = ""
	m.streamTransferProgress = streams.TransferProgress{}
	m.streamTransferStarting = true
	m.streamTransferStartedAt = m.runtime.Now()
	m.streamTransferDirection = map[bool]string{true: "restore", false: "backup"}[form.mode == streamRestore]
	m.streamTransferName, m.streamTransferDirectory = form.stream, form.directory
	m.streamTransferForm = nil
	if form.mode == streamRestore {
		return m, tea.Batch(m.spinner.Tick, m.startStreamRestore(streams.RestoreFile{Stream: form.stream, Directory: form.directory}))
	}
	return m, tea.Batch(m.spinner.Tick, m.startStreamBackup(streams.BackupFile{Stream: form.stream, Directory: form.directory, Force: form.force, IncludeConsumers: form.includeConsumers, CheckMessages: form.checkMessages}))
}

func (m Model) streamTransferFormView(width, height int) string {
	form := m.streamTransferForm
	title := "Back up stream"
	subtitle := "Portable backup · compatible with the official NATS CLI"
	fields := []struct{ label, value, help string }{
		{"Destination", form.directory, "Creates backup.json and stream.tar.s2."},
		{"Consumers", enabledLabel(form.includeConsumers), "Include durable consumer config and delivery state."},
		{"Health check", enabledLabel(form.checkMessages), "Verify message checksums first; safer but slower."},
		{"Replace", enabledLabel(form.force), "Publish atomically; replace the old directory only on success."},
	}
	if form.mode == streamRestore {
		title = "Restore stream"
		subtitle = "Recreates the stream, messages and durable consumers"
		fields = []struct{ label, value, help string }{
			{"Stream name", form.stream, "Must match the name stored in backup.json."},
			{"Backup directory", form.directory, "Directory containing backup.json and stream.tar.s2."},
			{"Confirm restore", enabledLabel(form.restoreConfirmed), "Target stream must not already exist."},
		}
	}
	cardWidth := min(max(width-14, 48), 88)
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(title), lipgloss.NewStyle().Foreground(m.theme.secondary).Render(subtitle), ""}
	for index, field := range fields {
		selected := index == form.field
		marker := "  "
		style := lipgloss.NewStyle().Foreground(m.theme.secondary)
		if selected {
			marker = "› "
			style = style.Foreground(m.theme.accent).Bold(true)
		}
		value := field.value
		if value == "" {
			value = "(empty)"
		}
		isToggle := (form.mode == streamBackup && index > 0) || (form.mode == streamRestore && index == 2)
		if selected && !isToggle {
			value += " ▏"
		}
		lines = append(lines, marker+lipgloss.NewStyle().Foreground(m.theme.muted).Width(20).Render(field.label)+style.Render(tailTruncate(value, max(cardWidth-24, 12))), lipgloss.NewStyle().Foreground(m.theme.muted).Render("    "+field.help), "")
	}
	if m.streamTransferErr != nil {
		lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.red).Render(m.streamTransferErr.Error()), "")
	}
	lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.muted).Render("tab next  •  space toggle  •  ctrl+s start  •  esc cancel"))
	border := m.theme.border
	if form.mode == streamRestore {
		border = m.theme.yellow
	}
	card := lipgloss.NewStyle().Width(cardWidth).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(border).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}

func (m Model) streamTransferView(width, height int) string {
	if m.streamTransferForm != nil {
		return m.streamTransferFormView(width, height)
	}
	if m.streamTransferErr != nil && !m.streamTransferActive && !m.streamTransferStarting {
		content := strings.Join([]string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.red).Render("Stream transfer failed"), "", m.streamTransferErr.Error(), "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("esc close")}, "\n")
		card := lipgloss.NewStyle().Width(min(max(width-14, 44), 82)).Padding(2, 3).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.red).Render(content)
		return m.centeredCanvas(width, height, card)
	}
	title := strings.ToUpper(m.streamTransferDirection) + " STREAM"
	status := "Preparing secure local files…"
	if m.streamTransferCanceling {
		status = "Canceling safely…"
	} else if m.streamTransferActive {
		status = "Transfer in progress"
	}
	progress := m.streamTransferProgress
	elapsed := m.runtime.Now().Sub(m.streamTransferStartedAt)
	rate := uint64(0)
	if elapsed > 0 {
		rate = uint64(float64(progress.Transferred) / elapsed.Seconds())
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(title), "",
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.primary).Render(m.streamTransferName),
		lipgloss.NewStyle().Foreground(m.theme.muted).Render(m.streamTransferDirectory), "",
		lipgloss.NewStyle().Foreground(m.theme.accent).Render(m.spinner.View() + " " + status),
		fmt.Sprintf("%s transferred  •  %d chunks", humanBytes(progress.Transferred), progress.Chunks),
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render(fmt.Sprintf("%s/s  •  %s", humanBytes(rate), elapsed.Round(time.Second))), "",
		lipgloss.NewStyle().Foreground(m.theme.muted).Render("esc cancel transfer"),
	}
	card := lipgloss.NewStyle().Width(min(max(width-12, 44), 80)).Padding(2, 3).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}

func waitForStreamTransferEvent(events <-chan streams.TransferEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return streamTransferEndedMsg{}
		}
		return streamTransferEventMsg{event: event, events: events}
	}
}
