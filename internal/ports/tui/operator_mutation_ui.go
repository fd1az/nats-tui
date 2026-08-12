package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	domain "github.com/fd1az/nats-tui/internal/domain/authority"
)

type operatorSigningKeyForm struct {
	accountID                                                                   string
	field                                                                       int
	scoped                                                                      bool
	role, description, publishAllow, publishDeny, subscribeAllow, subscribeDeny string
}

func newOperatorSigningKeyForm(accountID string) operatorSigningKeyForm {
	return operatorSigningKeyForm{accountID: accountID}
}

type operatorConfirmation struct {
	kind, accountID, resourceID, label string
}

func (m Model) updateOperatorSigningKeyForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.operator.running {
		return m, nil
	}
	form := m.operator.signingForm
	switch key.String() {
	case "esc":
		m.operator.signingForm, m.operator.err = nil, nil
	case "tab", "down":
		form.field = (form.field + 1) % 7
	case "shift+tab", "up":
		form.field = (form.field + 6) % 7
	case "left", "right", " ":
		if form.field == 0 {
			form.scoped = !form.scoped
		}
	case "ctrl+s":
		definition, err := domain.NewSigningKeyDefinition(form.role, form.description, form.scoped, domain.Permissions{PublishAllow: splitOperatorSubjects(form.publishAllow), PublishDeny: splitOperatorSubjects(form.publishDeny), SubscribeAllow: splitOperatorSubjects(form.subscribeAllow), SubscribeDeny: splitOperatorSubjects(form.subscribeDeny), SubscriptionLimit: -1})
		if err != nil {
			m.operator.err = err
			return m, nil
		}
		if m.operatorServices.AddSigningKey == nil {
			m.operator.err = fmt.Errorf("signing key creation is unavailable")
			return m, nil
		}
		m.operator.running, m.operator.err = true, nil
		return m, m.operatorServices.AddSigningKey(form.accountID, definition)
	case "backspace":
		if field := form.textField(); field != nil {
			if runes := []rune(*field); len(runes) > 0 {
				*field = string(runes[:len(runes)-1])
			}
		}
	default:
		if key.Type == tea.KeyRunes {
			if field := form.textField(); field != nil {
				*field += string(key.Runes)
			}
		}
	}
	return m, nil
}

func (f *operatorSigningKeyForm) textField() *string {
	return []*string{nil, &f.role, &f.description, &f.publishAllow, &f.publishDeny, &f.subscribeAllow, &f.subscribeDeny}[f.field]
}

func (m Model) operatorSigningKeyFormView(width, height int) string {
	f := m.operator.signingForm
	kind := "REGULAR"
	if f.scoped {
		kind = "SCOPED"
	}
	fields := []struct{ label, value, help string }{
		{"Type", kind, "Regular keys can sign any user for the account. Scoped keys enforce this permission template."},
		{"Role", f.role, "Required for scoped keys; use a concise operational role such as orders-reader."},
		{"Description", f.description, "Optional ownership or purpose note."},
		{"Publish allow", f.publishAllow, "Comma-separated publish subjects allowed by the scope."},
		{"Publish deny", f.publishDeny, "Optional publish deny rules."},
		{"Subscribe allow", f.subscribeAllow, "Comma-separated subscribe subjects allowed by the scope."},
		{"Subscribe deny", f.subscribeDeny, "Optional subscribe deny rules."},
	}
	cardWidth := min(max(width-14, 46), 88)
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("Add account signing key"), lipgloss.NewStyle().Foreground(m.theme.secondary).Render("The private seed is stored locally with mode 0600 and never rendered.")}
	for index, field := range fields {
		marker, color, value := "  ", m.theme.secondary, field.value
		if index == f.field {
			marker, color = "› ", m.theme.accent
		}
		if value == "" {
			value = "…"
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(color).Bold(index == f.field).Render(fmt.Sprintf("%s%-18s %s", marker, field.label, truncate(value, max(cardWidth-22, 12)))))
	}
	lines = append(lines, "", lipgloss.NewStyle().Bold(true).Foreground(m.theme.cyan).Render("WHAT TO PUT HERE"), lipgloss.NewStyle().Width(cardWidth).Foreground(m.theme.secondary).Render(fields[f.field].help))
	if m.operator.err != nil {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.red).Render(m.operator.err.Error()))
	}
	if m.operator.running {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.yellow).Render(m.spinner.View()+" signing and publishing key…"))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("tab / ↑↓ field  •  ←/→ type  •  ctrl+s create  •  esc cancel"))
	card := lipgloss.NewStyle().Width(cardWidth+4).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}

func (m Model) updateOperatorConfirmation(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.operator.running {
		return m, nil
	}
	confirmation := m.operator.confirm
	switch key.String() {
	case "esc", "n":
		m.operator.confirm, m.operator.err = nil, nil
		return m, nil
	case "y", "enter":
		var command tea.Cmd
		switch confirmation.kind {
		case "delete-account":
			if m.operatorServices.DeleteAccount != nil {
				command = m.operatorServices.DeleteAccount(confirmation.resourceID)
			}
		case "revoke-user":
			if m.operatorServices.RevokeUser != nil {
				command = m.operatorServices.RevokeUser(confirmation.accountID, confirmation.resourceID)
			}
		case "delete-user":
			if m.operatorServices.DeleteUser != nil {
				command = m.operatorServices.DeleteUser(confirmation.accountID, confirmation.resourceID)
			}
		}
		if command == nil {
			m.operator.err = fmt.Errorf("operator mutation is unavailable")
			return m, nil
		}
		m.operator.running, m.operator.err = true, nil
		return m, command
	}
	return m, nil
}

func (m Model) operatorConfirmationView(width, height int) string {
	c := m.operator.confirm
	action := map[string]string{"delete-account": "Delete account", "revoke-user": "Revoke user", "delete-user": "Delete user"}[c.kind]
	detail := "The server claim will be changed first. Local files are archived in .trash and remain recoverable."
	if c.kind == "revoke-user" {
		detail = "A revocation timestamp will be added to the account claim and published to NATS."
	}
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.red).Render(action), lipgloss.NewStyle().Bold(true).Foreground(m.theme.primary).Render(c.label), "", lipgloss.NewStyle().Width(64).Foreground(m.theme.secondary).Render(detail)}
	if m.operator.err != nil {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.red).Render(m.operator.err.Error()))
	}
	if m.operator.running {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.yellow).Render(m.spinner.View()+" applying claim mutation…"))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("y / enter confirm  •  n / esc cancel"))
	card := lipgloss.NewStyle().Width(min(max(width-14, 44), 72)).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.red).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}
