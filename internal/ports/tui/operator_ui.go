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

type OperatorServices struct {
	Load          func() tea.Cmd
	CreateAccount func(domain.AccountDefinition) tea.Cmd
	CreateUser    func(domain.UserDefinition) tea.Cmd
	UpdateAccount func(string, domain.AccountDefinition) tea.Cmd
	DeleteAccount func(string) tea.Cmd
	UpdateUser    func(string, domain.UserDefinition) tea.Cmd
	RevokeUser    func(string, string) tea.Cmd
	DeleteUser    func(string, string) tea.Cmd
	AddSigningKey func(string, domain.SigningKeyDefinition) tea.Cmd
}

type operatorAccountForm struct {
	field                                                                              int
	name, description, connections, subscriptions, streams, consumers, memory, storage string
	accountID                                                                          string
	baseLimits                                                                         domain.AccountLimits
}

func newOperatorAccountForm() operatorAccountForm {
	limits := domain.AccountLimits{Connections: -1, LeafNodes: -1, Subscriptions: -1, Payload: -1, Data: -1, Exports: -1, Imports: -1, Streams: -1, Consumers: -1, Memory: -1, Storage: -1}
	return operatorAccountForm{connections: "-1", subscriptions: "-1", streams: "-1", consumers: "-1", memory: "-1", storage: "-1", baseLimits: limits}
}

func editOperatorAccountForm(account app.Account) operatorAccountForm {
	return operatorAccountForm{accountID: account.ID, name: account.Name, description: account.Description, connections: strconv.FormatInt(account.Limits.Connections, 10), subscriptions: strconv.FormatInt(account.Limits.Subscriptions, 10), streams: strconv.FormatInt(account.Limits.Streams, 10), consumers: strconv.FormatInt(account.Limits.Consumers, 10), memory: strconv.FormatInt(account.Limits.Memory, 10), storage: strconv.FormatInt(account.Limits.Storage, 10), baseLimits: domain.AccountLimits{Connections: account.Limits.Connections, LeafNodes: account.Limits.LeafNodes, Subscriptions: account.Limits.Subscriptions, Payload: account.Limits.Payload, Data: account.Limits.Data, Exports: account.Limits.Exports, Imports: account.Limits.Imports, Streams: account.Limits.Streams, Consumers: account.Limits.Consumers, Memory: account.Limits.Memory, Storage: account.Limits.Storage}}
}

type operatorState struct {
	catalog     app.Catalog
	cursor      int
	detail      bool
	users       bool
	loading     bool
	err         error
	form        *operatorAccountForm
	userForm    *operatorUserForm
	signingForm *operatorSigningKeyForm
	confirm     *operatorConfirmation
	running     bool
	notice      string
}

type operatorLoadedMsg struct{ catalog app.Catalog }
type operatorFailedMsg struct{ err error }
type operatorAccountCreatedMsg struct{ artifacts app.AccountArtifacts }
type operatorAccountCreateFailedMsg struct{ err error }
type operatorUserCreatedMsg struct{ artifacts app.UserArtifacts }
type operatorUserCreateFailedMsg struct{ err error }
type operatorMutationSucceededMsg struct{ notice string }
type operatorMutationFailedMsg struct{ err error }

func OperatorLoaded(catalog app.Catalog) tea.Msg { return operatorLoadedMsg{catalog} }
func OperatorFailed(err error) tea.Msg           { return operatorFailedMsg{err} }
func OperatorAccountCreated(artifacts app.AccountArtifacts) tea.Msg {
	return operatorAccountCreatedMsg{artifacts}
}
func OperatorAccountCreateFailed(err error) tea.Msg { return operatorAccountCreateFailedMsg{err} }
func OperatorUserCreated(artifacts app.UserArtifacts) tea.Msg {
	return operatorUserCreatedMsg{artifacts}
}
func OperatorUserCreateFailed(err error) tea.Msg      { return operatorUserCreateFailedMsg{err} }
func OperatorMutationSucceeded(notice string) tea.Msg { return operatorMutationSucceededMsg{notice} }
func OperatorMutationFailed(err error) tea.Msg        { return operatorMutationFailedMsg{err} }

func (m Model) WithOperator(services OperatorServices) Model {
	m.operatorServices = services
	return m
}

func (m Model) startOperatorLoad() (tea.Model, tea.Cmd) {
	if m.operatorServices.Load == nil {
		m.operator.err = fmt.Errorf("operator catalog service is unavailable")
		return m, nil
	}
	m.operator.loading, m.operator.err = true, nil
	return m, m.operatorServices.Load()
}

func (m Model) updateOperatorKey(key tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if m.operator.confirm != nil {
		model, command := m.updateOperatorConfirmation(key)
		return model, command, true
	}
	if m.operator.signingForm != nil {
		model, command := m.updateOperatorSigningKeyForm(key)
		return model, command, true
	}
	if m.operator.userForm != nil {
		model, command := m.updateOperatorUserForm(key)
		return model, command, true
	}
	if m.operator.form != nil {
		model, command := m.updateOperatorAccountForm(key)
		return model, command, true
	}
	if m.operator.loading {
		return m, nil, true
	}
	accounts, users := m.operator.catalog.Accounts(), m.operator.catalog.Users()
	length := len(accounts)
	if m.operator.users {
		length = len(users)
	}
	switch key.String() {
	case "r":
		model, command := m.startOperatorLoad()
		return model, command, true
	case "up", "k":
		m.operator.cursor = max(m.operator.cursor-1, 0)
		return m, nil, true
	case "down", "j":
		m.operator.cursor = min(m.operator.cursor+1, max(length-1, 0))
		return m, nil, true
	case "u":
		m.operator.users = !m.operator.users
		m.operator.cursor, m.operator.detail = 0, false
		return m, nil, true
	case "n":
		capabilities := m.operator.catalog.Capabilities()
		if m.operator.users {
			accounts := managedOperatorAccounts(m.operator.catalog.Accounts())
			if !capabilities.LocalAuthority || len(accounts) == 0 {
				m.operator.err = fmt.Errorf("user creation requires a locally managed account")
				return m, nil, true
			}
			form := newOperatorUserForm(accounts)
			m.operator.userForm, m.operator.err, m.operator.notice = &form, nil, ""
			return m, nil, true
		}
		if !capabilities.ClaimsWrite || !capabilities.LocalAuthority {
			m.operator.err = fmt.Errorf("account creation requires --authority-dir, --authority-operator-seed, and system-account permissions")
			return m, nil, true
		}
		form := newOperatorAccountForm()
		m.operator.form, m.operator.err, m.operator.notice = &form, nil, ""
		return m, nil, true
	case "e":
		if length == 0 {
			return m, nil, true
		}
		if m.operator.users {
			user := users[min(m.operator.cursor, len(users)-1)]
			if user.Revoked {
				m.operator.err = fmt.Errorf("revoked users cannot be edited")
				return m, nil, true
			}
			form := editOperatorUserForm(managedOperatorAccounts(accounts), user)
			m.operator.userForm, m.operator.err, m.operator.notice = &form, nil, ""
			return m, nil, true
		}
		account := accounts[min(m.operator.cursor, len(accounts)-1)]
		if !account.Managed || !m.operator.catalog.Capabilities().ClaimsWrite {
			m.operator.err = fmt.Errorf("account editing requires a managed account and claim-write capability")
			return m, nil, true
		}
		form := editOperatorAccountForm(account)
		m.operator.form, m.operator.err, m.operator.notice = &form, nil, ""
		return m, nil, true
	case "d":
		if length == 0 {
			return m, nil, true
		}
		if m.operator.users {
			user := users[min(m.operator.cursor, len(users)-1)]
			m.operator.confirm = &operatorConfirmation{kind: "delete-user", accountID: user.Account, resourceID: user.PublicKey, label: user.Name}
		} else {
			account := accounts[min(m.operator.cursor, len(accounts)-1)]
			if !account.Managed {
				m.operator.err = fmt.Errorf("only locally managed accounts can be deleted")
				return m, nil, true
			}
			m.operator.confirm = &operatorConfirmation{kind: "delete-account", resourceID: account.ID, label: account.Name}
		}
		return m, nil, true
	case "x":
		if !m.operator.users || len(users) == 0 {
			return m, nil, false
		}
		user := users[min(m.operator.cursor, len(users)-1)]
		m.operator.confirm = &operatorConfirmation{kind: "revoke-user", accountID: user.Account, resourceID: user.PublicKey, label: user.Name}
		return m, nil, true
	case "s":
		if m.operator.users || len(accounts) == 0 {
			return m, nil, false
		}
		account := accounts[min(m.operator.cursor, len(accounts)-1)]
		if !account.Managed || !m.operator.catalog.Capabilities().ClaimsWrite {
			m.operator.err = fmt.Errorf("signing keys require a managed account and claim-write capability")
			return m, nil, true
		}
		form := newOperatorSigningKeyForm(account.ID)
		m.operator.signingForm, m.operator.err = &form, nil
		return m, nil, true
	case "enter":
		m.operator.detail = !m.operator.detail
		return m, nil, true
	case "esc":
		m.operator.detail = false
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) operatorView(width, height int) string {
	state := m.operator
	if state.confirm != nil {
		return m.operatorConfirmationView(width, height)
	}
	if state.signingForm != nil {
		return m.operatorSigningKeyFormView(width, height)
	}
	if state.userForm != nil {
		return m.operatorUserFormView(width, height)
	}
	if state.form != nil {
		return m.operatorAccountFormView(width, height)
	}
	if state.loading {
		return m.centeredCanvas(width, height, m.spinner.View()+" discovering operator accounts…")
	}
	if state.err != nil {
		return "Accounts & Users\n\n" + lipgloss.NewStyle().Foreground(m.theme.red).Render(state.err.Error()) + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render("Use system-account credentials or configure --monitor-url.  •  r retry")
	}
	accounts := state.catalog.Accounts()
	users := state.catalog.Users()
	capabilities := state.catalog.Capabilities()
	mode, modeColor := "ACCOUNT SCOPED", m.theme.yellow
	if capabilities.OperatorMode {
		mode, modeColor = "OPERATOR MODE", m.theme.mint
	}
	pane := "ACCOUNTS"
	if state.users {
		pane = "MANAGED USERS"
	}
	heading := lipgloss.NewStyle().Bold(true).Foreground(m.theme.primary).Render("Accounts & Users") + "  " + lipgloss.NewStyle().Bold(true).Foreground(modeColor).Render(mode) + "  " + lipgloss.NewStyle().Foreground(m.theme.accent).Render(pane) + "  " + lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("%d accounts  •  %d managed users", len(accounts), len(users)))
	if state.notice != "" {
		heading += "  " + lipgloss.NewStyle().Foreground(m.theme.mint).Render("✓ "+state.notice)
	}
	capabilityLine := fmt.Sprintf("system account %s  •  claims read %s  •  claims write %s  •  local authority %s", yesNo(capabilities.SystemAccount), yesNo(capabilities.ClaimsRead), yesNo(capabilities.ClaimsWrite), yesNo(capabilities.LocalAuthority))
	if state.users {
		return m.operatorUsersView(width, height, heading, capabilityLine, users)
	}
	if len(accounts) == 0 {
		return heading + "\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render(capabilityLine) + "\n\nNo operator accounts are visible to this connection."
	}
	cursor := min(state.cursor, len(accounts)-1)
	body := m.operatorTable(width, height-5, accounts, cursor)
	if state.detail || width < 96 {
		body = m.operatorDetail(width, accounts[cursor])
	}
	hint := lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ select  •  enter detail  •  n create  •  e edit  •  s signing key  •  d delete  •  u users  •  r refresh")
	return heading + "\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render(capabilityLine) + "\n\n" + body + "\n\n" + hint
}

func (m Model) updateOperatorAccountForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.operator.running {
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.operator.form, m.operator.err = nil, nil
	case "tab", "down":
		m.operator.form.field = (m.operator.form.field + 1) % 8
	case "shift+tab", "up":
		m.operator.form.field = (m.operator.form.field + 7) % 8
	case "ctrl+s":
		definition, err := m.operator.form.definition()
		if err != nil {
			m.operator.err = err
			return m, nil
		}
		service := m.operatorServices.CreateAccount
		if m.operator.form.accountID != "" && m.operatorServices.UpdateAccount != nil {
			service = func(definition domain.AccountDefinition) tea.Cmd {
				return m.operatorServices.UpdateAccount(m.operator.form.accountID, definition)
			}
		} else if m.operator.form.accountID != "" {
			service = nil
		}
		if service == nil {
			m.operator.err = fmt.Errorf("account mutation is unavailable")
			return m, nil
		}
		m.operator.running, m.operator.err = true, nil
		return m, service(definition)
	case "backspace":
		field := m.operator.form.textField()
		if runes := []rune(*field); len(runes) > 0 {
			*field = string(runes[:len(runes)-1])
		}
	default:
		if key.Type == tea.KeyRunes {
			field := m.operator.form.textField()
			*field += string(key.Runes)
		}
	}
	return m, nil
}

func (f *operatorAccountForm) textField() *string {
	return []*string{&f.name, &f.description, &f.connections, &f.subscriptions, &f.streams, &f.consumers, &f.memory, &f.storage}[f.field]
}

func (f operatorAccountForm) definition() (domain.AccountDefinition, error) {
	values := []*string{&f.connections, &f.subscriptions, &f.streams, &f.consumers, &f.memory, &f.storage}
	parsed := make([]int64, len(values))
	for index, value := range values {
		number, err := strconv.ParseInt(strings.TrimSpace(*value), 10, 64)
		if err != nil {
			return domain.AccountDefinition{}, fmt.Errorf("limits must be integers; use -1 for unlimited")
		}
		parsed[index] = number
	}
	limits := f.baseLimits
	limits.Connections, limits.Subscriptions, limits.Streams, limits.Consumers, limits.Memory, limits.Storage = parsed[0], parsed[1], parsed[2], parsed[3], parsed[4], parsed[5]
	definition, err := domain.NewAccountDefinition(f.name, limits)
	if err != nil {
		return domain.AccountDefinition{}, err
	}
	return definition.WithDescription(f.description), nil
}

func (m Model) operatorAccountFormView(width, height int) string {
	form := m.operator.form
	fields := []struct{ label, value, help string }{
		{"Name", form.name, "Human-readable account name, for example orders or payments-prod."},
		{"Description", form.description, "Optional purpose or ownership note."},
		{"Connections", form.connections, "Maximum simultaneous clients. -1 means unlimited."},
		{"Subscriptions", form.subscriptions, "Maximum subscriptions across the account. -1 means unlimited."},
		{"Streams", form.streams, "Maximum JetStream streams. -1 means unlimited."},
		{"Consumers", form.consumers, "Maximum JetStream consumers. -1 means unlimited."},
		{"Memory bytes", form.memory, "JetStream memory quota in bytes. -1 means unlimited."},
		{"Storage bytes", form.storage, "JetStream disk quota in bytes. -1 means unlimited."},
	}
	cardWidth := min(max(width-14, 42), 82)
	title, subtitle, action := "Create operator account", "The seed is saved privately before the claim is published.", "create"
	if form.accountID != "" {
		title, subtitle, action = "Edit operator account", "Advanced JWT configuration is preserved while these limits are updated.", "update"
	}
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(title), lipgloss.NewStyle().Foreground(m.theme.secondary).Render(subtitle)}
	for index, field := range fields {
		marker, color := "  ", m.theme.secondary
		if index == form.field {
			marker, color = "› ", m.theme.accent
		}
		value := field.value
		if value == "" {
			value = "…"
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(color).Bold(index == form.field).Render(fmt.Sprintf("%s%-18s %s", marker, field.label, value)))
	}
	lines = append(lines, "", lipgloss.NewStyle().Bold(true).Foreground(m.theme.cyan).Render("WHAT TO PUT HERE"), lipgloss.NewStyle().Width(cardWidth).Foreground(m.theme.secondary).Render(fields[form.field].help))
	if m.operator.err != nil {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.red).Render(m.operator.err.Error()))
	}
	if m.operator.running {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.yellow).Render(m.spinner.View()+" signing and publishing account…"))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("tab / ↑↓ field  •  ctrl+s "+action+"  •  esc cancel"))
	card := lipgloss.NewStyle().Width(cardWidth+4).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}

func (m Model) operatorUsersView(width, height int, heading, capabilityLine string, users []app.User) string {
	if len(users) == 0 {
		return heading + "\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render(capabilityLine) + "\n\nNo users are managed by the configured local authority.\n\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render("NATS cannot enumerate bearer JWTs globally.  •  u accounts  •  r refresh")
	}
	cursor := min(m.operator.cursor, len(users)-1)
	body := m.operatorUserTable(width, height-5, users, cursor)
	if m.operator.detail || width < 96 {
		body = m.operatorUserDetail(width, users[cursor])
	}
	hint := lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ select  •  enter detail  •  n create  •  e edit  •  x revoke  •  d delete  •  u accounts  •  r refresh")
	return heading + "\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render(capabilityLine) + "\n\n" + body + "\n\n" + hint
}

func (m Model) operatorUserTable(width, height int, users []app.User, cursor int) string {
	rows := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.muted).Render(fmt.Sprintf("  %-22s %-18s %-18s %8s %8s %9s", "USER", "PUBLIC KEY", "ACCOUNT", "PUB", "SUB", "STATUS"))}
	visible, start := max(height-1, 1), max(cursor-max(height-1, 1)+1, 0)
	for index := start; index < min(start+visible, len(users)); index++ {
		user, marker, color := users[index], "  ", m.theme.secondary
		if index == cursor {
			marker, color = "› ", m.theme.accent
		}
		status := "active"
		if user.Revoked {
			status = "revoked"
		}
		rows = append(rows, lipgloss.NewStyle().Foreground(color).Bold(index == cursor).Render(fmt.Sprintf("%s%-22s %-18s %-18s %8d %8d %9s", marker, truncate(user.Name, 22), truncate(user.PublicKey, 18), truncate(user.Account, 18), len(user.PublishAllow), len(user.SubscribeAllow), status)))
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(strings.Join(rows, "\n"))
}

func (m Model) operatorUserDetail(width int, user app.User) string {
	label, value := lipgloss.NewStyle().Bold(true).Foreground(m.theme.muted), lipgloss.NewStyle().Foreground(m.theme.primary)
	status := "active"
	if user.Revoked {
		status = "revoked"
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(user.Name) + "  " + status,
		label.Render("PUBLIC KEY") + "  " + value.Render(user.PublicKey),
		label.Render("ACCOUNT") + "  " + value.Render(user.Account),
		label.Render("ISSUER") + "  " + value.Render(valueOr(user.Issuer, "-")),
		label.Render("PUBLISH ALLOW") + "  " + value.Render(joinOr(user.PublishAllow, "none")),
		label.Render("PUBLISH DENY") + "  " + value.Render(joinOr(user.PublishDeny, "none")),
		label.Render("SUBSCRIBE ALLOW") + "  " + value.Render(joinOr(user.SubscribeAllow, "none")),
		label.Render("SUBSCRIBE DENY") + "  " + value.Render(joinOr(user.SubscribeDeny, "none")),
		label.Render("RESPONSES") + "  " + value.Render(fmt.Sprintf("max %d  •  ttl %s", user.ResponseMax, operatorDuration(user.ResponseTTL))),
		label.Render("SUBSCRIPTION LIMIT") + "  " + value.Render(fmt.Sprintf("%d", user.SubscriptionLimit)),
	}
	return lipgloss.NewStyle().Width(max(width-2, 1)).MaxHeight(30).Render(strings.Join(lines, "\n"))
}

func operatorDuration(nanoseconds int64) string {
	if nanoseconds <= 0 {
		return "unlimited"
	}
	return time.Duration(nanoseconds).String()
}

func (m Model) operatorTable(width, height int, accounts []app.Account, cursor int) string {
	header := lipgloss.NewStyle().Bold(true).Foreground(m.theme.muted).Render(fmt.Sprintf("  %-22s %-18s %7s %7s %9s %8s %8s", "ACCOUNT", "PUBLIC KEY", "CONNS", "SUBS", "JS", "IMPORTS", "EXPORTS"))
	rows := []string{header}
	visible := max(height-1, 1)
	start := max(cursor-visible+1, 0)
	for index := start; index < min(start+visible, len(accounts)); index++ {
		account := accounts[index]
		marker, color := "  ", m.theme.secondary
		if index == cursor {
			marker, color = "› ", m.theme.accent
		}
		jetstream := "off"
		if account.JetStream {
			jetstream = "on"
		}
		rows = append(rows, lipgloss.NewStyle().Foreground(color).Bold(index == cursor).Render(fmt.Sprintf("%s%-22s %-18s %7d %7d %9s %8d %8d", marker, truncate(account.Name, 22), truncate(account.ID, 18), account.Connections, account.Subscriptions, jetstream, len(account.Imports), len(account.Exports))))
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(strings.Join(rows, "\n"))
}

func (m Model) operatorDetail(width int, account app.Account) string {
	label := lipgloss.NewStyle().Bold(true).Foreground(m.theme.muted)
	value := lipgloss.NewStyle().Foreground(m.theme.primary)
	status := "active"
	if account.Expired {
		status = "expired"
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(account.Name) + "  " + status,
		label.Render("PUBLIC KEY") + "  " + value.Render(account.ID),
		label.Render("ISSUER") + "  " + value.Render(valueOr(account.Issuer, "-")),
		label.Render("LIVE USAGE") + "  " + value.Render(fmt.Sprintf("%d connections  •  %d subscriptions  •  %d leaf nodes", account.Connections, account.Subscriptions, account.LeafNodes)),
		label.Render("JETSTREAM ASSETS") + "  " + value.Render(fmt.Sprintf("%d streams  •  %d consumers  •  %d messages", account.Streams, account.Consumers, account.Messages)),
		label.Render("JETSTREAM USED") + "  " + value.Render(humanBytes(account.JetStreamMemory)+" memory  •  "+humanBytes(account.JetStreamStorage)+" disk"),
		label.Render("LIMITS") + "  " + value.Render(fmt.Sprintf("connections %d  •  subscriptions %d  •  streams %d  •  consumers %d", account.Limits.Connections, account.Limits.Subscriptions, account.Limits.Streams, account.Limits.Consumers)),
		label.Render("STORAGE") + "  " + value.Render(operatorLimitBytes(account.Limits.Memory)+" memory  •  "+operatorLimitBytes(account.Limits.Storage)+" disk"),
		label.Render("SIGNING KEYS") + "  " + value.Render(joinOr(account.SigningKeys, "none")),
		label.Render("IMPORTS") + "  " + value.Render(fmt.Sprintf("%d", len(account.Imports))),
	}
	for _, key := range account.SigningKeyDetails {
		kind := "regular"
		if key.Scoped {
			kind = "scoped  •  role " + key.Role
		}
		lines = append(lines, "  ◇ "+truncate(key.PublicKey, 24)+"  "+kind)
		if key.Scoped && (len(key.PublishAllow) > 0 || len(key.SubscribeAllow) > 0) {
			lines = append(lines, "    pub "+joinOr(key.PublishAllow, "none")+"  •  sub "+joinOr(key.SubscribeAllow, "none"))
		}
	}
	for _, sharing := range account.Imports {
		lines = append(lines, "  ← "+sharing.Subject+"  from "+valueOr(sharing.Account, "activation token"))
	}
	lines = append(lines, label.Render("EXPORTS")+"  "+value.Render(fmt.Sprintf("%d", len(account.Exports))))
	for _, sharing := range account.Exports {
		visibility := "token"
		if sharing.Public {
			visibility = "public"
		}
		lines = append(lines, "  → "+sharing.Subject+"  "+visibility)
	}
	lines = append(lines, label.Render("ACTIVATION TOKENS")+"  "+value.Render(fmt.Sprintf("%d", len(account.ActivationTokens))))
	return lipgloss.NewStyle().Width(max(width-2, 1)).MaxHeight(30).Render(strings.Join(lines, "\n"))
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func joinOr(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return strings.Join(values, ", ")
}

func operatorLimitBytes(value int64) string {
	if value < 0 {
		return "unlimited"
	}
	return humanBytes(uint64(value))
}
