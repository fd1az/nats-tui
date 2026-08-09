package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clcsolutions/nats-tui/internal/app/objectstore"
)

type objectTransferMode uint8

const (
	objectUpload objectTransferMode = iota + 1
	objectDownload
)

type objectTransferForm struct {
	mode        objectTransferMode
	field       int
	bucket      string
	objectName  string
	path        string
	description string
	metadata    string
	headers     string
	force       bool
}

func newObjectUploadForm(bucket string) objectTransferForm {
	return objectTransferForm{mode: objectUpload, bucket: bucket}
}

func newObjectDownloadForm(bucket, name string) objectTransferForm {
	return objectTransferForm{mode: objectDownload, bucket: bucket, objectName: name, path: filepath.Base(name)}
}

func (f objectTransferForm) fieldCount() int {
	if f.mode == objectDownload {
		return 2
	}
	return 5
}

func (m Model) updateObjectTransferForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.objectTransferForm = nil
		m.objectTransferErr = nil
	case "ctrl+s", "enter":
		return m.submitObjectTransfer()
	case "tab", "down":
		m.objectTransferForm.field = (m.objectTransferForm.field + 1) % m.objectTransferForm.fieldCount()
	case "shift+tab", "up":
		m.objectTransferForm.field = (m.objectTransferForm.field - 1 + m.objectTransferForm.fieldCount()) % m.objectTransferForm.fieldCount()
	case "left", "right", " ":
		if m.objectTransferForm.mode == objectDownload && m.objectTransferForm.field == 1 {
			m.objectTransferForm.force = !m.objectTransferForm.force
		}
	case "backspace":
		m.editObjectTransferField(true, "")
	default:
		if key.Type == tea.KeyRunes {
			m.editObjectTransferField(false, string(key.Runes))
		}
	}
	return m, nil
}

func (m *Model) editObjectTransferField(backspace bool, addition string) {
	field := m.objectTransferTextField()
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

func (m *Model) objectTransferTextField() *string {
	form := m.objectTransferForm
	if form.mode == objectDownload {
		if form.field == 0 {
			return &form.path
		}
		return nil
	}
	switch form.field {
	case 0:
		return &form.path
	case 1:
		return &form.objectName
	case 2:
		return &form.description
	case 3:
		return &form.metadata
	case 4:
		return &form.headers
	default:
		return nil
	}
}

func (m Model) submitObjectTransfer() (tea.Model, tea.Cmd) {
	form := *m.objectTransferForm
	if strings.TrimSpace(form.path) == "" {
		m.objectTransferErr = fmt.Errorf("file path is required")
		return m, nil
	}
	var metadata map[string]string
	var headers map[string][]string
	if form.mode == objectDownload {
		if m.startObjectDownload == nil {
			m.objectTransferErr = fmt.Errorf("object download is unavailable")
			return m, nil
		}
	} else {
		var err error
		metadata, err = parseObjectProperties(form.metadata)
		if err != nil {
			m.objectTransferErr = fmt.Errorf("metadata: %w", err)
			return m, nil
		}
		headers, err = parseObjectHeaders(form.headers)
		if err != nil {
			m.objectTransferErr = fmt.Errorf("headers: %w", err)
			return m, nil
		}
		if m.startObjectUpload == nil {
			m.objectTransferErr = fmt.Errorf("object upload is unavailable")
			return m, nil
		}
	}
	m.objectTransferErr = nil
	m.objectTransferNotice = ""
	m.objectTransferProgress = objectstore.Progress{}
	m.objectTransferStarting = true
	m.objectTransferStartedAt = m.runtime.Now()
	m.objectTransferPath = form.path
	m.objectTransferForm = nil
	if form.mode == objectDownload {
		m.objectTransferDirection = "download"
		m.objectTransferName = form.objectName
		return m, m.startObjectDownload(objectstore.DownloadFile{Bucket: form.bucket, Name: form.objectName, Path: form.path, Force: form.force})
	}
	name := strings.TrimSpace(form.objectName)
	if name == "" {
		name = filepath.Base(form.path)
	}
	m.objectTransferDirection = "upload"
	m.objectTransferName = name
	return m, m.startObjectUpload(objectstore.UploadFile{
		Bucket: form.bucket, Path: form.path, Name: form.objectName, Description: form.description, Metadata: metadata, Headers: headers,
	})
}

func parseObjectProperties(input string) (map[string]string, error) {
	return parseObjectPairs(input)
}

func parseObjectHeaders(input string) (map[string][]string, error) {
	if strings.TrimSpace(input) == "" {
		return nil, nil
	}
	result := make(map[string][]string)
	for _, pair := range strings.Split(input, ",") {
		name, value, ok := strings.Cut(pair, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("expected comma-separated name=value pairs")
		}
		result[name] = append(result[name], strings.TrimSpace(value))
	}
	return result, nil
}

func parseObjectPairs(input string) (map[string]string, error) {
	if strings.TrimSpace(input) == "" {
		return nil, nil
	}
	result := make(map[string]string)
	for _, pair := range strings.Split(input, ",") {
		name, value, ok := strings.Cut(pair, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("expected comma-separated key=value pairs")
		}
		result[name] = strings.TrimSpace(value)
	}
	return result, nil
}

func (m Model) objectTransferView(width, height int) string {
	if m.objectTransferForm != nil {
		return m.objectTransferFormView(width, height)
	}
	title := strings.ToUpper(m.objectTransferDirection) + " OBJECT"
	status := "Starting transfer…"
	if m.objectTransferCanceling {
		status = "Canceling safely…"
	} else if m.objectTransferActive {
		status = "Transfer in progress"
	}
	progress := m.objectTransferProgress
	percent := 0.0
	if progress.Total > 0 {
		percent = min(float64(progress.Transferred)/float64(progress.Total), 1) * 100
	}
	barWidth := min(max(width-24, 20), 64)
	filled := int(float64(barWidth) * percent / 100)
	bar := lipgloss.NewStyle().Foreground(m.theme.accent).Render(strings.Repeat("━", filled)) + lipgloss.NewStyle().Foreground(m.theme.border).Render(strings.Repeat("─", max(barWidth-filled, 0)))
	elapsed := m.runtime.Now().Sub(m.objectTransferStartedAt)
	speed := float64(0)
	if elapsed > 0 {
		speed = float64(progress.Transferred) / elapsed.Seconds()
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(title), "",
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.primary).Render(m.objectTransferName),
		lipgloss.NewStyle().Foreground(m.theme.muted).Render(m.objectTransferPath), "",
		bar, fmt.Sprintf("%.1f%%   %s / %s", percent, humanBytes(progress.Transferred), transferTotal(progress.Total)),
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render(fmt.Sprintf("%s  •  %s/s  •  %s", status, humanBytes(uint64(speed)), elapsed.Round(time.Second))), "",
		lipgloss.NewStyle().Foreground(m.theme.muted).Render("esc cancel transfer"),
	}
	card := lipgloss.NewStyle().Width(min(max(width-12, 42), 80)).Padding(2, 3).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}

func transferTotal(total uint64) string {
	if total == 0 {
		return "unknown"
	}
	return humanBytes(total)
}

func (m Model) objectTransferFormView(width, height int) string {
	form := m.objectTransferForm
	title := "Upload object"
	fields := []struct{ label, value string }{
		{"Local file path", form.path}, {"Object name", form.objectName}, {"Description", form.description},
		{"Metadata", form.metadata}, {"Headers", form.headers},
	}
	if form.mode == objectDownload {
		title = "Download object"
		fields = []struct{ label, value string }{{"Destination", form.path}, {"Overwrite", enabledLabel(form.force)}}
	}
	cardWidth := min(max(width-14, 42), 82)
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(title),
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render(form.bucket + " / " + valueOr(form.objectName, filepath.Base(form.path))), "",
	}
	for index, field := range fields {
		selected := index == form.field
		marker := "  "
		style := lipgloss.NewStyle().Foreground(m.theme.secondary)
		if selected {
			marker = "› "
			style = style.Foreground(m.theme.accent).Bold(true)
		}
		value := field.value
		if strings.TrimSpace(value) == "" {
			value = "(empty)"
		}
		if selected && !(form.mode == objectDownload && index == 1) {
			value += " ▏"
		}
		lines = append(lines, marker+lipgloss.NewStyle().Foreground(m.theme.muted).Width(18).Render(field.label)+style.Render(tailTruncate(value, max(cardWidth-22, 12))))
	}
	if m.objectTransferErr != nil {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.red).Render(m.objectTransferErr.Error()))
	}
	help := "Metadata/headers: comma-separated key=value pairs."
	if form.mode == objectDownload {
		help = "Overwrite stays off unless explicitly enabled. Bytes are verified before the destination appears."
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.cyan).Render(help), "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("tab next  •  ctrl+s start  •  esc cancel"))
	card := lipgloss.NewStyle().Width(cardWidth).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}

func waitForObjectTransferEvent(events <-chan objectstore.TransferEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return objectTransferEndedMsg{}
		}
		return objectTransferEventMsg{event: event, events: events}
	}
}

func (m *Model) applyObjectTransferResult(event objectstore.TransferEvent) {
	if m.objectTransferDirection == "upload" {
		replaced := false
		for index := range m.objects {
			if m.objects[index].Name == event.Object.Name {
				m.objects[index] = event.Object
				replaced = true
				break
			}
		}
		if !replaced {
			m.objects = append(m.objects, event.Object)
		}
		m.rebuildObjectSearch()
		m.objectObjectCursor = findObject(m.displayedObjects(), event.Object.Name)
		m.objectTransferNotice = "uploaded " + event.Object.Name
		return
	}
	verified := ""
	if event.Verified {
		verified = " and verified"
	}
	m.objectTransferNotice = "downloaded" + verified + " to " + event.Destination
}
