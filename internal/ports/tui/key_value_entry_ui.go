package tui

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fd1az/nats-tui/internal/app/kvstore"
)

type keyValueEntryForm struct {
	bucket           string
	key              textinput.Model
	value            textarea.Model
	editing          bool
	expectedRevision *uint64
	focus            int
}

func newCreateKeyValueEntryForm(bucket string, colors theme) keyValueEntryForm {
	key := textinput.New()
	key.Placeholder = "feature.name"
	key.Prompt = "› "
	key.CharLimit = 1024
	key.PromptStyle = lipgloss.NewStyle().Foreground(colors.accent)
	key.TextStyle = lipgloss.NewStyle().Foreground(colors.primary)
	key.Cursor.Style = lipgloss.NewStyle().Foreground(colors.cyan)
	key.Cursor.SetMode(cursor.CursorHide)
	key.Focus()
	value := newKeyValueTextarea(colors)
	return keyValueEntryForm{bucket: bucket, key: key, value: value}
}

func newEditKeyValueEntryForm(entry kvstore.Entry, colors theme) keyValueEntryForm {
	form := newCreateKeyValueEntryForm(entry.Bucket, colors)
	form.editing = true
	form.focus = 1
	form.key.SetValue(entry.Key)
	form.key.Blur()
	form.value.SetValue(string(entry.Value))
	form.value.Focus()
	revision := entry.Revision
	form.expectedRevision = &revision
	return form
}

func newKeyValueTextarea(colors theme) textarea.Model {
	value := textarea.New()
	value.Placeholder = "JSON, text, or any UTF-8 value"
	value.Prompt = ""
	value.ShowLineNumbers = true
	value.Cursor.Style = lipgloss.NewStyle().Foreground(colors.cyan)
	value.Cursor.SetMode(cursor.CursorHide)
	for _, style := range []*textarea.Style{&value.FocusedStyle, &value.BlurredStyle} {
		style.Base = lipgloss.NewStyle()
		style.CursorLine = lipgloss.NewStyle().Foreground(colors.primary)
		style.CursorLineNumber = lipgloss.NewStyle().Foreground(colors.accent)
		style.EndOfBuffer = lipgloss.NewStyle().Foreground(colors.border)
		style.LineNumber = lipgloss.NewStyle().Foreground(colors.muted)
		style.Placeholder = lipgloss.NewStyle().Foreground(colors.muted)
		style.Prompt = lipgloss.NewStyle().Foreground(colors.accent)
		style.Text = lipgloss.NewStyle().Foreground(colors.primary)
	}
	return value
}

func (m Model) updateKeyValueEntryForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.keyValueEntrySaving {
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.keyValueEntryForm = nil
		m.keyValueEntryWriteErr = nil
		return m, nil
	case "ctrl+s":
		return m.submitKeyValueEntryForm()
	case "tab", "shift+tab":
		if m.keyValueEntryForm.editing {
			m.keyValueEntryForm.focus = 1
			return m, nil
		}
		m.keyValueEntryForm.focus = 1 - m.keyValueEntryForm.focus
		if m.keyValueEntryForm.focus == 0 {
			m.keyValueEntryForm.value.Blur()
			return m, m.keyValueEntryForm.key.Focus()
		}
		m.keyValueEntryForm.key.Blur()
		return m, m.keyValueEntryForm.value.Focus()
	}
	var command tea.Cmd
	if m.keyValueEntryForm.focus == 0 {
		m.keyValueEntryForm.key, command = m.keyValueEntryForm.key.Update(key)
		return m, command
	}
	m.keyValueEntryForm.value, command = m.keyValueEntryForm.value.Update(key)
	return m, command
}

func (m Model) submitKeyValueEntryForm() (tea.Model, tea.Cmd) {
	if m.keyValuePut == nil {
		m.keyValueEntryWriteErr = errors.New("Key-Value writes are unavailable")
		return m, nil
	}
	command := kvstore.WriteEntry{
		Bucket: m.keyValueEntryForm.bucket, Key: m.keyValueEntryForm.key.Value(),
		Value: []byte(m.keyValueEntryForm.value.Value()), ExpectedRevision: m.keyValueEntryForm.expectedRevision,
	}
	if strings.TrimSpace(command.Key) == "" {
		m.keyValueEntryWriteErr = errors.New("key is required")
		return m, nil
	}
	input := KeyValuePutInput{Generation: m.refreshGeneration, Command: command}
	m.keyValueEntrySaving = true
	m.keyValueEntryWriteErr = nil
	return m, m.keyValuePut(input)
}

func (m Model) keyValueEntryFormView(width, height int) string {
	form := *m.keyValueEntryForm
	contentWidth := max(width-6, 24)
	form.key.Width = contentWidth - 2
	form.value.SetWidth(contentWidth)
	form.value.SetHeight(max(height-12, 5))
	title := "Create KV entry"
	keyLabel := "Key"
	if form.editing {
		title = "Edit KV entry"
		keyLabel = fmt.Sprintf("Key (immutable)  •  CAS revision %d", *form.expectedRevision)
	}
	label := lipgloss.NewStyle().Foreground(m.theme.muted)
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(title),
		label.Render("Bucket  ") + lipgloss.NewStyle().Foreground(m.theme.secondary).Render(form.bucket), "",
		label.Render(keyLabel), form.key.View(), "", label.Render("Value"), form.value.View(),
	}
	if m.keyValueEntryWriteErr != nil {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.red).Render(m.keyValueEntryWriteErr.Error()))
	}
	if m.keyValueEntrySaving {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.yellow).Render(m.spinner.View()+" Storing value…"))
	}
	lines = append(lines, "", label.Render("tab switch field  •  enter newline  •  ctrl+s save  •  esc cancel"))
	return lipgloss.NewStyle().Width(width).Height(height).Padding(1, 2).Render(strings.Join(lines, "\n"))
}

func (m *Model) rememberKeyValueKey(key string) {
	for _, current := range m.keyValueKeys {
		if current == key {
			m.keyValueKeyQuery = ""
			m.filteredKeyValueKeys = nil
			m.keyValueKeyCursor = findString(m.keyValueKeys, key)
			return
		}
	}
	m.keyValueKeys = append(m.keyValueKeys, key)
	sort.Strings(m.keyValueKeys)
	m.keyValueKeyQuery = ""
	m.filteredKeyValueKeys = nil
	m.keyValueKeyCursor = findString(m.keyValueKeys, key)
}

func findString(values []string, target string) int {
	for index, value := range values {
		if value == target {
			return index
		}
	}
	return 0
}

func (m Model) updateKeyValueEntryRemovalConfirmation(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "y", "enter":
		return m.runKeyValueEntryRemoval()
	case "n", "esc":
		m.pendingKeyValueEntryRemoval = ""
	}
	return m, nil
}

func (m Model) runKeyValueEntryRemoval() (tea.Model, tea.Cmd) {
	if m.keyValueRemove == nil || m.keyValueEntry == nil {
		m.pendingKeyValueEntryRemoval = ""
		m.keyValueEntryWriteErr = errors.New("Key-Value removal is unavailable")
		return m, nil
	}
	revision := m.keyValueEntry.Revision
	command := kvstore.RemoveEntry{
		Bucket: m.keyValueEntry.Bucket, Key: m.keyValueEntry.Key,
		Action: m.pendingKeyValueEntryRemoval, ExpectedRevision: &revision,
	}
	input := KeyValueRemoveInput{Generation: m.refreshGeneration, Command: command}
	m.pendingKeyValueEntryRemoval = ""
	m.keyValueEntrySaving = true
	m.keyValueEntryWriteErr = nil
	return m, m.keyValueRemove(input)
}

func (m Model) keyValueEntryRemovalConfirmationView(width, height int) string {
	entry := *m.keyValueEntry
	purge := m.pendingKeyValueEntryRemoval == kvstore.RemovePurge
	title := "Delete KV key?"
	impact := "A delete marker is added; revision history remains and the key can be restored."
	warning := "This does not destroy previous values."
	if purge {
		title = "Purge KV key?"
		impact = "This permanently removes all previous revisions and leaves only a purge marker."
		warning = "Revision history will be lost. This cannot be undone."
	}
	content := strings.Join([]string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.red).Render(title), "",
		lipgloss.NewStyle().Foreground(m.theme.primary).Render(entry.Bucket + " / " + entry.Key),
		lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("Revision %d  •  %s", entry.Revision, humanBytes(uint64(len(entry.Value))))), "",
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render(impact),
		lipgloss.NewStyle().Foreground(m.theme.red).Render(warning), "",
		lipgloss.NewStyle().Foreground(m.theme.muted).Render("y confirm   n/esc cancel"),
	}, "\n")
	card := lipgloss.NewStyle().Width(min(max(width-12, 38), 84)).Padding(2, 3).BorderStyle(lipgloss.RoundedBorder()).BorderForeground(m.theme.red).Render(content)
	return m.centeredCanvas(width, height, card)
}

func (m *Model) removeKeyValueKey(key string) {
	index := findString(m.keyValueKeys, key)
	if index < 0 || index >= len(m.keyValueKeys) || m.keyValueKeys[index] != key {
		return
	}
	m.keyValueKeys = append(m.keyValueKeys[:index], m.keyValueKeys[index+1:]...)
	m.keyValueKeyQuery = ""
	m.filteredKeyValueKeys = nil
	m.keyValueKeyCursor = min(m.keyValueKeyCursor, max(len(m.keyValueKeys)-1, 0))
}
