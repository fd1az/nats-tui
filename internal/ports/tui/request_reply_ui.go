package tui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	domain "github.com/clcsolutions/nats-tui/internal/domain/requestreply"
)

type PublishCoreFunc func(domain.Message) tea.Cmd
type SendCoreRequestFunc func(domain.Request) tea.Cmd
type LoadPayloadFileFunc func(string) tea.Cmd
type LoadRequestHistoryFunc func() tea.Cmd
type LoadRequestTemplatesFunc func() tea.Cmd
type SaveRequestTemplateFunc func(domain.Template) tea.Cmd
type DeleteRequestTemplateFunc func(string) tea.Cmd

type RequestReplyServices struct {
	Publish        PublishCoreFunc
	Request        SendCoreRequestFunc
	LoadPayload    LoadPayloadFileFunc
	LoadHistory    LoadRequestHistoryFunc
	LoadTemplates  LoadRequestTemplatesFunc
	SaveTemplate   SaveRequestTemplateFunc
	DeleteTemplate DeleteRequestTemplateFunc
}

type requestPane uint8

const (
	requestCompose requestPane = iota
	requestResults
	requestHistory
	requestTemplates
)

type requestReplyState struct {
	requestMode    bool
	pane           requestPane
	focus          int
	subject        textinput.Model
	headers        textinput.Model
	payload        textarea.Model
	file           textinput.Model
	timeout        textinput.Model
	maxReplies     textinput.Model
	template       textinput.Model
	running        bool
	error          error
	notice         string
	exchange       *domain.Exchange
	replyCursor    int
	history        []domain.Exchange
	historyCursor  int
	templates      []domain.Template
	templateCursor int
	loadingState   bool
	cliEquivalent  string
}

type corePublishSucceededMsg struct{ message domain.Message }
type corePublishFailedMsg struct{ err error }
type coreRequestSucceededMsg struct{ exchange domain.Exchange }
type coreRequestFailedMsg struct{ err error }
type payloadFileLoadedMsg struct {
	path    string
	payload []byte
}
type payloadFileFailedMsg struct {
	path string
	err  error
}
type requestHistoryLoadedMsg struct{ history []domain.Exchange }
type requestHistoryFailedMsg struct{ err error }
type requestTemplatesLoadedMsg struct{ templates []domain.Template }
type requestTemplatesFailedMsg struct{ err error }
type requestTemplateSavedMsg struct{ template domain.Template }
type requestTemplateSaveFailedMsg struct{ err error }
type requestTemplateDeletedMsg struct{ name string }
type requestTemplateDeleteFailedMsg struct{ err error }

func CorePublishSucceeded(message domain.Message) tea.Msg   { return corePublishSucceededMsg{message} }
func CorePublishFailed(err error) tea.Msg                   { return corePublishFailedMsg{err} }
func CoreRequestSucceeded(exchange domain.Exchange) tea.Msg { return coreRequestSucceededMsg{exchange} }
func CoreRequestFailed(err error) tea.Msg                   { return coreRequestFailedMsg{err} }
func PayloadFileLoaded(path string, payload []byte) tea.Msg {
	return payloadFileLoadedMsg{path, payload}
}
func PayloadFileFailed(path string, err error) tea.Msg       { return payloadFileFailedMsg{path, err} }
func RequestHistoryLoaded(history []domain.Exchange) tea.Msg { return requestHistoryLoadedMsg{history} }
func RequestHistoryFailed(err error) tea.Msg                 { return requestHistoryFailedMsg{err} }
func RequestTemplatesLoaded(templates []domain.Template) tea.Msg {
	return requestTemplatesLoadedMsg{templates}
}
func RequestTemplatesFailed(err error) tea.Msg              { return requestTemplatesFailedMsg{err} }
func RequestTemplateSaved(template domain.Template) tea.Msg { return requestTemplateSavedMsg{template} }
func RequestTemplateSaveFailed(err error) tea.Msg           { return requestTemplateSaveFailedMsg{err} }
func RequestTemplateDeleted(name string) tea.Msg            { return requestTemplateDeletedMsg{name} }
func RequestTemplateDeleteFailed(err error) tea.Msg         { return requestTemplateDeleteFailedMsg{err} }

func newRequestReplyState(colors theme) requestReplyState {
	state := requestReplyState{
		subject:    requestTextInput(colors, "events.payment.created"),
		headers:    requestTextInput(colors, "Content-Type=application/json, X-Trace=abc"),
		payload:    newRequestTextarea(colors),
		file:       requestTextInput(colors, "./payload.json"),
		timeout:    requestTextInput(colors, "2s"),
		maxReplies: requestTextInput(colors, "1"),
		template:   requestTextInput(colors, "lookup-user"),
	}
	state.timeout.SetValue("2s")
	state.maxReplies.SetValue("1")
	state.subject.Focus()
	return state
}

func requestTextInput(colors theme, placeholder string) textinput.Model {
	input := textinput.New()
	input.Placeholder = placeholder
	input.Prompt = "› "
	input.CharLimit = 4096
	input.PromptStyle = lipgloss.NewStyle().Foreground(colors.accent)
	input.TextStyle = lipgloss.NewStyle().Foreground(colors.primary)
	input.Cursor.Style = lipgloss.NewStyle().Foreground(colors.cyan)
	input.Cursor.SetMode(cursor.CursorHide)
	return input
}

func newRequestTextarea(colors theme) textarea.Model {
	input := textarea.New()
	input.Placeholder = "JSON, text, or any UTF-8 payload"
	input.Prompt = ""
	input.ShowLineNumbers = true
	input.Cursor.Style = lipgloss.NewStyle().Foreground(colors.cyan)
	input.Cursor.SetMode(cursor.CursorHide)
	for _, style := range []*textarea.Style{&input.FocusedStyle, &input.BlurredStyle} {
		style.Base = lipgloss.NewStyle()
		style.CursorLine = lipgloss.NewStyle().Foreground(colors.primary)
		style.CursorLineNumber = lipgloss.NewStyle().Foreground(colors.accent)
		style.EndOfBuffer = lipgloss.NewStyle().Foreground(colors.border)
		style.LineNumber = lipgloss.NewStyle().Foreground(colors.muted)
		style.Placeholder = lipgloss.NewStyle().Foreground(colors.muted)
		style.Text = lipgloss.NewStyle().Foreground(colors.primary)
	}
	return input
}

func (m Model) WithRequestReply(services RequestReplyServices) Model {
	m.requestReplyServices = services
	return m
}

func (m Model) updateRequestReplyKey(key tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if len(key.String()) == 1 && ((key.String()[0] >= '1' && key.String()[0] <= '9') || key.String()[0] == '0') {
		return m, nil, false
	}
	state := &m.requestReply
	if state.running || state.loadingState {
		return m, nil, true
	}
	if state.pane != requestCompose {
		return m.updateRequestNavigation(key)
	}
	switch key.String() {
	case "ctrl+r":
		state.requestMode = !state.requestMode
		state.notice, state.error = "", nil
		return m, nil, true
	case "ctrl+s":
		model, command := m.submitRequestCompose()
		return model, command, true
	case "ctrl+f":
		state.error = formatRequestJSON(&state.payload)
		return m, nil, true
	case "ctrl+l":
		if m.requestReplyServices.LoadPayload == nil {
			state.error = errors.New("payload file loading is unavailable")
			return m, nil, true
		}
		path := strings.TrimSpace(state.file.Value())
		if path == "" {
			state.error = errors.New("payload file path is required")
			return m, nil, true
		}
		state.running, state.error = true, nil
		return m, m.requestReplyServices.LoadPayload(path), true
	case "ctrl+t":
		model, command := m.saveRequestTemplate()
		return model, command, true
	case "ctrl+y":
		if state.cliEquivalent == "" || m.copyValue == nil {
			state.error = errors.New("no CLI equivalent is available yet")
			return m, nil, true
		}
		if err := m.copyValue(state.cliEquivalent); err != nil {
			state.error = fmt.Errorf("copy CLI equivalent: %w", err)
		} else {
			state.notice = "CLI equivalent copied"
		}
		return m, nil, true
	case "ctrl+h":
		model, command := m.loadRequestHistory()
		return model, command, true
	case "ctrl+o":
		model, command := m.loadRequestTemplates()
		return model, command, true
	case "tab", "shift+tab":
		step := 1
		if key.String() == "shift+tab" {
			step = -1
		}
		state.focus = (state.focus + step + 7) % 7
		m.focusRequestField()
		return m, nil, true
	}
	var command tea.Cmd
	switch state.focus {
	case 0:
		state.subject, command = state.subject.Update(key)
	case 1:
		state.headers, command = state.headers.Update(key)
	case 2:
		state.payload, command = state.payload.Update(key)
	case 3:
		state.file, command = state.file.Update(key)
	case 4:
		state.timeout, command = state.timeout.Update(key)
	case 5:
		state.maxReplies, command = state.maxReplies.Update(key)
	case 6:
		state.template, command = state.template.Update(key)
	}
	return m, command, true
}

func (m *Model) focusRequestField() {
	state := &m.requestReply
	state.subject.Blur()
	state.headers.Blur()
	state.payload.Blur()
	state.file.Blur()
	state.timeout.Blur()
	state.maxReplies.Blur()
	state.template.Blur()
	switch state.focus {
	case 0:
		state.subject.Focus()
	case 1:
		state.headers.Focus()
	case 2:
		state.payload.Focus()
	case 3:
		state.file.Focus()
	case 4:
		state.timeout.Focus()
	case 5:
		state.maxReplies.Focus()
	case 6:
		state.template.Focus()
	}
}

func (m Model) composeRequest() (domain.Request, error) {
	state := m.requestReply
	headers, err := parseObjectHeaders(state.headers.Value())
	if err != nil {
		return domain.Request{}, fmt.Errorf("headers: %w", err)
	}
	payload := []byte(state.payload.Value())
	if requestPayloadIsJSON(payload, headers) && !json.Valid(payload) {
		return domain.Request{}, errors.New("payload is not valid JSON; ctrl+f formats and validates it")
	}
	message, err := domain.NewMessage(state.subject.Value(), payload, headers)
	if err != nil {
		return domain.Request{}, err
	}
	timeout := time.Second
	maxReplies := 1
	if state.requestMode {
		timeout, err = time.ParseDuration(strings.TrimSpace(state.timeout.Value()))
		if err != nil {
			return domain.Request{}, errors.New("timeout must be a duration such as 2s or 500ms")
		}
		maxReplies, err = strconv.Atoi(strings.TrimSpace(state.maxReplies.Value()))
		if err != nil {
			return domain.Request{}, errors.New("max replies must be an integer")
		}
	}
	return domain.NewRequest(message, timeout, maxReplies)
}

func (m Model) submitRequestCompose() (tea.Model, tea.Cmd) {
	request, err := m.composeRequest()
	if err != nil {
		m.requestReply.error = err
		return m, nil
	}
	m.requestReply.error, m.requestReply.notice = nil, ""
	if m.requestReply.requestMode {
		if m.requestReplyServices.Request == nil {
			m.requestReply.error = errors.New("Core NATS request is unavailable")
			return m, nil
		}
		m.requestReply.running = true
		return m, m.requestReplyServices.Request(request)
	}
	if m.requestReplyServices.Publish == nil {
		m.requestReply.error = errors.New("Core NATS publish is unavailable")
		return m, nil
	}
	m.requestReply.running = true
	return m, m.requestReplyServices.Publish(request.Message())
}

func requestPayloadIsJSON(payload []byte, headers map[string][]string) bool {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return true
	}
	for name, values := range headers {
		if strings.EqualFold(name, "Content-Type") {
			for _, value := range values {
				if strings.Contains(strings.ToLower(value), "json") {
					return true
				}
			}
		}
	}
	return false
}

func formatRequestJSON(payload *textarea.Model) error {
	raw := []byte(payload.Value())
	if len(bytes.TrimSpace(raw)) == 0 {
		return errors.New("payload is empty")
	}
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, raw, "", "  "); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	payload.SetValue(formatted.String())
	return nil
}

func (m Model) requestReplyView(width, height int) string {
	switch m.requestReply.pane {
	case requestResults:
		return m.requestResultsView(width, height)
	case requestHistory:
		return m.requestHistoryView(width, height)
	case requestTemplates:
		return m.requestTemplatesView(width, height)
	default:
		return m.requestComposeView(width, height)
	}
}

func (m Model) requestComposeView(width, height int) string {
	state := m.requestReply
	// The workspace height already excludes the application chrome. At twenty
	// rows the complete form still fits; below that, render one focused field at
	// a time so the footer and validation feedback remain visible.
	if width < 64 || height < 20 {
		return m.compactRequestComposeView(width, height)
	}
	contentWidth := min(max(width-12, 44), 104)
	state.subject.Width = contentWidth - 4
	state.headers.Width = contentWidth - 4
	state.file.Width = contentWidth - 4
	state.timeout.Width = 16
	state.maxReplies.Width = 10
	state.template.Width = 28
	state.payload.SetWidth(contentWidth - 2)
	state.payload.SetHeight(max(min(height-22, 10), 4))
	mode := lipgloss.NewStyle().Bold(true).Foreground(m.theme.mint).Render("PUBLISH") + "  " + lipgloss.NewStyle().Foreground(m.theme.muted).Render("REQUEST")
	if state.requestMode {
		mode = lipgloss.NewStyle().Foreground(m.theme.muted).Render("PUBLISH") + "  " + lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("REQUEST")
	}
	label := lipgloss.NewStyle().Bold(true).Foreground(m.theme.muted)
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("Publish / Request Lab") + "   " + mode,
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render("Core NATS messages with validated subjects, editable headers, and Pretty JSON."), "",
		label.Render("SUBJECT") + "  " + state.subject.View(),
		label.Render("HEADERS") + "  " + state.headers.View(),
		label.Render("PAYLOAD"), state.payload.View(),
		label.Render("PAYLOAD FILE") + "  " + state.file.View(),
	}
	if state.requestMode {
		lines = append(lines, label.Render("TIMEOUT")+"  "+state.timeout.View()+"    "+label.Render("MAX REPLIES")+"  "+state.maxReplies.View())
	}
	lines = append(lines, label.Render("TEMPLATE NAME")+"  "+state.template.View())
	if state.error != nil {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.red).Render(state.error.Error()))
	}
	if state.notice != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.mint).Render("✓ "+state.notice))
	}
	if state.cliEquivalent != "" {
		lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(m.theme.muted).Render("CLI EQUIVALENT"), lipgloss.NewStyle().Foreground(m.theme.cyan).Render(state.cliEquivalent))
	}
	if state.running {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.yellow).Render(m.spinner.View()+" sending…"))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("ctrl+r mode  •  tab field  •  ctrl+f format JSON  •  ctrl+l load file  •  ctrl+s send"), lipgloss.NewStyle().Foreground(m.theme.muted).Render("ctrl+t save template  •  ctrl+y copy CLI  •  ctrl+o templates  •  ctrl+h history  •  1-9/0 jump workspace"))
	card := lipgloss.NewStyle().Width(contentWidth).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}

func (m Model) compactRequestComposeView(width, height int) string {
	state := m.requestReply
	contentWidth := max(width-6, 18)
	mode := "PUBLISH"
	if state.requestMode {
		mode = "REQUEST"
	}
	label := lipgloss.NewStyle().Bold(true).Foreground(m.theme.muted)
	value := lipgloss.NewStyle().Foreground(m.theme.primary)
	state.subject.Width = max(contentWidth-12, 8)
	state.headers.Width = max(contentWidth-12, 8)
	state.file.Width = max(contentWidth-12, 8)
	state.timeout.Width = max(contentWidth-12, 8)
	state.maxReplies.Width = max(contentWidth-12, 8)
	state.template.Width = max(contentWidth-12, 8)
	state.payload.SetWidth(max(contentWidth-2, 12))
	state.payload.SetHeight(3)
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("Publish / Request") + "  " + lipgloss.NewStyle().Bold(true).Foreground(m.theme.mint).Render(mode),
		lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("Field %d/7", state.focus+1)),
		label.Render("SUBJECT") + "  " + value.Render(truncate(valueOr(state.subject.Value(), "(empty)"), max(contentWidth-11, 8))),
	}
	switch state.focus {
	case 0:
		lines = append(lines, state.subject.View())
	case 1:
		lines = append(lines, label.Render("HEADERS"), state.headers.View())
	case 2:
		lines = append(lines, label.Render("PAYLOAD"), state.payload.View())
	case 3:
		lines = append(lines, label.Render("PAYLOAD FILE"), state.file.View())
	case 4:
		lines = append(lines, label.Render("TIMEOUT"), state.timeout.View())
	case 5:
		lines = append(lines, label.Render("MAX REPLIES"), state.maxReplies.View())
	case 6:
		lines = append(lines, label.Render("TEMPLATE"), state.template.View())
	}
	if state.focus != 2 {
		lines = append(lines, label.Render("PAYLOAD")+"  "+value.Render(fmt.Sprintf("%d bytes", len(state.payload.Value()))))
	}
	if state.error != nil {
		lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.red).Render(truncate(state.error.Error(), contentWidth)))
	}
	if state.notice != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.mint).Render("✓ "+truncate(state.notice, max(contentWidth-2, 8))))
	}
	lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.muted).Render("tab field • ctrl+s send • ctrl+r mode"))
	card := lipgloss.NewStyle().Width(contentWidth).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}

func (m Model) requestResultsView(width, height int) string {
	if m.requestReply.exchange == nil {
		return "No request result."
	}
	exchange := *m.requestReply.exchange
	replies := exchange.Replies()
	if len(replies) == 0 {
		return lipgloss.NewStyle().Foreground(m.theme.secondary).Render("Request completed with 0 replies.\n\nesc compose  •  ctrl+h history")
	}
	cursor := min(m.requestReply.replyCursor, len(replies)-1)
	reply := replies[cursor]
	if width < 90 {
		return m.compactRequestResultsView(width, height, exchange, cursor, reply, len(replies))
	}
	listWidth := min(max(width/3, 28), 42)
	rows := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(fmt.Sprintf("%d replies", len(replies)))}
	for index, item := range replies {
		marker, style := "  ", lipgloss.NewStyle().Foreground(m.theme.secondary)
		if index == cursor {
			marker, style = "› ", style.Bold(true).Foreground(m.theme.accent)
		}
		rows = append(rows, style.Render(fmt.Sprintf("%s#%-3d %9s  %d B", marker, index+1, item.Latency().Round(time.Microsecond), len(item.Payload()))))
	}
	payload := prettyRequestPayload(reply.Payload())
	detailLines := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("REPLY DETAIL"), lipgloss.NewStyle().Foreground(m.theme.muted).Render("Latency " + reply.Latency().String()), ""}
	if headers := reply.Headers(); len(headers) > 0 {
		detailLines = append(detailLines, lipgloss.NewStyle().Bold(true).Foreground(m.theme.muted).Render("HEADERS"))
		names := make([]string, 0, len(headers))
		for name := range headers {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			detailLines = append(detailLines, lipgloss.NewStyle().Foreground(m.theme.secondary).Render(name+": "+strings.Join(headers[name], ", ")))
		}
		detailLines = append(detailLines, "")
	}
	for _, line := range strings.Split(payload, "\n") {
		detailLines = append(detailLines, m.highlightJSONLine(line))
	}
	cli := requestCLIEquivalent(exchange.Request())
	detailLines = append(detailLines, "", lipgloss.NewStyle().Bold(true).Foreground(m.theme.muted).Render("CLI EQUIVALENT"), lipgloss.NewStyle().Foreground(m.theme.cyan).Render(cli), "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ reply  •  C copy CLI  •  esc compose  •  ctrl+h history"))
	left := lipgloss.NewStyle().Width(listWidth).Render(strings.Join(rows, "\n"))
	right := lipgloss.NewStyle().Width(max(width-listWidth-5, 30)).Render(strings.Join(detailLines, "\n"))
	heading := lipgloss.NewStyle().Foreground(m.theme.muted).Render("Publish / Request Lab  ›  " + exchange.ID() + "  ›  " + exchange.Request().Message().Subject().String())
	return heading + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right)
}

func (m Model) compactRequestResultsView(width, height int, exchange domain.Exchange, cursor int, reply domain.Reply, count int) string {
	heading := lipgloss.NewStyle().Foreground(m.theme.muted).Render(truncate("Publish / Request  ›  "+exchange.ID()+"  ›  "+exchange.Request().Message().Subject().String(), width))
	status := lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(fmt.Sprintf("Reply %d/%d", cursor+1, count)) + lipgloss.NewStyle().Foreground(m.theme.secondary).Render("  •  "+reply.Latency().String()+fmt.Sprintf("  •  %d B", len(reply.Payload())))
	payloadLines := strings.Split(prettyRequestPayload(reply.Payload()), "\n")
	capacity := max(height-9, 2)
	if len(payloadLines) > capacity {
		payloadLines = payloadLines[:capacity]
	}
	lines := []string{heading, "", status, ""}
	for _, line := range payloadLines {
		lines = append(lines, m.highlightJSONLine(truncate(line, width)))
	}
	lines = append(lines, "", lipgloss.NewStyle().Bold(true).Foreground(m.theme.muted).Render("CLI EQUIVALENT"), lipgloss.NewStyle().Foreground(m.theme.cyan).Render(truncate(requestCLIEquivalent(exchange.Request()), width)), lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ reply • C copy • esc compose"))
	return strings.Join(lines, "\n")
}

func prettyRequestPayload(payload []byte) string {
	var output bytes.Buffer
	if json.Indent(&output, payload, "", "  ") == nil {
		return output.String()
	}
	return string(payload)
}

func (m Model) loadRequestHistory() (tea.Model, tea.Cmd) {
	if m.requestReplyServices.LoadHistory == nil {
		m.requestReply.error = errors.New("request history is unavailable")
		return m, nil
	}
	m.requestReply.loadingState = true
	m.requestReply.error = nil
	return m, m.requestReplyServices.LoadHistory()
}

func (m Model) loadRequestTemplates() (tea.Model, tea.Cmd) {
	if m.requestReplyServices.LoadTemplates == nil {
		m.requestReply.error = errors.New("request templates are unavailable")
		return m, nil
	}
	m.requestReply.loadingState = true
	m.requestReply.error = nil
	return m, m.requestReplyServices.LoadTemplates()
}

func (m Model) saveRequestTemplate() (tea.Model, tea.Cmd) {
	if m.requestReplyServices.SaveTemplate == nil {
		m.requestReply.error = errors.New("request template saving is unavailable")
		return m, nil
	}
	name := strings.TrimSpace(m.requestReply.template.Value())
	copy := m
	copy.requestReply.requestMode = true
	request, err := copy.composeRequest()
	if err != nil {
		m.requestReply.error = err
		return m, nil
	}
	template, err := domain.NewTemplate(name, request)
	if err != nil {
		m.requestReply.error = err
		return m, nil
	}
	m.requestReply.running = true
	m.requestReply.error = nil
	return m, m.requestReplyServices.SaveTemplate(template)
}

func (m Model) updateRequestNavigation(key tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	state := &m.requestReply
	switch key.String() {
	case "esc":
		state.pane = requestCompose
		state.error = nil
		return m, nil, true
	case "ctrl+h":
		model, command := m.loadRequestHistory()
		return model, command, true
	case "ctrl+o":
		model, command := m.loadRequestTemplates()
		return model, command, true
	case "up", "k":
		m.moveRequestCursor(-1)
		return m, nil, true
	case "down", "j":
		m.moveRequestCursor(1)
		return m, nil, true
	case "C":
		request, ok := m.selectedRequest()
		if !ok || m.copyValue == nil {
			return m, nil, true
		}
		if err := m.copyValue(requestCLIEquivalent(request)); err != nil {
			state.error = fmt.Errorf("copy CLI equivalent: %w", err)
		} else {
			state.notice = "CLI equivalent copied"
		}
		return m, nil, true
	case "enter":
		request, ok := m.selectedRequest()
		if ok {
			m.fillRequestCompose(request)
			state.pane = requestCompose
		}
		return m, nil, true
	case "R":
		request, ok := m.selectedRequest()
		if !ok || m.requestReplyServices.Request == nil {
			state.error = errors.New("request replay is unavailable")
			return m, nil, true
		}
		state.running = true
		state.error = nil
		return m, m.requestReplyServices.Request(request), true
	case "d":
		if state.pane != requestTemplates || len(state.templates) == 0 {
			return m, nil, true
		}
		if m.requestReplyServices.DeleteTemplate == nil {
			state.error = errors.New("request template deletion is unavailable")
			return m, nil, true
		}
		name := state.templates[state.templateCursor].Name()
		state.running = true
		return m, m.requestReplyServices.DeleteTemplate(name), true
	}
	return m, nil, true
}

func (m *Model) moveRequestCursor(step int) {
	switch m.requestReply.pane {
	case requestResults:
		count := 0
		if m.requestReply.exchange != nil {
			count = len(m.requestReply.exchange.Replies())
		}
		m.requestReply.replyCursor = min(max(m.requestReply.replyCursor+step, 0), max(count-1, 0))
	case requestHistory:
		m.requestReply.historyCursor = min(max(m.requestReply.historyCursor+step, 0), max(len(m.requestReply.history)-1, 0))
	case requestTemplates:
		m.requestReply.templateCursor = min(max(m.requestReply.templateCursor+step, 0), max(len(m.requestReply.templates)-1, 0))
	}
}

func (m Model) selectedRequest() (domain.Request, bool) {
	switch m.requestReply.pane {
	case requestResults:
		if m.requestReply.exchange != nil {
			return m.requestReply.exchange.Request(), true
		}
	case requestHistory:
		if len(m.requestReply.history) > 0 {
			return m.requestReply.history[m.requestReply.historyCursor].Request(), true
		}
	case requestTemplates:
		if len(m.requestReply.templates) > 0 {
			return m.requestReply.templates[m.requestReply.templateCursor].Request(), true
		}
	}
	return domain.Request{}, false
}

func (m *Model) fillRequestCompose(request domain.Request) {
	message := request.Message()
	m.requestReply.requestMode = true
	m.requestReply.subject.SetValue(message.Subject().String())
	m.requestReply.headers.SetValue(formatObjectHeaders(message.Headers()))
	m.requestReply.payload.SetValue(string(message.Payload()))
	m.requestReply.timeout.SetValue(request.Timeout().String())
	m.requestReply.maxReplies.SetValue(strconv.Itoa(request.MaxReplies()))
	m.requestReply.notice = "Loaded request into composer"
}

func (m Model) requestHistoryView(width, height int) string {
	heading := lipgloss.NewStyle().Foreground(m.theme.muted).Render("Publish / Request Lab  ›  History")
	if len(m.requestReply.history) == 0 {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.secondary).Render("No saved requests yet.") + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render("esc compose  •  ctrl+o templates")
	}
	return heading + "\n\n" + m.requestCollectionView(width, height, true)
}

func (m Model) requestTemplatesView(width, height int) string {
	heading := lipgloss.NewStyle().Foreground(m.theme.muted).Render("Publish / Request Lab  ›  Templates")
	if len(m.requestReply.templates) == 0 {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.secondary).Render("No request templates saved.") + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render("esc compose  •  ctrl+t saves from composer")
	}
	return heading + "\n\n" + m.requestCollectionView(width, height, false)
}

func (m Model) requestCollectionView(width, height int, history bool) string {
	listWidth := min(max(width*2/5, 34), 58)
	rows := make([]string, 0)
	var request domain.Request
	if history {
		rows = append(rows, lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(fmt.Sprintf("REQUEST HISTORY  %d", len(m.requestReply.history))))
		for index, exchange := range m.requestReply.history {
			marker, style := "  ", lipgloss.NewStyle().Foreground(m.theme.secondary)
			if index == m.requestReply.historyCursor {
				marker, style = "› ", style.Bold(true).Foreground(m.theme.accent)
				request = exchange.Request()
			}
			rows = append(rows, style.Render(truncate(fmt.Sprintf("%s%-12s %-10s %-14s %d replies", marker, exchange.StartedAt().Local().Format("Jan 02 15:04"), exchange.ID(), exchange.Request().Message().Subject(), len(exchange.Replies())), listWidth)))
		}
	} else {
		rows = append(rows, lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(fmt.Sprintf("REQUEST TEMPLATES  %d", len(m.requestReply.templates))))
		for index, template := range m.requestReply.templates {
			marker, style := "  ", lipgloss.NewStyle().Foreground(m.theme.secondary)
			if index == m.requestReply.templateCursor {
				marker, style = "› ", style.Bold(true).Foreground(m.theme.accent)
				request = template.Request()
			}
			rows = append(rows, style.Render(truncate(fmt.Sprintf("%s%-18s %s", marker, template.Name(), template.Request().Message().Subject()), listWidth)))
		}
	}
	if width < 90 {
		capacity := max(height-8, 2)
		if len(rows) > capacity {
			rows = rows[:capacity]
		}
		rows = append(rows, "", lipgloss.NewStyle().Bold(true).Foreground(m.theme.muted).Render("REQUEST PREVIEW"), lipgloss.NewStyle().Foreground(m.theme.primary).Render(truncate(request.Message().Subject().String(), width)), lipgloss.NewStyle().Foreground(m.theme.cyan).Render(truncate(requestCLIEquivalent(request), width)), lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ select • enter edit • R run • esc"))
		return strings.Join(rows, "\n")
	}
	detail := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("REQUEST PREVIEW"), "", lipgloss.NewStyle().Foreground(m.theme.primary).Render(request.Message().Subject().String()), lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("timeout %s  •  max %d replies", request.Timeout(), request.MaxReplies())), ""}
	for _, line := range strings.Split(prettyRequestPayload(request.Message().Payload()), "\n") {
		detail = append(detail, m.highlightJSONLine(line))
	}
	detail = append(detail, "", lipgloss.NewStyle().Bold(true).Foreground(m.theme.muted).Render("CLI EQUIVALENT"), lipgloss.NewStyle().Foreground(m.theme.cyan).Render(requestCLIEquivalent(request)), "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ select  •  enter edit  •  R run  •  C copy CLI  •  esc compose"))
	if !history {
		detail = append(detail, lipgloss.NewStyle().Foreground(m.theme.muted).Render("d delete template"))
	}
	left := lipgloss.NewStyle().Width(listWidth).Render(strings.Join(rows, "\n"))
	right := lipgloss.NewStyle().Width(max(width-listWidth-4, 28)).MaxHeight(max(height, 4)).Render(strings.Join(detail, "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right)
}

func (m *Model) upsertRequestTemplate(template domain.Template) {
	for index := range m.requestReply.templates {
		if m.requestReply.templates[index].Name() == template.Name() {
			m.requestReply.templates[index] = template
			return
		}
	}
	m.requestReply.templates = append(m.requestReply.templates, template)
	sort.Slice(m.requestReply.templates, func(i, j int) bool { return m.requestReply.templates[i].Name() < m.requestReply.templates[j].Name() })
}

func (m *Model) removeRequestTemplate(name string) {
	for index, template := range m.requestReply.templates {
		if template.Name() == name {
			m.requestReply.templates = append(m.requestReply.templates[:index], m.requestReply.templates[index+1:]...)
			break
		}
	}
	m.requestReply.templateCursor = min(m.requestReply.templateCursor, max(len(m.requestReply.templates)-1, 0))
	m.requestReply.notice = "Deleted template " + name
}

func requestCLIEquivalent(request domain.Request) string {
	message := request.Message()
	parts := []string{"nats-tui", "request", requestShellQuote(message.Subject().String()), requestShellQuote(string(message.Payload()))}
	headers := message.Headers()
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, value := range headers[name] {
			parts = append(parts, "-H", requestShellQuote(name+"="+value))
		}
	}
	parts = append(parts, "--timeout", request.Timeout().String(), "--max-replies", strconv.Itoa(request.MaxReplies()))
	return strings.Join(parts, " ")
}

func publishCLIEquivalent(message domain.Message) string {
	parts := []string{"nats-tui", "publish", requestShellQuote(message.Subject().String()), requestShellQuote(string(message.Payload()))}
	headers := message.Headers()
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, value := range headers[name] {
			parts = append(parts, "-H", requestShellQuote(name+"="+value))
		}
	}
	return strings.Join(parts, " ")
}

func requestShellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\r\n'\"$`\\!&|;()<>*?[]{}") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
