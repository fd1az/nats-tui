package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clcsolutions/nats-tui/internal/app/objectstore"
)

type objectEditForm struct {
	field       int
	bucket      string
	currentName string
	name        string
	description string
	metadata    string
	headers     string
}

func newObjectEditForm(bucket string, object objectstore.ObjectSummary) objectEditForm {
	return objectEditForm{
		bucket: bucket, currentName: object.Name, name: object.Name, description: object.Description,
		metadata: formatObjectProperties(object.Metadata), headers: formatObjectHeaders(object.Headers),
	}
}

func (m Model) updateObjectEditForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.objectEditForm = nil
		m.objectOperationErr = nil
	case "tab", "down":
		m.objectEditForm.field = (m.objectEditForm.field + 1) % 4
	case "shift+tab", "up":
		m.objectEditForm.field = (m.objectEditForm.field + 3) % 4
	case "ctrl+s", "enter":
		if m.editObject == nil {
			m.objectOperationErr = fmt.Errorf("object editing is unavailable")
			return m, nil
		}
		metadata, err := parseObjectProperties(m.objectEditForm.metadata)
		if err != nil {
			m.objectOperationErr = fmt.Errorf("metadata: %w", err)
			return m, nil
		}
		headers, err := parseObjectHeaders(m.objectEditForm.headers)
		if err != nil {
			m.objectOperationErr = fmt.Errorf("headers: %w", err)
			return m, nil
		}
		name, description := m.objectEditForm.name, m.objectEditForm.description
		patch := objectstore.EditObject{
			Bucket: m.objectEditForm.bucket, Name: m.objectEditForm.currentName,
			NewName: &name, Description: &description, Metadata: &metadata, Headers: &headers,
		}
		m.objectEditForm = nil
		m.objectOperationRunning = true
		m.objectOperationName = patch.Name
		m.objectOperationErr = nil
		return m, m.editObject(patch)
	case "backspace":
		field := m.objectEditField()
		if runes := []rune(*field); len(runes) > 0 {
			*field = string(runes[:len(runes)-1])
		}
	default:
		if key.Type == tea.KeyRunes {
			*m.objectEditField() += string(key.Runes)
		}
	}
	return m, nil
}

func (m *Model) objectEditField() *string {
	switch m.objectEditForm.field {
	case 0:
		return &m.objectEditForm.name
	case 1:
		return &m.objectEditForm.description
	case 2:
		return &m.objectEditForm.metadata
	default:
		return &m.objectEditForm.headers
	}
}

func (m Model) objectEditFormView(width, height int) string {
	form := m.objectEditForm
	fields := []struct{ label, value, help string }{
		{"Name", form.name, "Rename safely; existing object names are rejected."},
		{"Description", form.description, "Human context for operators. Empty clears it."},
		{"Metadata", form.metadata, "Comma-separated key=value pairs. Empty clears all."},
		{"Headers", form.headers, "Comma-separated name=value pairs. Empty clears all."},
	}
	cardWidth := min(max(width-14, 48), 88)
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("Edit object metadata"),
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render(form.bucket + " / " + form.currentName), "",
	}
	for index, field := range fields {
		marker := "  "
		style := lipgloss.NewStyle().Foreground(m.theme.secondary)
		if index == form.field {
			marker = "› "
			style = style.Foreground(m.theme.accent).Bold(true)
		}
		value := field.value
		if value == "" {
			value = "(empty)"
		}
		if index == form.field {
			value += " ▏"
		}
		lines = append(lines, marker+lipgloss.NewStyle().Foreground(m.theme.muted).Width(15).Render(field.label)+style.Render(tailTruncate(value, max(cardWidth-19, 12))))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render(fields[form.field].help))
	if m.objectOperationErr != nil {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.red).Render(m.objectOperationErr.Error()))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("tab next  •  ctrl+s/enter save  •  esc cancel"))
	card := lipgloss.NewStyle().Width(cardWidth).Padding(2, 3).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}

func (m Model) updateObjectDeleteConfirmation(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.String() == "esc" || key.String() == "n" {
		m.confirmObjectDelete = false
		return m, nil
	}
	if key.String() != "y" && key.String() != "enter" {
		return m, nil
	}
	objects := m.displayedObjects()
	if m.deleteObject == nil || len(objects) == 0 {
		m.confirmObjectDelete = false
		m.objectOperationErr = fmt.Errorf("object deletion is unavailable")
		return m, nil
	}
	name := objects[m.objectObjectCursor].Name
	m.confirmObjectDelete = false
	m.objectOperationRunning = true
	m.objectOperationName = name
	return m, m.deleteObject(m.objectBucket, name)
}

func (m Model) objectDeleteConfirmationView(width, height int) string {
	object := m.displayedObjects()[m.objectObjectCursor]
	content := strings.Join([]string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.red).Render("Delete object permanently?"), "",
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.primary).Render(m.objectBucket + " / " + object.Name),
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render(fmt.Sprintf("%s across %d chunks will be removed.", humanBytes(object.Size), object.Chunks)), "",
		lipgloss.NewStyle().Foreground(m.theme.red).Render("This removes object data and cannot be undone from this UI."), "",
		lipgloss.NewStyle().Foreground(m.theme.muted).Render("y/enter confirm  •  n/esc cancel"),
	}, "\n")
	card := lipgloss.NewStyle().Width(min(max(width-14, 46), 76)).Padding(2, 3).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.red).Render(content)
	return m.centeredCanvas(width, height, card)
}

func (m *Model) upsertObject(object objectstore.ObjectSummary) {
	for index := range m.objects {
		if m.objects[index].Name == object.Name || m.objects[index].Name == m.objectOperationName {
			m.objects[index] = object
			m.rebuildObjectSearch()
			m.objectObjectCursor = findObject(m.displayedObjects(), object.Name)
			return
		}
	}
	// A rename no longer has the old name in the result; replace the selected
	// catalog entry, which is the object the form was opened from.
	if len(m.objects) > 0 {
		selected := selectedObjectName(m.displayedObjects(), m.objectObjectCursor)
		for index := range m.objects {
			if m.objects[index].Name == selected {
				m.objects[index] = object
				m.rebuildObjectSearch()
				m.objectObjectCursor = findObject(m.displayedObjects(), object.Name)
				return
			}
		}
	}
	m.objects = append(m.objects, object)
	m.rebuildObjectSearch()
}

func (m *Model) removeObject(name string) {
	for index := range m.objects {
		if m.objects[index].Name == name {
			m.objects = append(m.objects[:index], m.objects[index+1:]...)
			break
		}
	}
	m.rebuildObjectSearch()
	m.objectObjectCursor = min(m.objectObjectCursor, max(len(m.displayedObjects())-1, 0))
	m.showObjectDetail = false
}

func formatObjectProperties(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values[key])
	}
	return strings.Join(parts, ", ")
}

func formatObjectHeaders(values map[string][]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(values))
	for _, key := range keys {
		for _, value := range values[key] {
			parts = append(parts, key+"="+value)
		}
	}
	return strings.Join(parts, ", ")
}
