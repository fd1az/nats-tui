package tui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clcsolutions/nats-tui/internal/app/kvstore"
)

const keyValueFormFieldCount = 11

type keyValueForm struct {
	editing      bool
	field        int
	name         string
	description  string
	storage      kvstore.Storage
	history      string
	ttl          string
	maxBytes     string
	maxValueSize string
	replicas     string
	compression  bool
	mirror       string
	sources      string
}

func newKeyValueForm() keyValueForm {
	return keyValueForm{storage: kvstore.StorageFile, history: "1", ttl: "0", maxBytes: "-1", maxValueSize: "-1", replicas: "1"}
}

func newEditKeyValueForm(bucket kvstore.BucketSummary) keyValueForm {
	return keyValueForm{
		editing: true, field: 1, name: bucket.Name, description: bucket.Description, storage: bucket.Storage,
		history: strconv.FormatInt(bucket.History, 10), ttl: durationInput(bucket.TTL),
		maxBytes: strconv.FormatInt(bucket.MaxBytes, 10), maxValueSize: strconv.FormatInt(int64(bucket.MaxValueSize), 10),
		replicas: strconv.Itoa(bucket.Replicas), compression: bucket.Compressed,
		mirror: bucket.Mirror, sources: strings.Join(bucket.Sources, ", "),
	}
}

func durationInput(duration time.Duration) string {
	if duration == 0 {
		return "0"
	}
	return duration.String()
}

func (f keyValueForm) definition() (kvstore.Definition, error) {
	history, err := strconv.Atoi(strings.TrimSpace(f.history))
	if err != nil {
		return kvstore.Definition{}, fmt.Errorf("history must be an integer")
	}
	ttl, err := time.ParseDuration(strings.TrimSpace(f.ttl))
	if err != nil {
		return kvstore.Definition{}, fmt.Errorf("TTL must be a duration such as 1h or 30m")
	}
	maxBytes, err := strconv.ParseInt(strings.TrimSpace(f.maxBytes), 10, 64)
	if err != nil {
		return kvstore.Definition{}, fmt.Errorf("max bytes must be an integer")
	}
	maxValueSize, err := strconv.ParseInt(strings.TrimSpace(f.maxValueSize), 10, 32)
	if err != nil {
		return kvstore.Definition{}, fmt.Errorf("max value size must be an integer")
	}
	replicas, err := strconv.Atoi(strings.TrimSpace(f.replicas))
	if err != nil {
		return kvstore.Definition{}, fmt.Errorf("replicas must be an integer")
	}
	definition := kvstore.Definition{
		Name: strings.TrimSpace(f.name), Description: strings.TrimSpace(f.description), Storage: f.storage,
		History: history, TTL: ttl, MaxBytes: maxBytes, MaxValueSize: int32(maxValueSize), Replicas: replicas, Compression: f.compression,
		Mirror: strings.TrimSpace(f.mirror), Sources: splitBucketNames(f.sources),
	}
	if err := definition.Validate(); err != nil {
		return kvstore.Definition{}, err
	}
	return definition, nil
}

func splitBucketNames(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if name := strings.TrimSpace(part); name != "" {
			result = append(result, name)
		}
	}
	return result
}

func (m Model) updateKeyValueForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.keyValueCreateRunning {
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.keyValueForm = nil
		m.keyValueCreateErr = nil
	case "ctrl+s":
		return m.submitKeyValueForm()
	case "tab", "down":
		m.moveKeyValueFormField(1)
	case "shift+tab", "up":
		m.moveKeyValueFormField(-1)
	case "left", "right", " ":
		if m.keyValueForm.field == 2 {
			m.toggleKeyValueStorage()
		}
		if m.keyValueForm.field == 8 {
			m.keyValueForm.compression = !m.keyValueForm.compression
		}
	case "backspace":
		m.editKeyValueField(true, "")
	default:
		if key.Type == tea.KeyRunes {
			m.editKeyValueField(false, string(key.Runes))
		}
	}
	return m, nil
}

func (m Model) submitKeyValueForm() (tea.Model, tea.Cmd) {
	if m.keyValueForm.editing {
		return m.submitKeyValueUpdate()
	}
	if m.keyValueCreate == nil {
		m.keyValueCreateErr = errors.New("Key-Value operations are unavailable")
		return m, nil
	}
	definition, err := m.keyValueForm.definition()
	if err != nil {
		m.keyValueCreateErr = err
		return m, nil
	}
	input := KeyValueCreateInput{Generation: m.refreshGeneration, Definition: definition}
	m.keyValueCreateRunning = true
	m.keyValueCreateErr = nil
	m.keyValueCreateNotice = ""
	return m, m.keyValueCreate(input)
}

func (m Model) submitKeyValueUpdate() (tea.Model, tea.Cmd) {
	if m.keyValueUpdate == nil {
		m.keyValueCreateErr = errors.New("Key-Value update is unavailable")
		return m, nil
	}
	definition, err := m.keyValueForm.definition()
	if err != nil {
		m.keyValueCreateErr = err
		return m, nil
	}
	description, history, ttl := definition.Description, definition.History, definition.TTL
	maxBytes, maxValueSize, replicas, compression := definition.MaxBytes, definition.MaxValueSize, definition.Replicas, definition.Compression
	changes := kvstore.Changes{
		Description: &description, History: &history, TTL: &ttl, MaxBytes: &maxBytes,
		MaxValueSize: &maxValueSize, Replicas: &replicas, Compression: &compression,
	}
	input := KeyValueUpdateInput{Generation: m.refreshGeneration, Name: definition.Name, Changes: changes}
	m.keyValueCreateRunning = true
	m.keyValueCreateErr = nil
	m.keyValueCreateNotice = ""
	return m, m.keyValueUpdate(input)
}

func (m *Model) moveKeyValueFormField(step int) {
	for {
		m.keyValueForm.field = (m.keyValueForm.field + step + keyValueFormFieldCount) % keyValueFormFieldCount
		if !m.keyValueForm.editing || (m.keyValueForm.field != 0 && m.keyValueForm.field != 2 && m.keyValueForm.field != 9 && m.keyValueForm.field != 10) {
			return
		}
	}
}

func (m *Model) toggleKeyValueStorage() {
	if m.keyValueForm.storage == kvstore.StorageFile {
		m.keyValueForm.storage = kvstore.StorageMemory
		return
	}
	m.keyValueForm.storage = kvstore.StorageFile
}

func (m *Model) editKeyValueField(backspace bool, addition string) {
	if m.keyValueForm.editing && m.keyValueForm.field == 0 {
		return
	}
	field := m.keyValueTextField()
	if field == nil {
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

func (m *Model) keyValueTextField() *string {
	switch m.keyValueForm.field {
	case 0:
		return &m.keyValueForm.name
	case 1:
		return &m.keyValueForm.description
	case 3:
		return &m.keyValueForm.history
	case 4:
		return &m.keyValueForm.ttl
	case 5:
		return &m.keyValueForm.maxBytes
	case 6:
		return &m.keyValueForm.maxValueSize
	case 7:
		return &m.keyValueForm.replicas
	case 9:
		return &m.keyValueForm.mirror
	case 10:
		return &m.keyValueForm.sources
	default:
		return nil
	}
}

func (m Model) keyValueFormView(width, height int) string {
	title := "Create KV bucket"
	if m.keyValueForm.editing {
		title = "Edit KV bucket"
	}
	nameLabel, storageLabel := "Name", "Storage"
	if m.keyValueForm.editing {
		nameLabel, storageLabel = "Name (immutable)", "Storage (immutable)"
	}
	fields := []struct{ label, value string }{
		{nameLabel, m.keyValueForm.name}, {"Description", m.keyValueForm.description}, {storageLabel, string(m.keyValueForm.storage)},
		{"History", m.keyValueForm.history}, {"TTL", m.keyValueForm.ttl}, {"Max bucket bytes", m.keyValueForm.maxBytes},
		{"Max value size", m.keyValueForm.maxValueSize}, {"Replicas", m.keyValueForm.replicas}, {"Compression", enabledLabel(m.keyValueForm.compression)},
		{"Mirror bucket", m.keyValueForm.mirror}, {"Source buckets", m.keyValueForm.sources},
	}
	cardWidth := min(max(width-14, 38), 78)
	cardWidth = min(cardWidth, max(width-6, 1))
	progress := lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("Field %d of %d", m.keyValueForm.field+1, keyValueFormFieldCount))
	heading := lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(title)
	heading += strings.Repeat(" ", max(cardWidth-lipgloss.Width(heading)-lipgloss.Width(progress), 1)) + progress
	compact := height < 28
	lines := []string{heading}
	if !compact {
		lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.secondary).Render("Configure the JetStream bucket. Defaults are safe for local use."))
	}
	sections := map[int]string{0: "GENERAL", 3: "RETENTION", 5: "LIMITS", 7: "AVAILABILITY"}
	for index, field := range fields {
		if section, ok := sections[index]; ok && height >= 30 {
			lines = append(lines, "", lipgloss.NewStyle().Bold(true).Foreground(m.theme.muted).Render(section))
		}
		lines = append(lines, m.keyValueFormRow(cardWidth, index, field.label, field.value))
	}
	if m.keyValueCreateErr != nil {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.red).Render(m.keyValueCreateErr.Error()))
	}
	if m.keyValueCreateRunning {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.yellow).Render("↻ creating KV bucket"))
	}
	if height >= 24 {
		help := keyValueFieldHelp(m.keyValueForm.field)
		if !compact {
			lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.border).Render(strings.Repeat("─", cardWidth)), lipgloss.NewStyle().Bold(true).Foreground(m.theme.cyan).Render("WHAT TO PUT HERE"))
		}
		lines = append(lines, lipgloss.NewStyle().Width(cardWidth).Foreground(m.theme.secondary).Render(help))
	}
	action := "create"
	if m.keyValueForm.editing {
		action = "save changes"
	}
	shortcut := "tab next  •  ↑/↓ navigate  •  ←/→ choose  •  ctrl+s " + action + "  •  esc cancel"
	if compact {
		shortcut = "tab next  •  ctrl+s " + action + "  •  esc cancel"
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render(shortcut))
	cardStyle := lipgloss.NewStyle().
		Width(cardWidth + 4).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.border)
	if compact {
		cardStyle = cardStyle.Padding(0, 2)
	} else {
		cardStyle = cardStyle.Padding(1, 2)
	}
	card := cardStyle.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, card)
}

func (m Model) keyValueFormRow(width, index int, label, rawValue string) string {
	selected := index == m.keyValueForm.field
	marker := "  "
	labelStyle := lipgloss.NewStyle().Foreground(m.theme.muted)
	valueStyle := lipgloss.NewStyle().Foreground(m.theme.secondary)
	if selected {
		marker = lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("› ")
		labelStyle = labelStyle.Bold(true).Foreground(m.theme.primary)
		valueStyle = valueStyle.Bold(true).Foreground(m.theme.accent)
	}
	value := strings.TrimSpace(rawValue)
	if value == "" {
		value = "(empty)"
		valueStyle = valueStyle.Bold(false).Foreground(m.theme.muted)
	}
	if selected && index != 2 && index != 8 {
		value += " ▏"
	}
	labelWidth := min(max(width/3, 10), 24)
	available := max(width-labelWidth-4, 8)
	return marker + labelStyle.Width(labelWidth).Render(truncate(label, labelWidth)) + "  " + valueStyle.Render(tailTruncate(value, available))
}

func keyValueFieldHelp(field int) string {
	help := []string{
		"Required. Use letters, numbers, dashes, or underscores. Example: SESSIONS.",
		"Optional. Describe what the bucket owns so operators can recognize it quickly.",
		"FILE survives server restarts. MEMORY is faster and ephemeral; use it for disposable data.",
		"Number of revisions retained per key, from 1 to 64. Use more than 1 when rollback or auditing matters.",
		"How long values live. Use durations such as 30m, 1h, or 24h. Set 0 and entries never expire.",
		"Maximum total bucket capacity in bytes. Set -1 for unlimited.",
		"Maximum size of one value in bytes. Set -1 for unlimited.",
		"Copies stored across the NATS cluster, from 1 to 5. Production commonly uses 3.",
		"Compress values on disk. Useful for JSON and text; it trades some CPU for lower storage usage.",
		"Optional, create-only. Mirror one existing KV bucket continuously. Cannot be combined with sources.",
		"Optional, create-only. Comma-separated KV buckets to aggregate. Cannot be combined with mirror.",
	}
	if field < 0 || field >= len(help) {
		return ""
	}
	return help[field]
}

func enabledLabel(enabled bool) string {
	if enabled {
		return "ON"
	}
	return "OFF"
}

func (m *Model) applyCreatedKeyValueBucket(definition kvstore.Definition) {
	m.keyValueBuckets = append(m.keyValueBuckets, kvstore.BucketSummary{
		Name: definition.Name, Description: definition.Description, History: int64(definition.History), TTL: definition.TTL,
		Storage: definition.Storage, Replicas: definition.Replicas, Compressed: definition.Compression,
		MaxBytes: definition.MaxBytes, MaxValueSize: definition.MaxValueSize,
		Mirror: definition.Mirror, Sources: append([]string(nil), definition.Sources...),
	})
	m.keyValueCursor = findBucket(m.keyValueBuckets, definition.Name)
}

func (m *Model) applyUpdatedKeyValueBucket(name string, changes kvstore.Changes) {
	index := findBucket(m.keyValueBuckets, name)
	if index < 0 || index >= len(m.keyValueBuckets) || m.keyValueBuckets[index].Name != name {
		return
	}
	bucket := &m.keyValueBuckets[index]
	if changes.Description != nil {
		bucket.Description = *changes.Description
	}
	if changes.History != nil {
		bucket.History = int64(*changes.History)
	}
	if changes.TTL != nil {
		bucket.TTL = *changes.TTL
	}
	if changes.MaxBytes != nil {
		bucket.MaxBytes = *changes.MaxBytes
	}
	if changes.MaxValueSize != nil {
		bucket.MaxValueSize = *changes.MaxValueSize
	}
	if changes.Replicas != nil {
		bucket.Replicas = *changes.Replicas
	}
	if changes.Compression != nil {
		bucket.Compressed = *changes.Compression
	}
}

func (m Model) updateKeyValueDeleteConfirmation(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "y", "enter":
		m.confirmKeyValueDelete = false
		return m.runKeyValueDelete()
	case "n", "esc":
		m.confirmKeyValueDelete = false
	}
	return m, nil
}

func (m Model) runKeyValueDelete() (tea.Model, tea.Cmd) {
	if m.keyValueDelete == nil {
		m.keyValueCreateErr = errors.New("Key-Value deletion is unavailable")
		return m, nil
	}
	bucket := m.keyValueBuckets[m.keyValueCursor]
	input := KeyValueDeleteInput{Generation: m.refreshGeneration, Name: bucket.Name}
	m.keyValueCreateRunning = true
	m.keyValueCreateErr = nil
	m.keyValueCreateNotice = ""
	return m, m.keyValueDelete(input)
}

func (m *Model) removeKeyValueBucket(name string) {
	index := findBucket(m.keyValueBuckets, name)
	if index < 0 || index >= len(m.keyValueBuckets) || m.keyValueBuckets[index].Name != name {
		return
	}
	m.keyValueBuckets = append(m.keyValueBuckets[:index], m.keyValueBuckets[index+1:]...)
	m.keyValueCursor = min(m.keyValueCursor, max(len(m.keyValueBuckets)-1, 0))
}

func (m Model) keyValueDeleteConfirmationView(width, height int) string {
	bucket := m.keyValueBuckets[m.keyValueCursor]
	content := strings.Join([]string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.red).Render("Delete KV bucket?"), "",
		lipgloss.NewStyle().Foreground(m.theme.primary).Render(bucket.Name), "",
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render(fmt.Sprintf("This permanently deletes %s values using %s.", compactNumber(bucket.Values), humanBytes(bucket.Bytes))),
		lipgloss.NewStyle().Foreground(m.theme.red).Render("All values and revision history will be lost. This cannot be undone."), "",
		lipgloss.NewStyle().Foreground(m.theme.muted).Render("y confirm   n/esc cancel"),
	}, "\n")
	card := lipgloss.NewStyle().Width(min(max(width-12, 36), 82)).Padding(2, 3).BorderStyle(lipgloss.RoundedBorder()).BorderForeground(m.theme.red).Render(content)
	return m.centeredCanvas(width, height, card)
}

func selectedBucketName(buckets []kvstore.BucketSummary, cursor int) string {
	if cursor < 0 || cursor >= len(buckets) {
		return ""
	}
	return buckets[cursor].Name
}

func findBucket(buckets []kvstore.BucketSummary, name string) int {
	for index, bucket := range buckets {
		if bucket.Name == name {
			return index
		}
	}
	return 0
}

func (m Model) keyValueView(width, height int) string {
	heading := lipgloss.NewStyle().Foreground(m.theme.muted).Render("Dashboard  ›  Key-Value Stores") + m.keyValueCreateFeedback()
	if m.loadingKeyValueHistory {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.accent).Render(m.spinner.View()+" Loading revision history…")
	}
	if m.keyValueHistoryErr != nil {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.red).Render("Could not load history: "+m.keyValueHistoryErr.Error()) + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render("esc back")
	}
	if m.showKeyValueHistory {
		return m.keyValueHistoryView(width, height)
	}
	if m.loadingKeyValueEntry {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.accent).Render(m.spinner.View()+" Loading value…")
	}
	if m.keyValueEntryErr != nil {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.red).Render("Could not load value: "+m.keyValueEntryErr.Error()) + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render("esc back to keys")
	}
	if m.showKeyValueEntry && m.keyValueEntry != nil {
		return m.keyValueEntryView(width, height, *m.keyValueEntry)
	}
	if m.loadingKeyValueKeys {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.accent).Render(m.spinner.View()+" Loading keys…")
	}
	if m.keyValueKeysErr != nil {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.red).Render("Could not load keys: "+m.keyValueKeysErr.Error())
	}
	if m.browsingKeyValue {
		return m.keyValueKeysView(width, height)
	}
	if len(m.keyValueBuckets) == 0 {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.secondary).Render("No Key-Value buckets found in this account.")
	}
	paneHeight := max(height-2, 3)
	if width < 100 {
		return heading + "\n\n" + m.keyValueListPane(width, paneHeight)
	}
	leftWidth := width * 60 / 100
	rightWidth := max(width-leftWidth-3, 24)
	list := m.keyValueListPane(leftWidth, paneHeight)
	detail := m.keyValueDetailPane(rightWidth, m.keyValueBuckets[m.keyValueCursor])
	dividerHeight := max(lipgloss.Height(list), lipgloss.Height(detail))
	divider := lipgloss.NewStyle().Foreground(m.theme.border).Render(strings.TrimSuffix(strings.Repeat("│\n", dividerHeight), "\n"))
	return heading + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, list, " ", divider, " ", detail)
}

func (m Model) updateKeyValueKeySearch(key tea.KeyMsg) Model {
	switch key.String() {
	case "esc", "enter":
		m.keyValueKeySearchMode = false
	case "backspace":
		runes := []rune(m.keyValueKeyQuery)
		if len(runes) > 0 {
			m.keyValueKeyQuery = string(runes[:len(runes)-1])
			m.applyKeyValueKeySearch()
		}
	case "ctrl+u":
		m.keyValueKeyQuery = ""
		m.applyKeyValueKeySearch()
	default:
		if key.Type == tea.KeyRunes {
			m.keyValueKeyQuery += string(key.Runes)
			m.applyKeyValueKeySearch()
		}
	}
	return m
}

func (m *Model) applyKeyValueKeySearch() {
	m.keyValueKeyCursor = 0
	query := strings.ToLower(strings.TrimSpace(m.keyValueKeyQuery))
	if query == "" {
		m.filteredKeyValueKeys = nil
		return
	}
	m.filteredKeyValueKeys = m.filteredKeyValueKeys[:0]
	for _, key := range m.keyValueKeys {
		if strings.Contains(strings.ToLower(key), query) {
			m.filteredKeyValueKeys = append(m.filteredKeyValueKeys, key)
		}
	}
}

func (m Model) displayedKeyValueKeys() []string {
	if strings.TrimSpace(m.keyValueKeyQuery) == "" {
		return m.keyValueKeys
	}
	return m.filteredKeyValueKeys
}

func (m Model) keyValueKeysView(width, height int) string {
	keys := m.displayedKeyValueKeys()
	heading := lipgloss.NewStyle().Foreground(m.theme.muted).Render("Key-Value Stores  ›  " + m.keyValueBucket + "  ›  Keys")
	search := lipgloss.NewStyle().Foreground(m.theme.muted).Render("/ search keys")
	if m.keyValueKeyQuery != "" || m.keyValueKeySearchMode {
		caret := ""
		if m.keyValueKeySearchMode {
			caret = lipgloss.NewStyle().Foreground(m.theme.accent).Render("▏")
		}
		search = lipgloss.NewStyle().Foreground(m.theme.accent).Render("/ ") + m.keyValueKeyQuery + caret + lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("  %d matches", len(keys)))
	}
	if len(keys) == 0 {
		message := "This bucket has no current keys."
		if m.keyValueKeyQuery != "" {
			message = "No keys match this search."
		}
		return heading + "\n" + search + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.secondary).Render(message) + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render("esc back to buckets")
	}
	capacity := max(height-6, 1)
	start, end := visibleWindow(len(keys), m.keyValueKeyCursor, capacity)
	rows := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		marker := "  "
		style := lipgloss.NewStyle().Foreground(m.theme.secondary)
		if index == m.keyValueKeyCursor {
			marker = "› "
			style = style.Foreground(m.theme.accent).Bold(true).Underline(true)
		}
		rows = append(rows, style.Render(truncate(marker+keys[index], width)))
	}
	count := lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("%d-%d / %d", start+1, end, len(keys)))
	hint := lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ select  •  enter view  •  n new  •  / search  •  esc buckets")
	return heading + "\n" + search + "\n\n" + lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("KEYS") + "  " + count + "\n" + strings.Join(rows, "\n") + "\n" + hint
}

func (m Model) keyValueEntryView(width, height int, entry kvstore.Entry) string {
	heading := lipgloss.NewStyle().Foreground(m.theme.muted).Render("Key-Value Stores  ›  " + entry.Bucket + "  ›  " + entry.Key)
	label := lipgloss.NewStyle().Foreground(m.theme.muted)
	value := lipgloss.NewStyle().Foreground(m.theme.secondary)
	operation := lipgloss.NewStyle().Bold(true).Foreground(m.theme.mint).Render(string(entry.Operation))
	metadata := []string{
		label.Render("Operation") + "  " + operation,
		label.Render("Revision") + "   " + value.Render(fmt.Sprintf("%d", entry.Revision)),
		label.Render("Created") + "    " + value.Render(entry.Created.Local().Format("2006-01-02 15:04:05.000 MST")),
		label.Render("Delta") + "      " + value.Render(fmt.Sprintf("%d", entry.Delta)),
		label.Render("Size") + "       " + value.Render(humanBytes(uint64(len(entry.Value)))),
	}
	payloadWidth := max(width-2, 1)
	payloadHeight := max(height-len(metadata)-7, 1)
	lines := wrapLines(prettyPayload(entry.Value), payloadWidth)
	maxOffset := max(len(lines)-payloadHeight, 0)
	offset := min(m.keyValueEntryOffset, maxOffset)
	end := min(offset+payloadHeight, len(lines))
	visible := make([]string, 0, end-offset)
	for _, line := range lines[offset:end] {
		visible = append(visible, m.highlightJSONLine(line))
	}
	position := label.Render(fmt.Sprintf("lines %d-%d / %d", min(offset+1, len(lines)), end, len(lines)))
	watch := "W watch"
	if m.keyValueWatchStarting {
		watch = m.spinner.View() + " starting watch"
	} else if m.keyValueWatching {
		watch = "W stop watch ●"
	}
	hint := label.Render("↑/↓ scroll  •  C copy  •  H history  •  " + watch + "  •  e edit  •  d delete  •  P purge  •  esc keys")
	feedback := ""
	if m.keyValueCopyNotice != "" {
		feedback = "\n" + lipgloss.NewStyle().Foreground(m.theme.mint).Render("✓ "+m.keyValueCopyNotice)
	}
	return heading + "\n\n" + strings.Join(metadata, "\n") + "\n\n" + lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("VALUE") + "  " + position + "\n" + strings.Join(visible, "\n") + "\n" + hint + feedback
}

func (m Model) keyValueListPane(width, height int) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("KV BUCKETS")
	count := lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("%d buckets", len(m.keyValueBuckets)))
	header := title + strings.Repeat(" ", max(width-lipgloss.Width(title)-lipgloss.Width(count), 1)) + count
	columns := lipgloss.NewStyle().Foreground(m.theme.muted).Render("  NAME" + strings.Repeat(" ", max(width-40, 1)) + "VALUES      SIZE  STORE")
	capacity := max(height-4, 1)
	start, end := visibleWindow(len(m.keyValueBuckets), m.keyValueCursor, capacity)
	rows := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		rows = append(rows, m.keyValueRow(m.keyValueBuckets[index], index == m.keyValueCursor, width))
	}
	hint := lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ select  •  enter browse keys  •  n new bucket")
	return strings.Join([]string{header, columns, strings.Join(rows, "\n"), hint}, "\n")
}

func (m Model) keyValueCreateFeedback() string {
	if m.keyValueCreateNotice != "" {
		return "  " + lipgloss.NewStyle().Foreground(m.theme.mint).Render("✓ "+m.keyValueCreateNotice)
	}
	return ""
}

func (m Model) keyValueRow(bucket kvstore.BucketSummary, selected bool, width int) string {
	marker := "  "
	style := lipgloss.NewStyle().Foreground(m.theme.secondary)
	if selected {
		marker = "› "
		style = style.Foreground(m.theme.accent).Bold(true).Underline(true)
	}
	nameWidth := max(width-34, 10)
	line := fmt.Sprintf("%s%-*s %8s %9s  %s", marker, nameWidth, truncate(bucket.Name, nameWidth), compactNumber(bucket.Values), humanBytes(bucket.Bytes), bucket.Storage)
	return style.Render(truncate(line, width))
}

func (m Model) keyValueDetailPane(width int, bucket kvstore.BucketSummary) string {
	label := lipgloss.NewStyle().Foreground(m.theme.muted)
	value := lipgloss.NewStyle().Foreground(m.theme.secondary)
	description := bucket.Description
	if description == "" {
		description = "—"
	}
	compression := "off"
	if bucket.Compressed {
		compression = "on"
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("BUCKET DETAIL"),
		"",
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.primary).Render(truncate(bucket.Name, width)),
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render(truncate(description, width)),
		"",
		label.Render("Values:       ") + value.Render(compactNumber(bucket.Values)),
		label.Render("Size:         ") + value.Render(humanBytes(bucket.Bytes)),
		label.Render("Storage:      ") + value.Render(string(bucket.Storage)),
		label.Render("Replicas:     ") + value.Render(fmt.Sprintf("R%d", bucket.Replicas)),
		label.Render("History:      ") + value.Render(fmt.Sprintf("%d revisions/key", bucket.History)),
		label.Render("TTL:          ") + value.Render(humanDuration(bucket.TTL)),
		label.Render("Compression:  ") + value.Render(compression),
	}
	if bucket.Mirror != "" {
		lines = append(lines, label.Render("Mirror:       ")+value.Render(bucket.Mirror))
	}
	if len(bucket.Sources) > 0 {
		lines = append(lines, label.Render("Sources:      ")+value.Render(strings.Join(bucket.Sources, ", ")))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("enter  →  browse keys"))
	return strings.Join(lines, "\n")
}

func humanDuration(duration time.Duration) string {
	if duration <= 0 {
		return "unlimited"
	}
	return duration.String()
}
