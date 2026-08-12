package tui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fd1az/nats-tui/internal/app/consumers"
	"github.com/fd1az/nats-tui/internal/app/streams"
)

const consumerFormFieldCount = 29

type consumerForm struct {
	action                                                  consumers.ConfigAction
	field                                                   int
	streamNames                                             []string
	stream, name, description, deliverSubject, deliverGroup string
	deliverPolicy, startSequence, startTime, ackPolicy      string
	ackWait, maxDeliver, backoff, filters, replayPolicy     string
	rateLimit, sample, maxAckPending, maxWaiting            string
	maxRequestBatch, maxRequestExpires, maxRequestBytes     string
	inactiveThreshold, replicas, heartbeat, metadata        string
	durable, memoryStorage, headersOnly, flowControl        bool
}

func newCreateConsumerForm(streams []streams.Summary) consumerForm {
	streamNames := make([]string, 0, len(streams))
	for _, stream := range streams {
		streamNames = append(streamNames, stream.Name)
	}
	stream := ""
	if len(streamNames) > 0 {
		stream = streamNames[0]
	}
	definition := consumers.DefaultDefinition(stream, "")
	return consumerFormFromDefinition(consumers.ConfigCreate, definition, streamNames)
}

func newEditConsumerForm(summary consumers.Summary) consumerForm {
	return consumerFormFromDefinition(consumers.ConfigUpdate, summary.Definition(), []string{summary.Stream})
}

func consumerFormFromDefinition(action consumers.ConfigAction, definition consumers.Definition, streamNames []string) consumerForm {
	startTime := ""
	if definition.StartTime != nil {
		startTime = definition.StartTime.Format(time.RFC3339)
	}
	return consumerForm{
		action: action, streamNames: append([]string(nil), streamNames...), stream: definition.Stream, name: definition.Name,
		description: definition.Description, durable: definition.Durable, deliverSubject: definition.DeliverSubject, deliverGroup: definition.DeliverGroup,
		deliverPolicy: string(definition.DeliverPolicy), startSequence: strconv.FormatUint(definition.StartSequence, 10), startTime: startTime,
		ackPolicy: string(definition.AckPolicy), ackWait: definition.AckWait.String(), maxDeliver: strconv.Itoa(definition.MaxDeliver),
		backoff: formatDurations(definition.Backoff), filters: strings.Join(definition.FilterSubjects, ", "), replayPolicy: string(definition.ReplayPolicy),
		rateLimit: strconv.FormatUint(definition.RateLimit, 10), sample: definition.SampleFrequency, maxAckPending: strconv.Itoa(definition.MaxAckPending),
		maxWaiting: strconv.Itoa(definition.MaxWaiting), maxRequestBatch: strconv.Itoa(definition.MaxRequestBatch), maxRequestExpires: definition.MaxRequestExpires.String(),
		maxRequestBytes: strconv.Itoa(definition.MaxRequestMaxBytes), inactiveThreshold: definition.InactiveThreshold.String(), replicas: strconv.Itoa(definition.Replicas),
		memoryStorage: definition.MemoryStorage, headersOnly: definition.HeadersOnly, flowControl: definition.FlowControl,
		heartbeat: definition.IdleHeartbeat.String(), metadata: formatObjectProperties(definition.Metadata),
	}
}

func formatDurations(values []time.Duration) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, value.String())
	}
	return strings.Join(parts, ", ")
}

func (f consumerForm) definition() (consumers.Definition, error) {
	startSequence, err := strconv.ParseUint(strings.TrimSpace(f.startSequence), 10, 64)
	if err != nil {
		return consumers.Definition{}, errors.New("start sequence must be an integer")
	}
	startTime, err := parseConsumerFormTime(f.startTime)
	if err != nil {
		return consumers.Definition{}, err
	}
	ackWait, err := parseConsumerFormDuration("ack wait", f.ackWait)
	if err != nil {
		return consumers.Definition{}, err
	}
	maxDeliver, err := parseConsumerFormInt("max deliver", f.maxDeliver)
	if err != nil {
		return consumers.Definition{}, err
	}
	backoff, err := parseConsumerFormDurations(f.backoff)
	if err != nil {
		return consumers.Definition{}, err
	}
	rateLimit, err := strconv.ParseUint(strings.TrimSpace(f.rateLimit), 10, 64)
	if err != nil {
		return consumers.Definition{}, errors.New("rate limit must be an integer")
	}
	maxAckPending, err := parseConsumerFormInt("max ack pending", f.maxAckPending)
	if err != nil {
		return consumers.Definition{}, err
	}
	maxWaiting, err := parseConsumerFormInt("max waiting", f.maxWaiting)
	if err != nil {
		return consumers.Definition{}, err
	}
	maxRequestBatch, err := parseConsumerFormInt("max request batch", f.maxRequestBatch)
	if err != nil {
		return consumers.Definition{}, err
	}
	maxRequestExpires, err := parseConsumerFormDuration("max request expires", f.maxRequestExpires)
	if err != nil {
		return consumers.Definition{}, err
	}
	maxRequestBytes, err := parseConsumerFormInt("max request bytes", f.maxRequestBytes)
	if err != nil {
		return consumers.Definition{}, err
	}
	inactiveThreshold, err := parseConsumerFormDuration("inactive threshold", f.inactiveThreshold)
	if err != nil {
		return consumers.Definition{}, err
	}
	replicas, err := parseConsumerFormInt("replicas", f.replicas)
	if err != nil {
		return consumers.Definition{}, err
	}
	heartbeat, err := parseConsumerFormDuration("heartbeat", f.heartbeat)
	if err != nil {
		return consumers.Definition{}, err
	}
	metadata, err := parseObjectProperties(f.metadata)
	if err != nil {
		return consumers.Definition{}, fmt.Errorf("metadata: %w", err)
	}
	definition := consumers.Definition{
		Stream: strings.TrimSpace(f.stream), Name: strings.TrimSpace(f.name), Description: strings.TrimSpace(f.description), Durable: f.durable,
		DeliverSubject: strings.TrimSpace(f.deliverSubject), DeliverGroup: strings.TrimSpace(f.deliverGroup), DeliverPolicy: consumers.DeliverPolicy(f.deliverPolicy),
		StartSequence: startSequence, StartTime: startTime, AckPolicy: consumers.AckPolicy(f.ackPolicy), AckWait: ackWait, MaxDeliver: maxDeliver,
		Backoff: backoff, FilterSubjects: splitSubjects(f.filters), ReplayPolicy: consumers.ReplayPolicy(f.replayPolicy), RateLimit: rateLimit,
		SampleFrequency: strings.TrimSpace(f.sample), MaxAckPending: maxAckPending, MaxWaiting: maxWaiting, MaxRequestBatch: maxRequestBatch,
		MaxRequestExpires: maxRequestExpires, MaxRequestMaxBytes: maxRequestBytes, InactiveThreshold: inactiveThreshold, Replicas: replicas,
		MemoryStorage: f.memoryStorage, HeadersOnly: f.headersOnly, FlowControl: f.flowControl, IdleHeartbeat: heartbeat, Metadata: metadata,
	}
	return definition, definition.Validate()
}

func parseConsumerFormTime(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return nil, errors.New("start time must use RFC3339")
	}
	return &parsed, nil
}

func parseConsumerFormDuration(label, value string) (time.Duration, error) {
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration", label)
	}
	return parsed, nil
}

func parseConsumerFormInt(label, value string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", label)
	}
	return parsed, nil
}

func parseConsumerFormDurations(value string) ([]time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	result := make([]time.Duration, 0, len(parts))
	for _, part := range parts {
		parsed, err := time.ParseDuration(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("backoff %q must be a duration", strings.TrimSpace(part))
		}
		result = append(result, parsed)
	}
	return result, nil
}

func (m Model) updateConsumerForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.consumerActionRunning {
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.consumerForm, m.consumerActionErr = nil, nil
		return m, nil
	case "ctrl+s":
		return m.submitConsumerForm()
	case "tab", "down":
		m.consumerForm.field = (m.consumerForm.field + 1) % consumerFormFieldCount
		return m, nil
	case "shift+tab", "up":
		m.consumerForm.field = (m.consumerForm.field - 1 + consumerFormFieldCount) % consumerFormFieldCount
		return m, nil
	case "left", "right", " ":
		if m.chooseConsumerFormValue(key.String() == "left") {
			return m, nil
		}
	case "backspace":
		m.editConsumerFormField(true, "")
		return m, nil
	}
	if key.Type == tea.KeyRunes {
		m.editConsumerFormField(false, string(key.Runes))
	}
	return m, nil
}

func (m Model) submitConsumerForm() (tea.Model, tea.Cmd) {
	if m.consumerConfigure == nil {
		m.consumerActionErr = errors.New("consumer configuration is unavailable")
		return m, nil
	}
	definition, err := m.consumerForm.definition()
	if err != nil {
		m.consumerActionErr = err
		return m, nil
	}
	input := ConsumerConfigInput{Generation: m.refreshGeneration, Action: m.consumerForm.action, Definition: definition}
	m.consumerActionRunning, m.consumerActionErr, m.consumerActionNotice = true, nil, ""
	return m, m.consumerConfigure(input)
}

func (m *Model) chooseConsumerFormValue(reverse bool) bool {
	f := m.consumerForm
	switch f.field {
	case 0:
		if f.action == consumers.ConfigUpdate || len(f.streamNames) == 0 {
			return true
		}
		index := 0
		for i, value := range f.streamNames {
			if value == f.stream {
				index = i
			}
		}
		step := 1
		if reverse {
			step = -1
		}
		f.stream = f.streamNames[(index+step+len(f.streamNames))%len(f.streamNames)]
		return true
	case 3:
		if f.action == consumers.ConfigCreate {
			f.durable = !f.durable
		}
		return true
	case 6:
		f.deliverPolicy = cycleConsumerValue(f.deliverPolicy, reverse, []string{string(consumers.DeliverAll), string(consumers.DeliverLast), string(consumers.DeliverNew), string(consumers.DeliverByStartSequence), string(consumers.DeliverByStartTime), string(consumers.DeliverLastPerSubject)})
		return true
	case 9:
		f.ackPolicy = cycleConsumerValue(f.ackPolicy, reverse, []string{string(consumers.AckExplicit), string(consumers.AckAll), string(consumers.AckNone)})
		return true
	case 14:
		f.replayPolicy = cycleConsumerValue(f.replayPolicy, reverse, []string{string(consumers.ReplayInstant), string(consumers.ReplayOriginal)})
		return true
	case 24:
		f.memoryStorage = !f.memoryStorage
		return true
	case 25:
		f.headersOnly = !f.headersOnly
		return true
	case 26:
		f.flowControl = !f.flowControl
		return true
	default:
		return false
	}
}

func cycleConsumerValue(current string, reverse bool, values []string) string {
	index := 0
	for i, value := range values {
		if value == current {
			index = i
		}
	}
	step := 1
	if reverse {
		step = -1
	}
	return values[(index+step+len(values))%len(values)]
}

func (m *Model) editConsumerFormField(backspace bool, addition string) {
	field := m.consumerFormTextField()
	if field == nil || (m.consumerForm.action == consumers.ConfigUpdate && (m.consumerForm.field == 0 || m.consumerForm.field == 1)) {
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

func (m *Model) consumerFormTextField() *string {
	f := m.consumerForm
	switch f.field {
	case 1:
		return &f.name
	case 2:
		return &f.description
	case 4:
		return &f.deliverSubject
	case 5:
		return &f.deliverGroup
	case 7:
		return &f.startSequence
	case 8:
		return &f.startTime
	case 10:
		return &f.ackWait
	case 11:
		return &f.maxDeliver
	case 12:
		return &f.backoff
	case 13:
		return &f.filters
	case 15:
		return &f.rateLimit
	case 16:
		return &f.sample
	case 17:
		return &f.maxAckPending
	case 18:
		return &f.maxWaiting
	case 19:
		return &f.maxRequestBatch
	case 20:
		return &f.maxRequestExpires
	case 21:
		return &f.maxRequestBytes
	case 22:
		return &f.inactiveThreshold
	case 23:
		return &f.replicas
	case 27:
		return &f.heartbeat
	case 28:
		return &f.metadata
	default:
		return nil
	}
}

func (m Model) consumerFormView(width, height int) string {
	f := m.consumerForm
	title := "Create consumer"
	if f.action == consumers.ConfigUpdate {
		title = "Edit consumer"
	}
	values := []struct{ label, value, help string }{
		{"Stream", f.stream, "Existing stream that owns this consumer."}, {"Name", f.name, "Stable identity; immutable after creation."},
		{"Description", f.description, "Human context for operators."}, {"Durable", onOff(f.durable), "OFF creates a named ephemeral consumer."},
		{"Deliver subject", f.deliverSubject, "Empty = pull; set a subject for push."}, {"Deliver group", f.deliverGroup, "Optional queue group for push delivery."},
		{"Deliver policy", f.deliverPolicy, "Where replay begins."}, {"Start sequence", f.startSequence, "Required by by-start-sequence."},
		{"Start time", f.startTime, "RFC3339; required by by-start-time."}, {"Ack policy", f.ackPolicy, "Explicit is safest for workers."},
		{"Ack wait", f.ackWait, "Example: 30s."}, {"Max deliver", f.maxDeliver, "-1 means unlimited retries."},
		{"Backoff", f.backoff, "Comma-separated: 1s, 5s, 30s."}, {"Filter subjects", f.filters, "Comma-separated; multiple require NATS 2.10+."},
		{"Replay", f.replayPolicy, "Instant or original message timing."}, {"Rate limit (bps)", f.rateLimit, "0 uses no delivery rate limit."},
		{"Ack sample", f.sample, "0-100%, for example 25%."}, {"Max ack pending", f.maxAckPending, "Backpressure threshold; -1 unlimited."},
		{"Max waiting", f.maxWaiting, "Maximum waiting pull requests."}, {"Max request batch", f.maxRequestBatch, "0 uses server default."},
		{"Max request expires", f.maxRequestExpires, "Maximum pull wait duration."}, {"Max request bytes", f.maxRequestBytes, "Maximum bytes in one pull."},
		{"Inactive threshold", f.inactiveThreshold, "Cleanup after inactivity; 0 uses default."}, {"Replicas", f.replicas, "0 inherits stream replicas."},
		{"Memory storage", onOff(f.memoryStorage), "Keep consumer state in memory."}, {"Headers only", onOff(f.headersOnly), "Omit payload during delivery."},
		{"Flow control", onOff(f.flowControl), "Push only; requires heartbeat."}, {"Idle heartbeat", f.heartbeat, "Push liveness interval, e.g. 5s."},
		{"Metadata", f.metadata, "Comma-separated key=value pairs."},
	}
	visible := max((height-15)/2, 3)
	start := max(f.field-visible/2, 0)
	end := min(start+visible, len(values))
	start = max(end-visible, 0)
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(title), lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("Field %d/%d  ·  complete JetStream configuration", f.field+1, len(values))), ""}
	if start > 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.muted).Render("  ↑ more"))
	}
	for index := start; index < end; index++ {
		item := values[index]
		prefix := "  "
		rendered := tailTruncate(item.value, max(width-12, 12))
		if index == f.field {
			prefix = lipgloss.NewStyle().Foreground(m.theme.accent).Render("› ")
			if m.consumerFormTextField() != nil {
				rendered += lipgloss.NewStyle().Foreground(m.theme.accent).Render("▏")
			}
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.muted).Render(item.label), prefix+rendered)
		if index == f.field {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(m.theme.secondary).Render(item.help))
		}
	}
	if end < len(values) {
		lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.muted).Render("  ↓ more"))
	}
	if m.consumerActionErr != nil {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.red).Render(m.consumerActionErr.Error()))
	}
	if m.consumerActionRunning {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.yellow).Render("↻ applying consumer configuration"))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("tab next  •  ←/→ choose  •  ctrl+s save  •  esc cancel"))
	cardWidth := min(max(width-12, 52), 100)
	card := lipgloss.NewStyle().Width(cardWidth).Padding(1, 2).BorderStyle(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}
