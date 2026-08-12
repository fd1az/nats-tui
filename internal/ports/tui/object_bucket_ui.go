package tui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fd1az/nats-tui/internal/app/objectstore"
)

const objectBucketFormFieldCount = 10

type objectBucketForm struct {
	editing     bool
	field       int
	name        string
	description string
	storage     objectstore.Storage
	ttl         string
	maxBytes    string
	replicas    string
	compression bool
	metadata    string
	mirror      string
	sources     string
}

func newObjectBucketForm() objectBucketForm {
	return objectBucketForm{storage: objectstore.StorageFile, ttl: "0", maxBytes: "-1", replicas: "1"}
}

func newEditObjectBucketForm(bucket objectstore.BucketSummary) objectBucketForm {
	storage := objectstore.Storage(bucket.Storage)
	if storage != objectstore.StorageMemory {
		storage = objectstore.StorageFile
	}
	return objectBucketForm{
		editing: true, field: 1, name: bucket.Name, description: bucket.Description,
		storage: storage, ttl: durationInput(bucket.TTL), maxBytes: strconv.FormatInt(bucket.MaxBytes, 10),
		replicas: strconv.Itoa(bucket.Replicas), compression: bucket.Compressed,
		metadata: formatObjectProperties(bucket.Metadata),
		mirror:   bucket.Mirror, sources: strings.Join(bucket.Sources, ", "),
	}
}

func (f objectBucketForm) definition() (objectstore.Definition, error) {
	ttl, err := time.ParseDuration(strings.TrimSpace(f.ttl))
	if err != nil {
		return objectstore.Definition{}, fmt.Errorf("TTL must be a duration such as 1h or 30m")
	}
	maxBytes, err := strconv.ParseInt(strings.TrimSpace(f.maxBytes), 10, 64)
	if err != nil {
		return objectstore.Definition{}, fmt.Errorf("max bytes must be an integer")
	}
	replicas, err := strconv.Atoi(strings.TrimSpace(f.replicas))
	if err != nil {
		return objectstore.Definition{}, fmt.Errorf("replicas must be an integer")
	}
	metadata, err := parseObjectProperties(f.metadata)
	if err != nil {
		return objectstore.Definition{}, fmt.Errorf("metadata: %w", err)
	}
	definition := objectstore.Definition{
		Name: strings.TrimSpace(f.name), Description: strings.TrimSpace(f.description), Storage: f.storage,
		TTL: ttl, MaxBytes: maxBytes, Replicas: replicas, Compression: f.compression, Metadata: metadata,
		Mirror: strings.TrimSpace(f.mirror), Sources: splitBucketNames(f.sources),
	}
	if err := definition.Validate(); err != nil {
		return objectstore.Definition{}, err
	}
	return definition, nil
}

func (m Model) updateObjectBucketForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.objectBucketOperationRunning {
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.objectBucketForm = nil
		m.objectBucketOperationErr = nil
	case "ctrl+s":
		return m.submitObjectBucketForm()
	case "tab", "down":
		m.moveObjectBucketFormField(1)
	case "shift+tab", "up":
		m.moveObjectBucketFormField(-1)
	case "left", "right", " ":
		if m.objectBucketForm.field == 2 && !m.objectBucketForm.editing {
			m.toggleObjectBucketStorage()
		}
		if m.objectBucketForm.field == 6 {
			m.objectBucketForm.compression = !m.objectBucketForm.compression
		}
	case "backspace":
		m.editObjectBucketField(true, "")
	default:
		if key.Type == tea.KeyRunes {
			m.editObjectBucketField(false, string(key.Runes))
		}
	}
	return m, nil
}

func (m Model) submitObjectBucketForm() (tea.Model, tea.Cmd) {
	definition, err := m.objectBucketForm.definition()
	if err != nil {
		m.objectBucketOperationErr = err
		return m, nil
	}
	m.objectBucketOperationErr = nil
	m.objectBucketOperationNotice = ""
	if m.objectBucketForm.editing {
		if m.objectBucketUpdate == nil {
			m.objectBucketOperationErr = errors.New("Object Store bucket update is unavailable")
			return m, nil
		}
		description, ttl, maxBytes := definition.Description, definition.TTL, definition.MaxBytes
		replicas, compression, metadata := definition.Replicas, definition.Compression, definition.Metadata
		input := ObjectBucketUpdateInput{Generation: m.refreshGeneration, Name: definition.Name, Changes: objectstore.Changes{
			Description: &description, TTL: &ttl, MaxBytes: &maxBytes, Replicas: &replicas,
			Compression: &compression, Metadata: &metadata,
		}}
		m.objectBucketOperationRunning = true
		return m, m.objectBucketUpdate(input)
	}
	if m.objectBucketCreate == nil {
		m.objectBucketOperationErr = errors.New("Object Store bucket creation is unavailable")
		return m, nil
	}
	input := ObjectBucketCreateInput{Generation: m.refreshGeneration, Definition: definition}
	m.objectBucketOperationRunning = true
	return m, m.objectBucketCreate(input)
}

func (m *Model) moveObjectBucketFormField(step int) {
	for {
		m.objectBucketForm.field = (m.objectBucketForm.field + step + objectBucketFormFieldCount) % objectBucketFormFieldCount
		if !m.objectBucketForm.editing || (m.objectBucketForm.field != 0 && m.objectBucketForm.field != 2 && m.objectBucketForm.field != 8 && m.objectBucketForm.field != 9) {
			return
		}
	}
}

func (m *Model) toggleObjectBucketStorage() {
	if m.objectBucketForm.storage == objectstore.StorageFile {
		m.objectBucketForm.storage = objectstore.StorageMemory
		return
	}
	m.objectBucketForm.storage = objectstore.StorageFile
}

func (m *Model) editObjectBucketField(backspace bool, addition string) {
	if m.objectBucketForm.editing && (m.objectBucketForm.field == 0 || m.objectBucketForm.field == 2 || m.objectBucketForm.field == 8 || m.objectBucketForm.field == 9) {
		return
	}
	field := m.objectBucketTextField()
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

func (m *Model) objectBucketTextField() *string {
	switch m.objectBucketForm.field {
	case 0:
		return &m.objectBucketForm.name
	case 1:
		return &m.objectBucketForm.description
	case 3:
		return &m.objectBucketForm.ttl
	case 4:
		return &m.objectBucketForm.maxBytes
	case 5:
		return &m.objectBucketForm.replicas
	case 7:
		return &m.objectBucketForm.metadata
	case 8:
		return &m.objectBucketForm.mirror
	case 9:
		return &m.objectBucketForm.sources
	default:
		return nil
	}
}

func (m Model) objectBucketFormView(width, height int) string {
	title := "Create Object Store bucket"
	nameLabel, storageLabel := "Name", "Storage"
	if m.objectBucketForm.editing {
		title, nameLabel, storageLabel = "Edit Object Store bucket", "Name (immutable)", "Storage (immutable)"
	}
	fields := []struct{ label, value string }{
		{nameLabel, m.objectBucketForm.name}, {"Description", m.objectBucketForm.description},
		{storageLabel, string(m.objectBucketForm.storage)}, {"TTL", m.objectBucketForm.ttl},
		{"Max bucket bytes", m.objectBucketForm.maxBytes}, {"Replicas", m.objectBucketForm.replicas},
		{"Compression", enabledLabel(m.objectBucketForm.compression)}, {"Metadata", m.objectBucketForm.metadata},
		{"Mirror bucket (create-only)", m.objectBucketForm.mirror}, {"Source buckets (create-only)", m.objectBucketForm.sources},
	}
	cardWidth := min(max(width-14, 42), 82)
	cardWidth = min(cardWidth, max(width-6, 1))
	progress := lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("Field %d of %d", m.objectBucketForm.field+1, len(fields)))
	heading := lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(title)
	heading += strings.Repeat(" ", max(cardWidth-lipgloss.Width(heading)-lipgloss.Width(progress), 1)) + progress
	lines := []string{heading, lipgloss.NewStyle().Foreground(m.theme.secondary).Render("Configure object storage. Defaults are safe for local use."), ""}
	for index, field := range fields {
		lines = append(lines, m.objectBucketFormRow(cardWidth, index, field.label, field.value))
	}
	if m.objectBucketOperationErr != nil {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.red).Render(m.objectBucketOperationErr.Error()))
	}
	if m.objectBucketOperationRunning {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.yellow).Render(m.spinner.View()+" applying bucket change…"))
	}
	if height >= 23 {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.border).Render(strings.Repeat("─", cardWidth)),
			lipgloss.NewStyle().Bold(true).Foreground(m.theme.cyan).Render("WHAT TO PUT HERE"),
			lipgloss.NewStyle().Width(cardWidth).Foreground(m.theme.secondary).Render(objectBucketFieldHelp(m.objectBucketForm.field)))
	}
	action := "create"
	if m.objectBucketForm.editing {
		action = "save changes"
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("tab next  •  ↑/↓ navigate  •  ←/→ choose  •  ctrl+s "+action+"  •  esc cancel"))
	card := lipgloss.NewStyle().Width(cardWidth+4).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}

func (m Model) objectBucketFormRow(width, index int, label, rawValue string) string {
	selected := index == m.objectBucketForm.field
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
	if selected && index != 2 && index != 6 {
		value += " ▏"
	}
	labelWidth := min(max(width/3, 12), 24)
	return marker + labelStyle.Width(labelWidth).Render(truncate(label, labelWidth)) + "  " + valueStyle.Render(tailTruncate(value, max(width-labelWidth-4, 8)))
}

func objectBucketFieldHelp(field int) string {
	help := []string{
		"Required. Use letters, numbers, dashes, or underscores. Example: ARTIFACTS.",
		"Optional operator-facing purpose, such as release artifacts or generated reports.",
		"FILE survives restarts. MEMORY is fast and ephemeral; use it only for disposable objects.",
		"How long objects live. Use 30m, 1h, or 24h. Set 0 to keep them indefinitely.",
		"Maximum total bucket capacity in bytes. Set -1 for unlimited.",
		"Copies across the cluster, from 1 to 5. Production commonly uses 3.",
		"Compress chunks at rest. Effective for text and JSON, with a small CPU cost.",
		"Optional comma-separated key=value labels, for example owner=platform,env=prod.",
		"Optional, create-only. Continuously mirror one Object Store bucket. Cannot be combined with sources.",
		"Optional, create-only. Aggregate comma-separated Object Store buckets. Cannot be combined with mirror.",
	}
	if field < 0 || field >= len(help) {
		return ""
	}
	return help[field]
}

func (m Model) updateObjectBucketDeleteConfirmation(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "n", "esc":
		m.confirmObjectBucketDelete = false
		return m, nil
	case "y", "enter":
		if m.objectBucketDelete == nil {
			m.confirmObjectBucketDelete = false
			m.objectBucketOperationErr = errors.New("Object Store bucket deletion is unavailable")
			return m, nil
		}
		if len(m.objectBuckets) == 0 {
			m.confirmObjectBucketDelete = false
			return m, nil
		}
		input := ObjectBucketDeleteInput{Generation: m.refreshGeneration, Name: m.objectBuckets[m.objectCursor].Name}
		m.confirmObjectBucketDelete = false
		m.objectBucketOperationRunning = true
		m.objectBucketOperationErr = nil
		m.objectBucketOperationNotice = ""
		return m, m.objectBucketDelete(input)
	default:
		return m, nil
	}
}

func (m Model) objectBucketDeleteConfirmationView(width, height int) string {
	bucket := m.objectBuckets[m.objectCursor]
	content := strings.Join([]string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.red).Render("Delete Object Store bucket?"), "",
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.primary).Render(bucket.Name),
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render(fmt.Sprintf("This permanently removes %s of stored objects.", humanBytes(bucket.Size))), "",
		lipgloss.NewStyle().Foreground(m.theme.red).Render("All objects, links, and stored bytes will be lost. This cannot be undone."), "",
		lipgloss.NewStyle().Foreground(m.theme.muted).Render("y/enter confirm  •  n/esc cancel"),
	}, "\n")
	card := lipgloss.NewStyle().Width(min(max(width-12, 42), 82)).Padding(2, 3).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.red).Render(content)
	return m.centeredCanvas(width, height, card)
}

func objectBucketSummary(definition objectstore.Definition) objectstore.BucketSummary {
	return objectstore.BucketSummary{
		Name: definition.Name, Description: definition.Description, TTL: definition.TTL, Storage: string(definition.Storage),
		Replicas: definition.Replicas, Compressed: definition.Compression, MaxBytes: definition.MaxBytes,
		Metadata: cloneObjectBucketMetadata(definition.Metadata),
		Mirror:   definition.Mirror, Sources: append([]string(nil), definition.Sources...),
	}
}

func findObjectBucket(buckets []objectstore.BucketSummary, name string) int {
	for index, bucket := range buckets {
		if bucket.Name == name {
			return index
		}
	}
	return 0
}

func (m *Model) applyObjectBucketChanges(name string, changes objectstore.Changes) {
	index := findObjectBucket(m.objectBuckets, name)
	if index < 0 || index >= len(m.objectBuckets) || m.objectBuckets[index].Name != name {
		return
	}
	bucket := &m.objectBuckets[index]
	if changes.Description != nil {
		bucket.Description = *changes.Description
	}
	if changes.TTL != nil {
		bucket.TTL = *changes.TTL
	}
	if changes.MaxBytes != nil {
		bucket.MaxBytes = *changes.MaxBytes
	}
	if changes.Replicas != nil {
		bucket.Replicas = *changes.Replicas
	}
	if changes.Compression != nil {
		bucket.Compressed = *changes.Compression
	}
	if changes.Metadata != nil {
		bucket.Metadata = cloneObjectBucketMetadata(*changes.Metadata)
	}
}

func (m *Model) removeObjectBucket(name string) {
	index := findObjectBucket(m.objectBuckets, name)
	if index < 0 || index >= len(m.objectBuckets) || m.objectBuckets[index].Name != name {
		return
	}
	m.objectBuckets = append(m.objectBuckets[:index], m.objectBuckets[index+1:]...)
	m.objectCursor = min(m.objectCursor, max(len(m.objectBuckets)-1, 0))
}

func cloneObjectBucketMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}
	clone := make(map[string]string, len(metadata))
	for key, value := range metadata {
		clone[key] = value
	}
	return clone
}
