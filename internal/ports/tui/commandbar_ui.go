package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	app "github.com/fd1az/nats-tui/internal/app/commandbar"
	"github.com/fd1az/nats-tui/internal/app/consumers"
	domain "github.com/fd1az/nats-tui/internal/domain/commandbar"
	domainrequestreply "github.com/fd1az/nats-tui/internal/domain/requestreply"
)

type CommandBarServices struct {
	Load   func() tea.Cmd
	Record func(string, string, time.Time, time.Time, error) tea.Cmd
}
type commandBarStage uint8

const (
	commandPalette commandBarStage = iota
	commandParameters
	commandPreview
	commandHistory
)

type commandBarState struct {
	open          bool
	stage         commandBarStage
	query         string
	cursor, field int
	custom        []app.Entry
	entries       []app.Entry
	index         *app.Index
	history       []app.HistoryView
	selected      *app.Entry
	values        map[string]string
	preview       app.PreviewView
	err           error
	notice        string
}
type commandBarLoadedMsg struct {
	custom  []app.Entry
	history []app.HistoryView
}
type commandBarFailedMsg struct{ err error }
type commandRecordedMsg struct{}

func CommandBarLoaded(custom []app.Entry, history []app.HistoryView) tea.Msg {
	return commandBarLoadedMsg{custom: custom, history: history}
}
func CommandBarFailed(err error) tea.Msg { return commandBarFailedMsg{err} }
func CommandRecorded() tea.Msg           { return commandRecordedMsg{} }
func (m Model) WithCommandBar(services CommandBarServices) Model {
	m.commandBarServices = services
	return m
}

func (m *Model) openCommandBar(stage commandBarStage) {
	m.commandBar.open = true
	m.commandBar.stage = stage
	m.commandBar.query = ""
	m.commandBar.cursor = 0
	m.commandBar.field = 0
	m.commandBar.selected = nil
	m.commandBar.values = nil
	m.commandBar.err = nil
	m.commandBar.notice = ""
	m.commandBar.entries = builtinAndCustomCommands(*m)
	m.commandBar.index = app.NewIndex(m.commandBar.entries)
}
func (m Model) commandEntries() []app.Entry {
	return m.commandBar.index.Search(m.commandBar.query, 12)
}
func builtinCommandEntries(m Model) []app.Entry {
	var result []app.Entry
	add := func(input domain.DefinitionInput) {
		definition, err := domain.NewDefinition(input)
		if err == nil {
			result = append(result, app.EntryFrom(definition))
		}
	}
	workspaces := []struct {
		id, title string
		index     int
	}{{"dashboard", "Dashboard", 0}, {"streams", "Streams", 1}, {"consumers", "Consumers", 2}, {"messages", "Messages", 3}, {"kv", "Key-Value Stores", 4}, {"objects", "Object Stores", 5}, {"subjects", "Subject Explorer", 6}, {"publish", "Publish / Request Lab", 7}, {"topology", "Server Topology", 8}, {"connections", "Connections", 9}, {"accounts", "Accounts & Users", 10}, {"alerts", "Alerts & Advisories", 11}}
	for _, workspace := range workspaces {
		add(domain.DefinitionInput{ID: "go-" + workspace.id, Title: "Go to " + workspace.title, Description: "Open workspace", Group: "Navigate", Action: "navigate.workspace", Arguments: map[string]string{"index": fmt.Sprint(workspace.index)}, Equivalent: []string{"nats-tui"}})
	}
	for _, stream := range m.streams {
		name := stream.Name
		add(domain.DefinitionInput{ID: "stream-" + slugCommandID(name), Title: "Go to stream " + name, Description: strings.Join(stream.Subjects, ", "), Group: "Navigate", Action: "navigate.stream", Arguments: map[string]string{"stream": name}, Equivalent: []string{"nats", "stream", "info", name}})
		add(domain.DefinitionInput{ID: "watch-stream-" + slugCommandID(name), Title: "Watch messages in " + name, Description: "Open recent messages and live watch controls", Group: "Inspect", Action: "navigate.messages", Arguments: map[string]string{"stream": name}, Equivalent: []string{"nats", "stream", "view", name}})
		add(domain.DefinitionInput{ID: "delete-stream-" + slugCommandID(name), Title: "Delete stream " + name, Description: fmt.Sprintf("Permanently remove %d stored messages", stream.Messages), Group: "Operate", Action: "operate.stream.delete", Risk: domain.RiskDestructive, Arguments: map[string]string{"stream": name}, Equivalent: []string{"nats", "stream", "rm", name}})
	}
	for _, consumer := range m.consumers {
		id := slugCommandID(consumer.Stream + "-" + consumer.Name)
		add(domain.DefinitionInput{ID: "consumer-" + id, Title: "Go to consumer " + consumer.Name, Description: consumer.Stream + " / " + consumer.Name, Group: "Navigate", Action: "navigate.consumer", Arguments: map[string]string{"stream": consumer.Stream, "consumer": consumer.Name}, Equivalent: []string{"nats", "consumer", "info", consumer.Stream, consumer.Name}})
		add(domain.DefinitionInput{ID: "delete-consumer-" + id, Title: "Delete consumer " + consumer.Name, Description: consumer.Stream + " / " + consumer.Name, Group: "Operate", Action: "operate.consumer.delete", Risk: domain.RiskDestructive, Arguments: map[string]string{"stream": consumer.Stream, "consumer": consumer.Name}, Equivalent: []string{"nats", "consumer", "rm", consumer.Stream, consumer.Name}})
	}
	add(domain.DefinitionInput{ID: "create-stream", Title: "Create stream", Description: "Open the guided stream form", Group: "Create", Action: "form.stream.create", Risk: domain.RiskMutation, Equivalent: []string{"nats", "stream", "add"}})
	add(domain.DefinitionInput{ID: "create-kv", Title: "Create Key-Value bucket", Description: "Open the guided KV form", Group: "Create", Action: "form.kv.create", Risk: domain.RiskMutation, Equivalent: []string{"nats", "kv", "add"}})
	add(domain.DefinitionInput{ID: "create-object-bucket", Title: "Create Object Store bucket", Description: "Open the guided object bucket form", Group: "Create", Action: "form.object.create", Risk: domain.RiskMutation, Equivalent: []string{"nats", "object", "add"}})
	add(domain.DefinitionInput{ID: "scan-alerts", Title: "Scan server health", Description: "Evaluate local alert thresholds now", Group: "Operate", Action: "operate.alerts.scan", Risk: domain.RiskMutation, Equivalent: []string{"nats-tui", "alerts", "scan"}})
	add(domain.DefinitionInput{ID: "publish-message", Title: "Publish Core NATS message", Description: "Validate and publish one message", Group: "Operate", Action: "operate.core.publish", Risk: domain.RiskMutation, Parameters: []domain.ParameterInput{{Name: "subject", Help: "Concrete subject without wildcards", Required: true}, {Name: "payload", Help: "JSON or plain-text payload", Default: "{}"}}, Arguments: map[string]string{"subject": "{{subject}}", "payload": "{{payload}}"}, Equivalent: []string{"nats", "pub", "{{subject}}", "{{payload}}"}})
	add(domain.DefinitionInput{ID: "send-request", Title: "Send Core NATS request", Description: "Wait for one reply with a bounded timeout", Group: "Operate", Action: "operate.core.request", Risk: domain.RiskMutation, Parameters: []domain.ParameterInput{{Name: "subject", Help: "Concrete request subject", Required: true}, {Name: "payload", Help: "JSON or plain-text payload", Default: "{}"}, {Name: "timeout", Help: "Examples: 500ms, 2s", Default: "2s", Required: true}}, Arguments: map[string]string{"subject": "{{subject}}", "payload": "{{payload}}", "timeout": "{{timeout}}"}, Equivalent: []string{"nats", "request", "{{subject}}", "{{payload}}", "--timeout", "{{timeout}}"}})
	priority := map[string]int{"Navigate": 0, "Inspect": 1, "Create": 2, "Operate": 3, "Custom": 4}
	sort.SliceStable(result, func(i, j int) bool { return priority[result[i].Group] < priority[result[j].Group] })
	return result
}
func slugCommandID(value string) string {
	value = strings.ToLower(value)
	var result strings.Builder
	dash := false
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			result.WriteRune(r)
			dash = false
		} else if !dash && result.Len() > 0 {
			result.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(result.String(), "-")
}

func (m Model) updateCommandBar(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.commandBar
	switch s.stage {
	case commandPalette:
		return m.updateCommandPalette(key)
	case commandParameters:
		return m.updateCommandParameters(key)
	case commandPreview:
		return m.updateCommandPreview(key)
	case commandHistory:
		return m.updateCommandHistory(key)
	}
	return m, nil
}
func (m Model) updateCommandPalette(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.commandBar
	entries := m.commandEntries()
	switch key.String() {
	case "esc":
		s.open = false
		return m, nil
	case "ctrl+r":
		s.stage = commandHistory
		s.cursor = 0
		return m, nil
	case "up", "ctrl+p":
		s.cursor = max(s.cursor-1, 0)
		return m, nil
	case "down", "ctrl+n":
		s.cursor = min(s.cursor+1, max(len(entries)-1, 0))
		return m, nil
	case "backspace":
		r := []rune(s.query)
		if len(r) > 0 {
			s.query = string(r[:len(r)-1])
		}
		s.cursor = 0
		return m, nil
	case "ctrl+u":
		s.query = ""
		s.cursor = 0
		return m, nil
	case "enter":
		if len(entries) > 0 {
			return m.selectCommand(entries[s.cursor], false)
		}
		return m, nil
	case "ctrl+enter":
		if len(entries) > 0 {
			return m.selectCommand(entries[s.cursor], true)
		}
		return m, nil
	}
	if key.Type == tea.KeyRunes {
		s.query += string(key.Runes)
		s.cursor = 0
	}
	return m, nil
}
func (m Model) selectCommand(entry app.Entry, _ bool) (tea.Model, tea.Cmd) {
	m.commandBar.selected = &entry
	m.commandBar.values = map[string]string{}
	parameters := entry.Definition.Parameters()
	for _, parameter := range parameters {
		m.commandBar.values[parameter.Name()] = parameter.Default()
	}
	if len(parameters) > 0 {
		m.commandBar.stage = commandParameters
		m.commandBar.field = 0
		return m, nil
	}
	preview, err := app.Preview(entry.Definition, nil)
	if err != nil {
		m.commandBar.err = err
		return m, nil
	}
	m.commandBar.preview = preview
	if preview.RequiresConfirmation {
		m.commandBar.stage = commandPreview
		return m, nil
	}
	return m.executeCommand(entry, preview)
}
func (m Model) updateCommandParameters(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.commandBar
	if s.selected == nil {
		s.stage = commandPalette
		return m, nil
	}
	parameters := s.selected.Definition.Parameters()
	current := parameters[s.field]
	switch key.String() {
	case "esc":
		s.stage = commandPalette
		s.selected = nil
		return m, nil
	case "tab", "down":
		s.field = (s.field + 1) % len(parameters)
		return m, nil
	case "shift+tab", "up":
		s.field = (s.field - 1 + len(parameters)) % len(parameters)
		return m, nil
	case "backspace":
		r := []rune(s.values[current.Name()])
		if len(r) > 0 {
			s.values[current.Name()] = string(r[:len(r)-1])
		}
		return m, nil
	case "ctrl+u":
		s.values[current.Name()] = ""
		return m, nil
	case "ctrl+p", "ctrl+enter", "enter":
		preview, err := app.Preview(s.selected.Definition, s.values)
		if err != nil {
			s.err = err
			return m, nil
		}
		s.preview = preview
		if preview.RequiresConfirmation || key.String() != "ctrl+enter" {
			s.stage = commandPreview
			return m, nil
		}
		return m.executeCommand(*s.selected, preview)
	}
	if key.Type == tea.KeyRunes {
		s.values[current.Name()] += string(key.Runes)
	}
	return m, nil
}
func (m Model) updateCommandPreview(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.commandBar
	switch key.String() {
	case "esc":
		if s.selected != nil && len(s.selected.Definition.Parameters()) > 0 {
			s.stage = commandParameters
		} else {
			s.stage = commandPalette
		}
		return m, nil
	case "C":
		if m.copyValue != nil && s.preview.Command != "" {
			if err := m.copyValue(s.preview.Command); err != nil {
				s.err = err
			} else {
				s.notice = "Equivalent command copied"
			}
		}
		return m, nil
	case "y", "enter", "ctrl+enter":
		if s.selected != nil {
			return m.executeCommand(*s.selected, s.preview)
		}
	}
	return m, nil
}
func (m Model) updateCommandHistory(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.commandBar
	switch key.String() {
	case "esc":
		s.stage = commandPalette
		s.cursor = 0
		return m, nil
	case "up", "k":
		s.cursor = max(s.cursor-1, 0)
		return m, nil
	case "down", "j":
		s.cursor = min(s.cursor+1, max(len(s.history)-1, 0))
		return m, nil
	case "enter":
		if len(s.history) > 0 {
			target := s.history[s.cursor].CommandID
			for _, entry := range s.entries {
				if entry.ID == target {
					return m.selectCommand(entry, false)
				}
			}
			s.err = fmt.Errorf("command %s is no longer available", target)
		}
		return m, nil
	}
	return m, nil
}
func builtinAndCustomCommands(m Model) []app.Entry {
	result := builtinCommandEntries(m)
	return append(result, m.commandBar.custom...)
}
func (m Model) executeCommand(entry app.Entry, preview app.PreviewView) (tea.Model, tea.Cmd) {
	started := m.runtime.Now()
	m.commandBar.open = false
	m.commandBar.err = nil
	var command tea.Cmd
	var executionErr error
	args := preview.Arguments
	switch preview.Action {
	case "navigate.workspace":
		var index int
		if _, err := fmt.Sscan(args["index"], &index); err == nil && index >= 0 && index < len(workspaces) {
			m.activeWorkspace = index
		} else {
			executionErr = fmt.Errorf("invalid workspace")
		}
	case "navigate.stream":
		executionErr = m.selectStream(args["stream"])
	case "navigate.consumer":
		executionErr = m.selectConsumer(args["stream"], args["consumer"])
	case "navigate.messages":
		if stream := args["stream"]; stream != "" {
			if err := m.selectStream(stream); err != nil {
				executionErr = err
			} else if m.loadMessages != nil {
				m.loadingMessages = true
				command = m.loadMessages(stream)
			}
		} else if subject := args["subject"]; subject != "" && m.startWatch != nil {
			m.setMessages(nil)
			m.messageStream = subject
			m.watchSubjects = []string{subject}
			m.watchStarting = true
			m.activeWorkspace = 3
			command = m.startWatch(subject, []string{subject})
		} else {
			executionErr = fmt.Errorf("messages command requires stream or subject")
		}
	case "form.stream.create":
		form := newCreateStreamForm()
		m.streamForm = &form
		m.activeWorkspace = 1
	case "form.kv.create":
		form := newKeyValueForm()
		m.keyValueForm = &form
		m.activeWorkspace = 4
	case "form.object.create":
		form := newObjectBucketForm()
		m.objectBucketForm = &form
		m.activeWorkspace = 5
	case "operate.stream.delete":
		if err := m.selectStream(args["stream"]); err != nil {
			executionErr = err
		} else {
			updated, actionCommand := m.runStreamDelete()
			m, command = updated.(Model), actionCommand
		}
	case "operate.consumer.delete":
		if err := m.selectConsumer(args["stream"], args["consumer"]); err != nil {
			executionErr = err
		} else {
			updated, actionCommand := m.runConsumerAction(consumers.ActionDelete)
			m, command = updated.(Model), actionCommand
		}
	case "operate.alerts.scan":
		m.activeWorkspace = 11
		if m.alertServices.Scan != nil {
			m.alerts.loading = true
			command = m.alertServices.Scan()
		} else {
			executionErr = fmt.Errorf("alert scan is unavailable")
		}
	case "operate.core.publish":
		message, err := domainrequestreply.NewMessage(args["subject"], []byte(args["payload"]), nil)
		if err != nil {
			executionErr = err
		} else if m.requestReplyServices.Publish == nil {
			executionErr = fmt.Errorf("Core NATS publish is unavailable")
		} else {
			m.activeWorkspace = 7
			m.requestReply.running = true
			command = m.requestReplyServices.Publish(message)
		}
	case "operate.core.request":
		message, err := domainrequestreply.NewMessage(args["subject"], []byte(args["payload"]), nil)
		if err != nil {
			executionErr = err
			break
		}
		timeout, err := time.ParseDuration(args["timeout"])
		if err != nil {
			executionErr = fmt.Errorf("invalid request timeout: %w", err)
			break
		}
		request, err := domainrequestreply.NewRequest(message, timeout, 1)
		if err != nil {
			executionErr = err
		} else if m.requestReplyServices.Request == nil {
			executionErr = fmt.Errorf("Core NATS request is unavailable")
		} else {
			m.activeWorkspace = 7
			m.requestReply.running = true
			command = m.requestReplyServices.Request(request)
		}
	default:
		executionErr = fmt.Errorf("command action %q is not supported by this client", preview.Action)
	}
	var record tea.Cmd
	if m.commandBarServices.Record != nil {
		record = m.commandBarServices.Record(entry.ID, entry.Title, started, m.runtime.Now(), executionErr)
	}
	if executionErr != nil {
		m.commandBar.err = executionErr
		m.commandBar.open = true
		m.commandBar.stage = commandPreview
	}
	return m, tea.Batch(command, record)
}
func (m *Model) selectStream(name string) error {
	for i, stream := range m.streams {
		if stream.Name == name {
			m.streamCursor = i
			m.activeWorkspace = 1
			return nil
		}
	}
	return fmt.Errorf("stream %s is no longer available", name)
}
func (m *Model) selectConsumer(stream, name string) error {
	for i, consumer := range m.consumers {
		if consumer.Stream == stream && consumer.Name == name {
			m.consumerCursor = i
			m.activeWorkspace = 2
			return nil
		}
	}
	return fmt.Errorf("consumer %s/%s is no longer available", stream, name)
}

func (m Model) commandBarView(width, height int) string {
	header := m.header(max(width-4, 1))
	tabs := m.workspaceTabs(max(width-4, 1))
	cardWidth := min(max(width-16, 46), 96)
	card := m.commandBarCard(cardWidth, min(max(height-10, 10), 30))
	body := header + "\n" + tabs + "\n" + lipgloss.Place(max(width-4, 1), max(height-lipgloss.Height(header)-lipgloss.Height(tabs)-4, 1), lipgloss.Center, lipgloss.Center, card)
	return lipgloss.NewStyle().Width(width).Height(height).Foreground(m.theme.primary).Padding(1, 2).Render(body)
}
func (m Model) commandBarCard(width, height int) string {
	s := m.commandBar
	title := "Command Bar"
	content := ""
	hints := ""
	switch s.stage {
	case commandPalette:
		title = "Command Bar"
		entries := m.commandEntries()
		lines := []string{lipgloss.NewStyle().Foreground(m.theme.accent).Render("› " + s.query + "▏"), ""}
		lastGroup := ""
		for i, entry := range entries {
			if entry.Group != lastGroup {
				lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.muted).Render(strings.ToUpper(valueOr(entry.Group, "COMMANDS"))))
				lastGroup = entry.Group
			}
			marker := "  "
			style := lipgloss.NewStyle().Foreground(m.theme.secondary)
			if i == s.cursor {
				marker = "▸ "
				style = style.Foreground(m.theme.accent).Bold(true)
			}
			risk := ""
			if entry.Risk == domain.RiskDestructive {
				risk = "  destructive"
			}
			lines = append(lines, style.Render(marker+entry.Title+risk))
		}
		if len(entries) == 0 {
			lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.muted).Render("No matching commands"))
		}
		content = strings.Join(lines, "\n")
		hints = "↑↓ select   enter open   ctrl+r history   esc close"
	case commandParameters:
		title = s.selected.Title
		lines := []string{}
		for i, p := range s.selected.Definition.Parameters() {
			marker := "  "
			style := lipgloss.NewStyle().Foreground(m.theme.secondary)
			if i == s.field {
				marker = "› "
				style = style.Foreground(m.theme.accent)
			}
			required := ""
			if p.Required() {
				required = " *"
			}
			lines = append(lines, style.Render(fmt.Sprintf("%s%-14s %s%s", marker, p.Name(), s.values[p.Name()], required)))
			if i == s.field && p.Help() != "" {
				lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.muted).Render("  "+p.Help()))
			}
		}
		content = strings.Join(lines, "\n")
		hints = "tab next   ctrl+p preview   ctrl+enter run   esc back"
	case commandPreview:
		title = "Preview: " + s.preview.Title
		riskColor := m.theme.yellow
		if s.preview.Risk == domain.RiskDestructive {
			riskColor = m.theme.red
		}
		lines := []string{lipgloss.NewStyle().Bold(true).Foreground(riskColor).Render(strings.ToUpper(string(s.preview.Risk))), "", lipgloss.NewStyle().Foreground(m.theme.secondary).Render(s.selected.Description), "", "Action", s.preview.Action}
		keys := make([]string, 0, len(s.preview.Arguments))
		for k := range s.preview.Arguments {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			lines = append(lines, fmt.Sprintf("%-14s %s", k, s.preview.Arguments[k]))
		}
		if s.preview.Command != "" {
			lines = append(lines, "", "Equivalent command", lipgloss.NewStyle().Foreground(m.theme.cyan).Render(s.preview.Command))
		}
		content = strings.Join(lines, "\n")
		hints = "y/enter execute   C copy command   esc back"
	case commandHistory:
		title = "Command History"
		lines := []string{}
		for i, item := range s.history {
			marker := "  "
			style := lipgloss.NewStyle().Foreground(m.theme.secondary)
			if i == s.cursor {
				marker = "▸ "
				style = style.Foreground(m.theme.accent).Bold(true)
			}
			status := "✓"
			if item.Status == "failed" {
				status = "✕"
			}
			lines = append(lines, style.Render(fmt.Sprintf("%s%s  %s  %s", marker, status, item.StartedAt.Format("15:04:05"), item.Title)))
		}
		if len(lines) == 0 {
			lines = append(lines, "No command history yet")
		}
		content = strings.Join(lines, "\n")
		hints = "↑↓ select   enter reopen safely   esc commands"
	}
	if s.err != nil {
		content += "\n\n" + lipgloss.NewStyle().Foreground(m.theme.red).Render(s.err.Error())
	}
	if s.notice != "" {
		content += "\n\n" + lipgloss.NewStyle().Foreground(m.theme.mint).Render(s.notice)
	}
	return lipgloss.NewStyle().Width(max(width-2, 1)).MaxHeight(height).Padding(1, 2).BorderStyle(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(lipgloss.NewStyle().Bold(true).Foreground(m.theme.primary).Render(title) + "\n\n" + content + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render(hints))
}
