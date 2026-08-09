package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clcsolutions/nats-tui/internal/app/objectstore"
)

type objectLinkForm struct {
	field        int
	bucket       string
	name         string
	targetBucket string
	targetObject string
}

func newObjectLinkForm(bucket string) objectLinkForm { return objectLinkForm{bucket: bucket} }

func (m Model) updateObjectLinkForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.objectLinkForm = nil
		m.objectOperationErr = nil
	case "tab", "down":
		m.objectLinkForm.field = (m.objectLinkForm.field + 1) % 3
	case "shift+tab", "up":
		m.objectLinkForm.field = (m.objectLinkForm.field + 2) % 3
	case "ctrl+s", "enter":
		return m.submitObjectLink()
	case "backspace":
		field := m.objectLinkField()
		if runes := []rune(*field); len(runes) > 0 {
			*field = string(runes[:len(runes)-1])
		}
	default:
		if key.Type == tea.KeyRunes {
			*m.objectLinkField() += string(key.Runes)
		}
	}
	return m, nil
}

func (m *Model) objectLinkField() *string {
	switch m.objectLinkForm.field {
	case 0:
		return &m.objectLinkForm.name
	case 1:
		return &m.objectLinkForm.targetBucket
	default:
		return &m.objectLinkForm.targetObject
	}
}

func (m Model) submitObjectLink() (tea.Model, tea.Cmd) {
	if m.createObjectLink == nil {
		m.objectOperationErr = fmt.Errorf("object links are unavailable")
		return m, nil
	}
	definition := objectstore.LinkDefinition{
		Bucket: m.objectLinkForm.bucket, Name: m.objectLinkForm.name,
		TargetBucket: m.objectLinkForm.targetBucket, TargetObject: m.objectLinkForm.targetObject,
	}
	if strings.TrimSpace(definition.Name) == "" || strings.TrimSpace(definition.TargetBucket) == "" {
		m.objectOperationErr = fmt.Errorf("link name and target bucket are required")
		return m, nil
	}
	m.objectLinkForm = nil
	m.objectOperationRunning = true
	m.objectOperationErr = nil
	m.objectOperationName = definition.Name
	return m, m.createObjectLink(definition)
}

func (m Model) objectLinkFormView(width, height int) string {
	form := m.objectLinkForm
	fields := []struct{ label, value, help string }{
		{"Link name", form.name, "Name visible inside " + form.bucket + "."},
		{"Target bucket", form.targetBucket, "Existing Object Store bucket containing the target."},
		{"Target object", form.targetObject, "Leave target object empty to link the entire bucket."},
	}
	cardWidth := min(max(width-14, 50), 88)
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("Create Object Store link"),
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render("Destination  " + form.bucket), "",
		lipgloss.NewStyle().Foreground(m.theme.muted).Render("Leave target object empty to link the entire bucket."), "",
	}
	for index, field := range fields {
		marker := "  "
		style := lipgloss.NewStyle().Foreground(m.theme.secondary)
		if index == form.field {
			marker = "› "
			style = style.Foreground(m.theme.accent).Bold(true)
		}
		value := valueOr(field.value, "(empty)")
		if index == form.field {
			value += " ▏"
		}
		lines = append(lines, marker+lipgloss.NewStyle().Foreground(m.theme.muted).Width(17).Render(field.label)+style.Render(tailTruncate(value, max(cardWidth-21, 12))))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render(fields[form.field].help))
	if m.objectOperationErr != nil {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.red).Render(m.objectOperationErr.Error()))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("tab next  •  ctrl+s/enter create  •  esc cancel"))
	card := lipgloss.NewStyle().Width(cardWidth).Padding(2, 3).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}
