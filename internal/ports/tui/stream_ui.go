package tui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
)

const streamFormFieldCount = 34

type streamForm struct {
	action                                                                       streams.Action
	field                                                                        int
	base                                                                         streams.Definition
	name                                                                         string
	description                                                                  string
	subjects                                                                     string
	storage                                                                      streams.Storage
	retention                                                                    streams.Retention
	maxMessages                                                                  string
	maxBytes                                                                     string
	replicas                                                                     string
	maxAge, maxPerSubject, maxMessageSize, maxConsumers, duplicateWindow         string
	discard                                                                      streams.Discard
	compression, allowDirect, allowRollup, denyDelete, denyPurge                 bool
	noAck, allowMessageTTL, mirrorDirect, discardNewPerSubject, sealed           bool
	placementCluster, placementTags, metadata, mirror, sources                   string
	transformSource, transformDestination, republishSource, republishDestination string
	republishHeadersOnly                                                         bool
	sealConfirmed                                                                bool
	err                                                                          error
}

type streamPurgeForm struct {
	stream, subject, sequence, keep string
	field                           int
	confirmed                       bool
}

func newStreamPurgeForm(stream streams.Summary) streamPurgeForm {
	return streamPurgeForm{stream: stream.Name, sequence: "0", keep: "0"}
}

func newCreateStreamForm() streamForm {
	base := streams.Definition{Storage: streams.StorageFile, Retention: streams.RetentionLimits, MaxMessages: -1, MaxBytes: -1, MaxMessagesPerSubject: -1, MaxMessageSize: -1, MaxConsumers: -1, Replicas: 1, Discard: streams.DiscardOld}
	return streamForm{action: streams.ActionCreate, base: base, storage: base.Storage, retention: base.Retention, discard: base.Discard, maxMessages: "-1", maxBytes: "-1", replicas: "1", maxAge: "0", maxPerSubject: "-1", maxMessageSize: "-1", maxConsumers: "-1", duplicateWindow: "0"}
}

func newEditStreamForm(stream streams.Summary) streamForm {
	base := stream.Definition()
	mirror, sources := "", make([]string, 0, len(stream.Sources))
	if stream.Mirror != nil {
		mirror = stream.Mirror.Name
	}
	for _, source := range stream.Sources {
		sources = append(sources, source.Name)
	}
	transformSource, transformDestination := "", ""
	if stream.SubjectTransform != nil {
		transformSource, transformDestination = stream.SubjectTransform.Source, stream.SubjectTransform.Destination
	}
	republishSource, republishDestination, republishHeadersOnly := "", "", false
	if stream.Republish != nil {
		republishSource, republishDestination, republishHeadersOnly = stream.Republish.Source, stream.Republish.Destination, stream.Republish.HeadersOnly
	}
	return streamForm{
		action: streams.ActionUpdate, field: 1, base: base, name: stream.Name, description: stream.Description,
		subjects: strings.Join(stream.Subjects, ", "), storage: stream.Storage, retention: stream.Retention,
		maxMessages: strconv.FormatInt(stream.MaxMessages, 10), maxBytes: strconv.FormatInt(stream.MaxBytes, 10), replicas: strconv.Itoa(stream.Replicas),
		maxAge: stream.MaxAge.String(), maxPerSubject: strconv.FormatInt(stream.MaxMessagesPerSubject, 10), maxMessageSize: strconv.FormatInt(int64(stream.MaxMessageSize), 10), maxConsumers: strconv.Itoa(stream.MaxConsumers), duplicateWindow: stream.DuplicateWindow.String(),
		discard: stream.Discard, compression: stream.Compression, allowDirect: stream.AllowDirect, allowRollup: stream.AllowRollup,
		denyDelete: stream.DenyDelete, denyPurge: stream.DenyPurge, noAck: stream.NoAck, allowMessageTTL: stream.AllowMessageTTL,
		mirrorDirect: stream.MirrorDirect, discardNewPerSubject: stream.DiscardNewPerSubject, sealed: stream.Sealed,
		placementCluster: stream.PlacementCluster, placementTags: strings.Join(stream.PlacementTags, ", "), metadata: formatObjectProperties(stream.Metadata),
		mirror: mirror, sources: strings.Join(sources, ", "), transformSource: transformSource, transformDestination: transformDestination,
		republishSource: republishSource, republishDestination: republishDestination, republishHeadersOnly: republishHeadersOnly,
	}
}

func (f streamForm) definition() (streams.Definition, error) {
	maxMessages, err := strconv.ParseInt(strings.TrimSpace(f.maxMessages), 10, 64)
	if err != nil {
		return streams.Definition{}, fmt.Errorf("max messages must be an integer")
	}
	maxBytes, err := strconv.ParseInt(strings.TrimSpace(f.maxBytes), 10, 64)
	if err != nil {
		return streams.Definition{}, fmt.Errorf("max bytes must be an integer")
	}
	replicas, err := strconv.Atoi(strings.TrimSpace(f.replicas))
	if err != nil {
		return streams.Definition{}, fmt.Errorf("replicas must be an integer")
	}
	maxAge, err := time.ParseDuration(strings.TrimSpace(f.maxAge))
	if err != nil {
		return streams.Definition{}, fmt.Errorf("max age must be a duration")
	}
	maxPerSubject, err := strconv.ParseInt(strings.TrimSpace(f.maxPerSubject), 10, 64)
	if err != nil {
		return streams.Definition{}, fmt.Errorf("max messages per subject must be an integer")
	}
	maxMessageSize, err := strconv.ParseInt(strings.TrimSpace(f.maxMessageSize), 10, 32)
	if err != nil {
		return streams.Definition{}, fmt.Errorf("max message size must be an integer")
	}
	maxConsumers, err := strconv.Atoi(strings.TrimSpace(f.maxConsumers))
	if err != nil {
		return streams.Definition{}, fmt.Errorf("max consumers must be an integer")
	}
	duplicateWindow, err := time.ParseDuration(strings.TrimSpace(f.duplicateWindow))
	if err != nil {
		return streams.Definition{}, fmt.Errorf("duplicate window must be a duration")
	}
	metadata, err := parseObjectProperties(f.metadata)
	if err != nil {
		return streams.Definition{}, fmt.Errorf("metadata: %w", err)
	}
	definition := f.base
	definition.Name, definition.Description, definition.Subjects = strings.TrimSpace(f.name), strings.TrimSpace(f.description), splitSubjects(f.subjects)
	definition.Storage, definition.Retention, definition.MaxMessages, definition.MaxBytes, definition.Replicas = f.storage, f.retention, maxMessages, maxBytes, replicas
	definition.MaxAge, definition.MaxMessagesPerSubject, definition.MaxMessageSize, definition.MaxConsumers, definition.DuplicateWindow = maxAge, maxPerSubject, int32(maxMessageSize), maxConsumers, duplicateWindow
	definition.Discard, definition.Compression, definition.AllowDirect, definition.AllowRollup = f.discard, f.compression, f.allowDirect, f.allowRollup
	definition.DenyDelete, definition.DenyPurge, definition.NoAck, definition.AllowMessageTTL = f.denyDelete, f.denyPurge, f.noAck, f.allowMessageTTL
	definition.MirrorDirect, definition.DiscardNewPerSubject, definition.Sealed = f.mirrorDirect, f.discardNewPerSubject, f.sealed
	definition.PlacementCluster, definition.PlacementTags, definition.Metadata = strings.TrimSpace(f.placementCluster), splitSubjects(f.placementTags), metadata
	if strings.TrimSpace(f.mirror) == "" {
		definition.Mirror = nil
	} else if definition.Mirror == nil {
		definition.Mirror = &streams.Source{Name: strings.TrimSpace(f.mirror)}
	} else {
		definition.Mirror.Name = strings.TrimSpace(f.mirror)
	}
	sourceNames := splitSubjects(f.sources)
	if !sameStreamSourceNames(sourceNames, definition.Sources) {
		definition.Sources = nil
		for _, name := range sourceNames {
			definition.Sources = append(definition.Sources, streams.Source{Name: name})
		}
	}
	if f.transformSource == "" && f.transformDestination == "" {
		definition.SubjectTransform = nil
	} else {
		definition.SubjectTransform = &streams.SubjectTransform{Source: strings.TrimSpace(f.transformSource), Destination: strings.TrimSpace(f.transformDestination)}
	}
	if f.republishSource == "" && f.republishDestination == "" {
		definition.Republish = nil
	} else {
		definition.Republish = &streams.Republish{Source: strings.TrimSpace(f.republishSource), Destination: strings.TrimSpace(f.republishDestination), HeadersOnly: f.republishHeadersOnly}
	}
	if err := definition.Validate(); err != nil {
		return streams.Definition{}, err
	}
	return definition, nil
}

func splitSubjects(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		subject := strings.TrimSpace(part)
		if subject == "" {
			continue
		}
		if _, exists := seen[subject]; exists {
			continue
		}
		seen[subject] = struct{}{}
		result = append(result, subject)
	}
	return result
}

func (m Model) updateStreamForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.streamActionRunning {
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.streamForm = nil
		m.streamActionErr = nil
		return m, nil
	case "ctrl+s":
		return m.submitStreamForm()
	case "tab", "down":
		m.moveStreamFormField(1)
		return m, nil
	case "shift+tab", "up":
		m.moveStreamFormField(-1)
		return m, nil
	case "left", "right", " ":
		if m.streamForm.field == 3 && m.streamForm.action == streams.ActionCreate {
			m.toggleStreamStorage()
			return m, nil
		}
		if m.streamForm.field == 4 {
			m.cycleStreamRetention(key.String() == "left")
			return m, nil
		}
		if m.streamForm.field == 13 {
			m.toggleStreamDiscard()
			return m, nil
		}
		if m.toggleStreamBoolean() {
			return m, nil
		}
	case "backspace":
		m.editStreamField(true, "")
		return m, nil
	}
	if key.Type == tea.KeyRunes {
		m.editStreamField(false, string(key.Runes))
	}
	return m, nil
}

func (m *Model) moveStreamFormField(step int) {
	for {
		m.streamForm.field = (m.streamForm.field + step + streamFormFieldCount) % streamFormFieldCount
		if m.streamForm.action == streams.ActionCreate || (m.streamForm.field != 0 && m.streamForm.field != 3) {
			return
		}
	}
}

func (m Model) submitStreamForm() (tea.Model, tea.Cmd) {
	if m.streamAction == nil {
		m.streamActionErr = errors.New("stream operations are unavailable")
		return m, nil
	}
	definition, err := m.streamForm.definition()
	if err != nil {
		m.streamActionErr = err
		return m, nil
	}
	if definition.Sealed && !m.streamForm.base.Sealed && !m.streamForm.sealConfirmed {
		m.streamForm.sealConfirmed = true
		m.streamActionErr = errors.New("sealing cannot be reversed; press ctrl+s again to confirm")
		return m, nil
	}
	input := StreamActionInput{Generation: m.refreshGeneration, Action: m.streamForm.action, Name: definition.Name, Definition: definition}
	m.streamActionRunning = true
	m.streamActionErr = nil
	m.streamActionNotice = ""
	return m, m.streamAction(input)
}

func sameStreamSourceNames(names []string, sources []streams.Source) bool {
	if len(names) != len(sources) {
		return false
	}
	for i := range names {
		if names[i] != sources[i].Name {
			return false
		}
	}
	return true
}

func (m *Model) editStreamField(backspace bool, addition string) {
	field := m.streamTextField()
	if field == nil || (m.streamForm.action == streams.ActionUpdate && m.streamForm.field == 0) {
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

func (m *Model) streamTextField() *string {
	switch m.streamForm.field {
	case 0:
		return &m.streamForm.name
	case 1:
		return &m.streamForm.description
	case 2:
		return &m.streamForm.subjects
	case 5:
		return &m.streamForm.maxMessages
	case 6:
		return &m.streamForm.maxBytes
	case 7:
		return &m.streamForm.replicas
	case 8:
		return &m.streamForm.maxAge
	case 9:
		return &m.streamForm.maxPerSubject
	case 10:
		return &m.streamForm.maxMessageSize
	case 11:
		return &m.streamForm.maxConsumers
	case 12:
		return &m.streamForm.duplicateWindow
	case 19:
		return &m.streamForm.placementCluster
	case 20:
		return &m.streamForm.placementTags
	case 21:
		return &m.streamForm.metadata
	case 22:
		return &m.streamForm.mirror
	case 23:
		return &m.streamForm.sources
	case 24:
		return &m.streamForm.transformSource
	case 25:
		return &m.streamForm.transformDestination
	case 26:
		return &m.streamForm.republishSource
	case 27:
		return &m.streamForm.republishDestination
	default:
		return nil
	}
}

func (m *Model) toggleStreamDiscard() {
	if m.streamForm.discard == streams.DiscardNew {
		m.streamForm.discard = streams.DiscardOld
	} else {
		m.streamForm.discard = streams.DiscardNew
	}
}

func (m *Model) toggleStreamBoolean() bool {
	values := map[int]*bool{14: &m.streamForm.compression, 15: &m.streamForm.allowDirect, 16: &m.streamForm.allowRollup, 17: &m.streamForm.denyDelete, 18: &m.streamForm.denyPurge, 28: &m.streamForm.republishHeadersOnly, 29: &m.streamForm.noAck, 30: &m.streamForm.allowMessageTTL, 31: &m.streamForm.mirrorDirect, 32: &m.streamForm.discardNewPerSubject, 33: &m.streamForm.sealed}
	value, ok := values[m.streamForm.field]
	if ok {
		if m.streamForm.field == 33 && m.streamForm.base.Sealed {
			m.streamActionErr = errors.New("sealed streams cannot be unsealed")
			return true
		}
		*value = !*value
	}
	return ok
}

func (m *Model) toggleStreamStorage() {
	if m.streamForm.storage == streams.StorageFile {
		m.streamForm.storage = streams.StorageMemory
		return
	}
	m.streamForm.storage = streams.StorageFile
}

func (m *Model) cycleStreamRetention(reverse bool) {
	values := []streams.Retention{streams.RetentionLimits, streams.RetentionInterest, streams.RetentionWorkQueue}
	index := 0
	for candidate, value := range values {
		if value == m.streamForm.retention {
			index = candidate
		}
	}
	step := 1
	if reverse {
		step = -1
	}
	m.streamForm.retention = values[(index+step+len(values))%len(values)]
}

func (m Model) streamFormView(width, height int) string {
	title := "Create stream"
	if m.streamForm.action == streams.ActionUpdate {
		title = "Edit stream"
	}
	nameLabel := "Name"
	storageLabel := "Storage"
	if m.streamForm.action == streams.ActionUpdate {
		nameLabel = "Name (immutable)"
		storageLabel = "Storage (immutable)"
	}
	values := []struct{ label, value string }{
		{nameLabel, m.streamForm.name}, {"Description", m.streamForm.description}, {"Subjects (comma separated)", m.streamForm.subjects},
		{storageLabel, string(m.streamForm.storage)}, {"Retention", string(m.streamForm.retention)}, {"Max messages (-1 unlimited)", m.streamForm.maxMessages},
		{"Max bytes (-1 unlimited)", m.streamForm.maxBytes}, {"Replicas (1-5)", m.streamForm.replicas},
		{"Max age (0, 24h)", m.streamForm.maxAge}, {"Max messages / subject", m.streamForm.maxPerSubject}, {"Max message size", m.streamForm.maxMessageSize}, {"Max consumers", m.streamForm.maxConsumers}, {"Duplicate window", m.streamForm.duplicateWindow},
		{"Discard policy", string(m.streamForm.discard)}, {"Compression", onOff(m.streamForm.compression)}, {"Allow direct", onOff(m.streamForm.allowDirect)}, {"Allow rollup", onOff(m.streamForm.allowRollup)}, {"Deny delete", onOff(m.streamForm.denyDelete)}, {"Deny purge", onOff(m.streamForm.denyPurge)},
		{"Placement cluster", m.streamForm.placementCluster}, {"Placement tags", m.streamForm.placementTags}, {"Metadata (key=value)", m.streamForm.metadata}, {"Mirror stream", m.streamForm.mirror}, {"Source streams", m.streamForm.sources},
		{"Transform source", m.streamForm.transformSource}, {"Transform destination", m.streamForm.transformDestination}, {"Republish source", m.streamForm.republishSource}, {"Republish destination", m.streamForm.republishDestination}, {"Republish headers only", onOff(m.streamForm.republishHeadersOnly)},
		{"No publish ACK", onOff(m.streamForm.noAck)}, {"Allow message TTL", onOff(m.streamForm.allowMessageTTL)}, {"Mirror direct", onOff(m.streamForm.mirrorDirect)}, {"Discard new / subject", onOff(m.streamForm.discardNewPerSubject)}, {"Sealed (irreversible)", onOff(m.streamForm.sealed)},
	}
	visible := max((height-13)/2, 3)
	start := max(m.streamForm.field-visible/2, 0)
	end := min(start+visible, len(values))
	start = max(end-visible, 0)
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(title), lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("Field %d/%d  ·  advanced settings preserved", m.streamForm.field+1, len(values))), ""}
	if start > 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.muted).Render("  ↑ more"))
	}
	for index := start; index < end; index++ {
		item := values[index]
		label := lipgloss.NewStyle().Foreground(m.theme.muted).Render(item.label)
		value := item.value
		value = tailTruncate(value, max(width-8, 12))
		prefix := "  "
		if index == m.streamForm.field {
			prefix = lipgloss.NewStyle().Foreground(m.theme.accent).Render("› ")
			if index != 3 && index != 4 {
				value += lipgloss.NewStyle().Foreground(m.theme.accent).Render("▏")
			}
		}
		lines = append(lines, label, prefix+value)
	}
	if end < len(values) {
		lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.muted).Render("  ↓ more"))
	}
	if m.streamActionErr != nil {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.red).Render(m.streamActionErr.Error()))
	}
	if m.streamActionRunning {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.yellow).Render("↻ applying stream configuration"))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("tab next  •  ←/→ choose  •  ctrl+s save  •  esc cancel"))
	cardWidth := min(max(width-12, 48), 96)
	card := lipgloss.NewStyle().Width(cardWidth).Padding(1, 2).BorderStyle(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}

func onOff(value bool) string {
	if value {
		return "ON"
	}
	return "OFF"
}

func tailTruncate(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	return "…" + string(runes[len(runes)-width+1:])
}

func (m Model) updateStreamPurgeForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.streamActionRunning {
		return m, nil
	}
	form := m.streamPurgeForm
	switch key.String() {
	case "esc":
		m.streamPurgeForm, m.streamActionErr = nil, nil
		return m, nil
	case "tab", "down":
		form.field = (form.field + 1) % 3
		form.confirmed = false
		return m, nil
	case "shift+tab", "up":
		form.field = (form.field + 2) % 3
		form.confirmed = false
		return m, nil
	case "backspace":
		field := streamPurgeTextField(form)
		runes := []rune(*field)
		if len(runes) > 0 {
			*field = string(runes[:len(runes)-1])
		}
		form.confirmed = false
		return m, nil
	case "ctrl+s":
		return m.submitStreamPurge()
	}
	if key.Type == tea.KeyRunes {
		field := streamPurgeTextField(form)
		*field += string(key.Runes)
		form.confirmed = false
	}
	return m, nil
}

func streamPurgeTextField(form *streamPurgeForm) *string {
	switch form.field {
	case 0:
		return &form.subject
	case 1:
		return &form.sequence
	default:
		return &form.keep
	}
}

func (m Model) submitStreamPurge() (tea.Model, tea.Cmd) {
	if m.streamPurge == nil {
		m.streamActionErr = errors.New("stream purge is unavailable")
		return m, nil
	}
	sequence, err := strconv.ParseUint(strings.TrimSpace(m.streamPurgeForm.sequence), 10, 64)
	if err != nil {
		m.streamActionErr = errors.New("sequence must be a non-negative integer")
		return m, nil
	}
	keep, err := strconv.ParseUint(strings.TrimSpace(m.streamPurgeForm.keep), 10, 64)
	if err != nil {
		m.streamActionErr = errors.New("keep must be a non-negative integer")
		return m, nil
	}
	command := streams.PurgeCommand{Stream: m.streamPurgeForm.stream, Subject: strings.TrimSpace(m.streamPurgeForm.subject), Sequence: sequence, Keep: keep}
	if err := command.Validate(); err != nil {
		m.streamActionErr = err
		return m, nil
	}
	if !m.streamPurgeForm.confirmed {
		m.streamPurgeForm.confirmed = true
		m.streamActionErr = errors.New("messages will be permanently removed; press ctrl+s again to confirm")
		return m, nil
	}
	m.streamActionRunning, m.streamActionErr = true, nil
	return m, m.streamPurge(StreamPurgeInput{Generation: m.refreshGeneration, Command: command})
}

func (m Model) streamPurgeFormView(width, height int) string {
	form := m.streamPurgeForm
	items := []struct{ label, value, help string }{{"Subject filter", form.subject, "Empty matches the whole stream; wildcards are allowed."}, {"Before sequence", form.sequence, "Remove messages below this sequence. Cannot combine with keep."}, {"Keep newest", form.keep, "Keep the newest N matching messages. Cannot combine with sequence."}}
	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(m.theme.red).Render("Purge stream"), lipgloss.NewStyle().Foreground(m.theme.secondary).Render(form.stream), ""}
	for i, item := range items {
		marker := "  "
		if i == form.field {
			marker = lipgloss.NewStyle().Foreground(m.theme.accent).Render("› ")
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.muted).Render(item.label), marker+item.value, lipgloss.NewStyle().Foreground(m.theme.muted).Render("  "+item.help), "")
	}
	if m.streamActionErr != nil {
		lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.yellow).Render(m.streamActionErr.Error()), "")
	}
	lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.muted).Render("tab next  •  ctrl+s review/confirm  •  esc cancel"))
	card := lipgloss.NewStyle().Width(min(max(width-14, 50), 88)).Padding(1, 2).BorderStyle(lipgloss.RoundedBorder()).BorderForeground(m.theme.red).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}

func (m Model) updateStreamDeleteConfirmation(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "y", "enter":
		m.confirmStreamDelete = false
		return m.runStreamDelete()
	case "n", "esc":
		m.confirmStreamDelete = false
	}
	return m, nil
}

func (m Model) runStreamDelete() (tea.Model, tea.Cmd) {
	if m.streamAction == nil || len(m.streams) == 0 {
		m.streamActionErr = errors.New("stream operations are unavailable")
		return m, nil
	}
	stream := m.streams[m.streamCursor]
	input := StreamActionInput{Generation: m.refreshGeneration, Action: streams.ActionDelete, Name: stream.Name}
	m.streamActionRunning = true
	m.streamActionErr = nil
	return m, m.streamAction(input)
}

func (m Model) streamDeleteConfirmationView(width, height int) string {
	stream := m.streams[m.streamCursor]
	content := strings.Join([]string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.red).Render("Delete stream?"), "",
		lipgloss.NewStyle().Foreground(m.theme.primary).Render(stream.Name),
		lipgloss.NewStyle().Foreground(m.theme.yellow).Render(fmt.Sprintf("%s stored messages  •  %s", compactNumber(stream.Messages), humanBytes(stream.Bytes))), "",
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render("This removes the stream and all stored messages. This cannot be undone."), "",
		lipgloss.NewStyle().Foreground(m.theme.muted).Render("y confirm   n/esc cancel"),
	}, "\n")
	card := lipgloss.NewStyle().Width(min(max(width-12, 34), 76)).Padding(2, 3).BorderStyle(lipgloss.RoundedBorder()).BorderForeground(m.theme.red).Render(content)
	return m.centeredCanvas(width, height, card)
}

func (m Model) streamDetailView(width int, stream streams.Summary) string {
	label := lipgloss.NewStyle().Foreground(m.theme.muted)
	value := lipgloss.NewStyle().Foreground(m.theme.secondary)
	description := stream.Description
	if description == "" {
		description = "—"
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(stream.Name), "",
		label.Render("Description:  ") + value.Render(description),
		label.Render("Subjects:     ") + value.Render(strings.Join(stream.Subjects, ", ")),
		label.Render("Storage:      ") + value.Render(string(stream.Storage)),
		label.Render("Retention:    ") + value.Render(string(stream.Retention)),
		label.Render("Replication:  ") + value.Render(fmt.Sprintf("%d replicas", stream.Replicas)), "",
		label.Render("Messages:     ") + value.Render(compactNumber(stream.Messages)),
		label.Render("Bytes:        ") + value.Render(humanBytes(stream.Bytes)),
		label.Render("Consumers:    ") + value.Render(fmt.Sprintf("%d", stream.Consumers)),
		label.Render("Max messages: ") + value.Render(compactSigned(stream.MaxMessages)),
		label.Render("Max bytes:    ") + value.Render(humanSignedBytes(stream.MaxBytes)), "",
		label.Render("Max age:      ") + value.Render(stream.MaxAge.String()),
		label.Render("Per subject:  ") + value.Render(compactSigned(stream.MaxMessagesPerSubject)),
		label.Render("Discard:      ") + value.Render(string(stream.Discard)),
		label.Render("Compression:  ") + value.Render(onOff(stream.Compression)),
		label.Render("Direct:       ") + value.Render(onOff(stream.AllowDirect)),
		label.Render("Placement:    ") + value.Render(streamPlacement(stream)),
		label.Render("Topology:     ") + value.Render(streamTopologyLabel(stream)), "",
		label.Render("Cluster:      ") + value.Render(streamClusterHealth(stream)),
		label.Render("Dependencies: ") + value.Render(streamDependencyHealth(stream)), "",
		lipgloss.NewStyle().Foreground(m.theme.muted).Render("m messages  •  e advanced edit  •  P purge  •  d delete  •  esc back"),
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(strings.Join(lines, "\n"))
}

func compactSigned(value int64) string {
	if value < 0 {
		return "unlimited"
	}
	return compactNumber(uint64(value))
}

func humanSignedBytes(value int64) string {
	if value < 0 {
		return "unlimited"
	}
	return humanBytes(uint64(value))
}

func (m *Model) applyStreamAction(input StreamActionInput) {
	summary := streamSummaryFromDefinition(input.Definition)
	switch input.Action {
	case streams.ActionCreate:
		m.streams = append(m.streams, summary)
		m.streamCursor = len(m.streams) - 1
	case streams.ActionUpdate:
		current := m.streams[m.streamCursor]
		summary.Messages, summary.Bytes, summary.Consumers = current.Messages, current.Bytes, current.Consumers
		m.streams[m.streamCursor] = summary
	case streams.ActionDelete:
		m.streams = append(m.streams[:m.streamCursor], m.streams[m.streamCursor+1:]...)
		m.streamCursor = min(m.streamCursor, max(len(m.streams)-1, 0))
	}
}

func streamSummaryFromDefinition(d streams.Definition) streams.Summary {
	return streams.Summary{Name: d.Name, Description: d.Description, Subjects: append([]string(nil), d.Subjects...), Storage: d.Storage, Retention: d.Retention, Replicas: d.Replicas, MaxMessages: d.MaxMessages, MaxBytes: d.MaxBytes, MaxAge: d.MaxAge, MaxMessageSize: d.MaxMessageSize, MaxMessagesPerSubject: d.MaxMessagesPerSubject, DuplicateWindow: d.DuplicateWindow, MaxConsumers: d.MaxConsumers, Discard: d.Discard, DiscardNewPerSubject: d.DiscardNewPerSubject, Compression: d.Compression, AllowDirect: d.AllowDirect, MirrorDirect: d.MirrorDirect, AllowRollup: d.AllowRollup, DenyDelete: d.DenyDelete, DenyPurge: d.DenyPurge, Sealed: d.Sealed, NoAck: d.NoAck, AllowMessageTTL: d.AllowMessageTTL, PlacementCluster: d.PlacementCluster, PlacementTags: append([]string(nil), d.PlacementTags...), Metadata: d.Metadata, SubjectTransform: d.SubjectTransform, Republish: d.Republish, Mirror: d.Mirror, Sources: append([]streams.Source(nil), d.Sources...)}
}

func streamPlacement(stream streams.Summary) string {
	parts := append([]string(nil), stream.PlacementTags...)
	if stream.PlacementCluster != "" {
		parts = append([]string{stream.PlacementCluster}, parts...)
	}
	if len(parts) == 0 {
		return "automatic"
	}
	return strings.Join(parts, ", ")
}

func streamTopologyLabel(stream streams.Summary) string {
	if stream.Mirror != nil {
		return "mirror of " + stream.Mirror.Name
	}
	if len(stream.Sources) > 0 {
		names := make([]string, 0, len(stream.Sources))
		for _, source := range stream.Sources {
			names = append(names, source.Name)
		}
		return "sources " + strings.Join(names, ", ")
	}
	return "standalone"
}

func streamClusterHealth(stream streams.Summary) string {
	if stream.Cluster == "" && stream.Leader == "" {
		return "standalone"
	}
	healthy := 0
	for _, peer := range stream.ReplicaHealth {
		if peer.Current && !peer.Offline {
			healthy++
		}
	}
	return fmt.Sprintf("%s · leader %s · %d/%d replicas current", valueOrDash(stream.Cluster), valueOrDash(stream.Leader), healthy, len(stream.ReplicaHealth))
}

func streamDependencyHealth(stream streams.Summary) string {
	if stream.MirrorHealth != nil {
		return sourceHealthLabel("← ", *stream.MirrorHealth)
	}
	if len(stream.SourceHealth) == 0 {
		return "—"
	}
	parts := make([]string, 0, len(stream.SourceHealth))
	for _, source := range stream.SourceHealth {
		parts = append(parts, sourceHealthLabel("← ", source))
	}
	return strings.Join(parts, "  ")
}

func sourceHealthLabel(prefix string, health streams.SourceHealth) string {
	status := fmt.Sprintf("lag %d", health.Lag)
	if health.Error != "" {
		status = "error: " + health.Error
	}
	return prefix + health.Name + " (" + status + ")"
}
func valueOrDash(value string) string {
	if value == "" {
		return "—"
	}
	return value
}
