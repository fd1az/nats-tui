package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	app "github.com/fd1az/nats-tui/internal/app/operator"
	domain "github.com/fd1az/nats-tui/internal/domain/authority"
)

const operatorUserFieldCount = 9

type operatorUserForm struct {
	accounts                                                       []app.Account
	account                                                        int
	field                                                          int
	name, publishAllow, publishDeny, subscribeAllow, subscribeDeny string
	responseMax, responseTTL, subscriptions                        string
	userID                                                         string
}

func newOperatorUserForm(accounts []app.Account) operatorUserForm {
	return operatorUserForm{accounts: append([]app.Account(nil), accounts...), responseMax: "0", responseTTL: "0", subscriptions: "-1"}
}

func editOperatorUserForm(accounts []app.Account, user app.User) operatorUserForm {
	form := newOperatorUserForm(accounts)
	form.userID, form.name = user.PublicKey, user.Name
	form.publishAllow, form.publishDeny = strings.Join(user.PublishAllow, ", "), strings.Join(user.PublishDeny, ", ")
	form.subscribeAllow, form.subscribeDeny = strings.Join(user.SubscribeAllow, ", "), strings.Join(user.SubscribeDeny, ", ")
	form.responseMax, form.responseTTL, form.subscriptions = strconv.Itoa(user.ResponseMax), operatorDuration(user.ResponseTTL), strconv.FormatInt(user.SubscriptionLimit, 10)
	if user.ResponseTTL == 0 {
		form.responseTTL = "0"
	}
	for index, account := range form.accounts {
		if account.ID == user.Account {
			form.account = index
			break
		}
	}
	return form
}

func managedOperatorAccounts(accounts []app.Account) []app.Account {
	result := make([]app.Account, 0, len(accounts))
	for _, account := range accounts {
		if account.Managed {
			result = append(result, account)
		}
	}
	return result
}

func (m Model) updateOperatorUserForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.operator.running {
		return m, nil
	}
	form := m.operator.userForm
	switch key.String() {
	case "esc":
		m.operator.userForm, m.operator.err = nil, nil
	case "tab", "down":
		form.field = (form.field + 1) % operatorUserFieldCount
	case "shift+tab", "up":
		form.field = (form.field + operatorUserFieldCount - 1) % operatorUserFieldCount
	case "left":
		if form.field == 0 {
			form.account = (form.account + len(form.accounts) - 1) % len(form.accounts)
		}
	case "right", " ":
		if form.field == 0 {
			form.account = (form.account + 1) % len(form.accounts)
		}
	case "ctrl+s":
		definition, err := form.definition()
		if err != nil {
			m.operator.err = err
			return m, nil
		}
		service := m.operatorServices.CreateUser
		if form.userID != "" {
			if m.operatorServices.UpdateUser == nil {
				m.operator.err = fmt.Errorf("user update is unavailable")
				return m, nil
			}
			service = func(definition domain.UserDefinition) tea.Cmd {
				return m.operatorServices.UpdateUser(form.userID, definition)
			}
		}
		if service == nil {
			m.operator.err = fmt.Errorf("user mutation is unavailable")
			return m, nil
		}
		m.operator.running, m.operator.err = true, nil
		return m, service(definition)
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

func (f *operatorUserForm) textField() *string {
	fields := []*string{nil, &f.name, &f.publishAllow, &f.publishDeny, &f.subscribeAllow, &f.subscribeDeny, &f.responseMax, &f.responseTTL, &f.subscriptions}
	return fields[f.field]
}

func (f operatorUserForm) definition() (domain.UserDefinition, error) {
	if len(f.accounts) == 0 {
		return domain.UserDefinition{}, fmt.Errorf("a locally managed account is required")
	}
	responseMax, err := strconv.Atoi(strings.TrimSpace(f.responseMax))
	if err != nil {
		return domain.UserDefinition{}, fmt.Errorf("response max must be an integer")
	}
	responseTTL := time.Duration(0)
	if strings.TrimSpace(f.responseTTL) != "0" {
		responseTTL, err = time.ParseDuration(strings.TrimSpace(f.responseTTL))
		if err != nil {
			return domain.UserDefinition{}, fmt.Errorf("response TTL must look like 1s or 30s")
		}
	}
	subscriptions, err := strconv.ParseInt(strings.TrimSpace(f.subscriptions), 10, 64)
	if err != nil {
		return domain.UserDefinition{}, fmt.Errorf("subscription limit must be an integer")
	}
	return domain.NewUserDefinition(f.name, f.accounts[f.account].ID, domain.Permissions{PublishAllow: splitOperatorSubjects(f.publishAllow), PublishDeny: splitOperatorSubjects(f.publishDeny), SubscribeAllow: splitOperatorSubjects(f.subscribeAllow), SubscribeDeny: splitOperatorSubjects(f.subscribeDeny), ResponseMax: responseMax, ResponseTTL: responseTTL, SubscriptionLimit: subscriptions})
}

func splitOperatorSubjects(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts, result := strings.Split(value, ","), make([]string, 0)
	for _, part := range parts {
		if subject := strings.TrimSpace(part); subject != "" {
			result = append(result, subject)
		}
	}
	return result
}

func (m Model) operatorUserFormView(width, height int) string {
	form := m.operator.userForm
	account := form.accounts[form.account]
	fields := []struct{ label, value, help string }{
		{"Account", account.Name + "  " + account.ID, "Choose a locally managed account with ←/→. Its seed stays inside the authority adapter."},
		{"Name", form.name, "Human-readable user name, for example orders-worker."},
		{"Publish allow", form.publishAllow, "Comma-separated subjects the user may publish, such as orders.*, audit.created."},
		{"Publish deny", form.publishDeny, "Optional deny rules applied after publish allow."},
		{"Subscribe allow", form.subscribeAllow, "Comma-separated subjects the user may subscribe to."},
		{"Subscribe deny", form.subscribeDeny, "Optional subscription deny rules."},
		{"Response max", form.responseMax, "Maximum replies allowed by response permissions. 0 disables response permission."},
		{"Response TTL", form.responseTTL, "Response permission lifetime such as 2s. Use 0 when disabled."},
		{"Subscriptions", form.subscriptions, "Maximum subscriptions for this user. -1 means unlimited."},
	}
	cardWidth := min(max(width-14, 46), 88)
	title, subtitle, action := "Create managed user", "A private .creds file will be generated; no seed enters the UI.", "create"
	if form.userID != "" {
		title, subtitle, action = "Edit managed user", "The user NKey is retained and its private credentials are replaced atomically.", "update"
	}
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(title), lipgloss.NewStyle().Foreground(m.theme.secondary).Render(subtitle)}
	for index, field := range fields {
		marker, color, value := "  ", m.theme.secondary, field.value
		if index == form.field {
			marker, color = "› ", m.theme.accent
		}
		if value == "" {
			value = "…"
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(color).Bold(index == form.field).Render(fmt.Sprintf("%s%-18s %s", marker, field.label, truncate(value, max(cardWidth-22, 12)))))
	}
	lines = append(lines, "", lipgloss.NewStyle().Bold(true).Foreground(m.theme.cyan).Render("WHAT TO PUT HERE"), lipgloss.NewStyle().Width(cardWidth).Foreground(m.theme.secondary).Render(fields[form.field].help))
	if m.operator.err != nil {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.red).Render(m.operator.err.Error()))
	}
	if m.operator.running {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.yellow).Render(m.spinner.View()+" signing user credentials…"))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("tab / ↑↓ field  •  ←/→ account  •  ctrl+s "+action+"  •  esc cancel"))
	card := lipgloss.NewStyle().Width(cardWidth+4).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}
