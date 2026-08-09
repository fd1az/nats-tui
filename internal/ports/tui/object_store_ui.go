package tui

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clcsolutions/nats-tui/internal/app/objectstore"
)

func (m Model) objectStoreView(width, height int) string {
	heading := lipgloss.NewStyle().Foreground(m.theme.muted).Render("Dashboard  ›  Object Stores")
	if m.objectBucketForm != nil {
		return m.objectBucketFormView(width, height)
	}
	if m.confirmObjectBucketDelete && len(m.objectBuckets) > 0 {
		return m.objectBucketDeleteConfirmationView(width, height)
	}
	if m.objectBucketOperationNotice != "" {
		heading += "  " + lipgloss.NewStyle().Foreground(m.theme.mint).Render("✓ "+m.objectBucketOperationNotice)
	}
	if m.objectBucketOperationErr != nil {
		heading += "  " + lipgloss.NewStyle().Foreground(m.theme.red).Render("✗ "+m.objectBucketOperationErr.Error())
	}
	if m.objectBucketOperationRunning {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.accent).Render(m.spinner.View()+" Applying bucket change…")
	}
	if m.objectEditForm != nil {
		return m.objectEditFormView(width, height)
	}
	if m.objectLinkForm != nil {
		return m.objectLinkFormView(width, height)
	}
	if m.confirmObjectDelete && len(m.displayedObjects()) > 0 {
		return m.objectDeleteConfirmationView(width, height)
	}
	if m.objectOperationRunning {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.accent).Render(m.spinner.View()+" Applying object change…")
	}
	if m.objectTransferForm != nil || m.objectTransferStarting || m.objectTransferActive || m.objectTransferCanceling {
		return m.objectTransferView(width, height)
	}
	if m.objectTransferErr != nil {
		content := strings.Join([]string{
			lipgloss.NewStyle().Bold(true).Foreground(m.theme.red).Render("Transfer failed"), "",
			lipgloss.NewStyle().Foreground(m.theme.secondary).Render(m.objectTransferErr.Error()), "",
			lipgloss.NewStyle().Foreground(m.theme.muted).Render("esc dismiss"),
		}, "\n")
		card := lipgloss.NewStyle().Width(min(max(width-12, 38), 82)).Padding(2, 3).Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.red).Render(content)
		return m.centeredCanvas(width, height, card)
	}
	if m.loadingObjects {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.accent).Render(m.spinner.View()+" Loading object metadata…")
	}
	if m.browsingObjects {
		return m.objectBrowserView(width, height)
	}
	if len(m.objectBuckets) == 0 {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.secondary).Render("No Object Store buckets found in this JetStream account.") + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render("n create bucket  •  r refresh")
	}
	bodyHeight := max(height-3, 3)
	if width < 100 {
		return heading + "\n\n" + m.objectStoreList(width, bodyHeight)
	}
	leftWidth := max(width*3/5, 42)
	rightWidth := max(width-leftWidth-3, 26)
	left := m.objectStoreList(leftWidth, bodyHeight)
	right := m.objectStoreDetail(rightWidth, m.objectBuckets[m.objectCursor])
	return heading + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right)
}

func (m Model) objectStoreList(width, height int) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("OBJECT BUCKETS")
	count := lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("%d buckets", len(m.objectBuckets)))
	header := title + strings.Repeat(" ", max(width-lipgloss.Width(title)-lipgloss.Width(count), 1)) + count
	columns := lipgloss.NewStyle().Foreground(m.theme.muted).Render("  NAME" + strings.Repeat(" ", max(width-28, 1)) + "SIZE      STORE")
	capacity := max(height-4, 1)
	start, end := visibleWindow(len(m.objectBuckets), m.objectCursor, capacity)
	rows := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		bucket := m.objectBuckets[index]
		marker := "  "
		style := lipgloss.NewStyle().Foreground(m.theme.secondary)
		if index == m.objectCursor {
			marker = "› "
			style = style.Foreground(m.theme.accent).Bold(true).Underline(true)
		}
		nameWidth := max(width-24, 10)
		rows = append(rows, style.Render(truncate(fmt.Sprintf("%s%-*s %9s  %s", marker, nameWidth, truncate(bucket.Name, nameWidth), humanBytes(bucket.Size), bucket.Storage), width)))
	}
	hint := lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ select  •  n create  •  e edit  •  d delete  •  enter browse  •  r refresh")
	return strings.Join([]string{header, columns, strings.Join(rows, "\n"), hint}, "\n")
}

func (m Model) objectStoreDetail(width int, bucket objectstore.BucketSummary) string {
	label := lipgloss.NewStyle().Foreground(m.theme.muted)
	value := lipgloss.NewStyle().Foreground(m.theme.secondary)
	description := bucket.Description
	if description == "" {
		description = "—"
	}
	state := "writable"
	if bucket.Sealed {
		state = "sealed"
	}
	compression := "off"
	if bucket.Compressed {
		compression = "on"
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("BUCKET DETAIL"), "",
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.primary).Render(truncate(bucket.Name, width)),
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render(truncate(description, width)), "",
		label.Render("Size:         ") + value.Render(humanBytes(bucket.Size)),
		label.Render("Storage:      ") + value.Render(bucket.Storage),
		label.Render("Replicas:     ") + value.Render(fmt.Sprintf("R%d", bucket.Replicas)),
		label.Render("TTL:          ") + value.Render(humanDuration(bucket.TTL)),
		label.Render("Max bytes:    ") + value.Render(objectBucketMaxBytes(bucket.MaxBytes)),
		label.Render("Compression:  ") + value.Render(compression),
		label.Render("State:        ") + value.Render(state), "",
	}
	if bucket.Mirror != "" {
		lines = append(lines, label.Render("Mirror:       ")+value.Render(bucket.Mirror+" (read-only)"))
	}
	if len(bucket.Sources) > 0 {
		lines = append(lines, label.Render("Sources:      ")+value.Render(strings.Join(bucket.Sources, ", ")))
	}
	lines = appendDetailMap(lines, "METADATA", bucket.Metadata, width, label, value)
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("enter  →  browse objects"))
	return strings.Join(lines, "\n")
}

func objectBucketMaxBytes(value int64) string {
	if value < 0 {
		return "unlimited"
	}
	return humanBytes(uint64(value))
}

func (m Model) objectBrowserView(width, height int) string {
	heading := lipgloss.NewStyle().Foreground(m.theme.muted).Render("Object Stores  ›  " + m.objectBucket)
	if source := m.objectMirrorSource(); source != "" {
		heading += "  " + lipgloss.NewStyle().Bold(true).Foreground(m.theme.yellow).Render("MIRROR of "+source+" • mirror is read-only")
	}
	if m.objectWatchStarting {
		heading += "  " + lipgloss.NewStyle().Foreground(m.theme.secondary).Render(m.spinner.View()+" WATCH")
	} else if m.objectWatching {
		status := fmt.Sprintf("● LIVE  %d", m.objectWatchReceived)
		if m.objectWatchPaused {
			status = fmt.Sprintf("Ⅱ PAUSED  %d", m.objectWatchReceived)
		}
		heading += "  " + lipgloss.NewStyle().Bold(true).Foreground(m.theme.mint).Render(status)
	}
	if m.objectWatchErr != nil {
		heading += "  " + lipgloss.NewStyle().Foreground(m.theme.red).Render("watch: "+m.objectWatchErr.Error())
	}
	if m.objectOperationNotice != "" {
		heading += "  " + lipgloss.NewStyle().Foreground(m.theme.mint).Render("✓ "+m.objectOperationNotice)
	}
	if m.objectOperationErr != nil {
		heading += "  " + lipgloss.NewStyle().Foreground(m.theme.red).Render("✗ "+m.objectOperationErr.Error())
	}
	if m.objectTransferNotice != "" {
		heading += "  " + lipgloss.NewStyle().Foreground(m.theme.mint).Render("✓ "+m.objectTransferNotice)
	}
	if m.objectsErr != nil {
		return heading + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.red).Render("Could not load objects: "+m.objectsErr.Error()) + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render("r retry  •  esc buckets")
	}
	objects := m.displayedObjects()
	if len(objects) == 0 {
		search := m.objectSearchView()
		message := "No objects found in this bucket."
		if strings.TrimSpace(m.objectSearchQuery) != "" {
			message = "No objects match the current search."
		}
		hint := "L create link  •  W live watch  •  u upload  •  / search  •  r reload"
		if m.objectMirrorSource() != "" {
			hint = "mirror is read-only  •  W live watch  •  / search  •  r reload"
		}
		return heading + "\n" + search + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.secondary).Render(message) + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render(hint)
	}
	selected := objects[min(m.objectObjectCursor, len(objects)-1)]
	if m.showObjectDetail || width < 92 {
		if m.showObjectDetail {
			return heading + "\n\n" + m.objectDetailView(width, max(height-2, 3), selected, true)
		}
		return heading + "\n" + m.objectSearchView() + "\n\n" + m.objectListPane(width, max(height-4, 3), objects)
	}
	leftWidth := max(width*11/20, 44)
	rightWidth := max(width-leftWidth-3, 30)
	left := m.objectSearchView() + "\n" + m.objectListPane(leftWidth, max(height-3, 3), objects)
	right := m.objectDetailView(rightWidth, max(height-2, 3), selected, false)
	return heading + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right)
}

func (m Model) objectSearchView() string {
	query := m.objectSearchQuery
	if query == "" {
		query = "type / to search name, digest, metadata…"
	}
	marker := "  "
	style := lipgloss.NewStyle().Foreground(m.theme.muted)
	if m.objectSearchMode {
		marker = "› "
		style = style.Foreground(m.theme.accent).Bold(true)
		if m.objectSearchQuery == "" {
			query = ""
		}
		query += "▏"
	}
	return style.Render(marker + "/ " + query)
}

func (m Model) objectListPane(width, height int, objects []objectstore.ObjectSummary) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("OBJECTS")
	count := lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("%d / %d", len(objects), len(m.objects)))
	header := title + strings.Repeat(" ", max(width-lipgloss.Width(title)-lipgloss.Width(count), 1)) + count
	columns := lipgloss.NewStyle().Foreground(m.theme.muted).Render("  NAME" + strings.Repeat(" ", max(width-36, 1)) + "SIZE   CHUNKS  MODIFIED")
	capacity := max(height-4, 1)
	start, end := visibleWindow(len(objects), m.objectObjectCursor, capacity)
	rows := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		object := objects[index]
		marker := "  "
		style := lipgloss.NewStyle().Foreground(m.theme.secondary)
		if index == m.objectObjectCursor {
			marker = "› "
			style = style.Foreground(m.theme.accent).Bold(true).Underline(true)
		}
		nameWidth := max(width-32, 10)
		modified := "—"
		if !object.ModifiedAt.IsZero() {
			modified = object.ModifiedAt.Local().Format("Jan 02 15:04")
		}
		displayName := object.Name
		if object.Link != nil {
			displayName = "↗ " + displayName
		}
		line := fmt.Sprintf("%s%-*s %8s  %4d  %s", marker, nameWidth, truncate(displayName, nameWidth), humanBytes(object.Size), object.Chunks, modified)
		rows = append(rows, style.Render(truncate(line, width)))
	}
	hintText := "↑/↓ select  •  enter detail  •  L link  •  W live  •  space pause  •  e edit  •  d delete  •  u/D transfer"
	if m.objectMirrorSource() != "" {
		hintText = "↑/↓ select  •  enter detail  •  W live  •  space pause  •  D download  •  mirror is read-only"
	}
	hint := lipgloss.NewStyle().Foreground(m.theme.muted).Render(hintText)
	return strings.Join([]string{header, columns, strings.Join(rows, "\n"), hint}, "\n")
}

func (m Model) objectMirrorSource() string {
	for _, bucket := range m.objectBuckets {
		if bucket.Name == m.objectBucket {
			return bucket.Mirror
		}
	}
	return ""
}

func (m *Model) rejectObjectMirrorMutation() bool {
	if m.objectMirrorSource() == "" {
		return false
	}
	m.objectOperationErr = errors.New("mirror is read-only; mutate its source bucket instead")
	m.objectOperationNotice = ""
	return true
}

func (m Model) objectDetailView(width, height int, object objectstore.ObjectSummary, standalone bool) string {
	label := lipgloss.NewStyle().Foreground(m.theme.muted)
	value := lipgloss.NewStyle().Foreground(m.theme.secondary)
	modified := "—"
	if !object.ModifiedAt.IsZero() {
		modified = object.ModifiedAt.Local().Format("2006-01-02 15:04:05.000 MST")
	}
	description := object.Description
	if description == "" {
		description = "—"
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("OBJECT DETAIL"), "",
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.primary).Render(truncate(object.Name, width)),
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render(truncate(description, width)), "",
		label.Render("Size:       ") + value.Render(humanBytes(object.Size)),
		label.Render("Chunks:     ") + value.Render(fmt.Sprintf("%d", object.Chunks)),
		label.Render("Modified:   ") + value.Render(modified),
		label.Render("Digest:     ") + value.Render(truncate(valueOr(object.Digest, "—"), max(width-12, 8))),
	}
	if object.Link != nil {
		kind := "object"
		target := object.Link.Bucket + "/" + object.Link.Name
		if object.Link.Name == "" {
			kind = "bucket"
			target = object.Link.Bucket
		}
		lines = append(lines,
			label.Render("Type:       ")+value.Render(kind+" link"),
			label.Render("Target:     ")+lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render(truncate(target, max(width-12, 8))),
		)
	}
	lines = appendDetailMap(lines, "METADATA", object.Metadata, width, label, value)
	headerValues := make(map[string]string, len(object.Headers))
	for name, values := range object.Headers {
		headerValues[name] = strings.Join(values, ", ")
	}
	lines = appendDetailMap(lines, "HEADERS", headerValues, width, label, value)
	hint := "enter full detail"
	if standalone {
		hint = "↑/↓ scroll  •  esc object list"
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render(hint))
	if standalone && len(lines) > 1 {
		body := lines[1:]
		capacity := max(height-2, 1)
		offset := min(m.objectDetailOffset, max(len(body)-capacity, 0))
		end := min(offset+capacity, len(body))
		position := label.Render(fmt.Sprintf("lines %d-%d / %d", min(offset+1, len(body)), end, len(body)))
		visible := []string{lines[0] + strings.Repeat(" ", max(width-lipgloss.Width(lines[0])-lipgloss.Width(position), 1)) + position}
		visible = append(visible, body[offset:end]...)
		return strings.Join(visible, "\n")
	}
	return strings.Join(lines, "\n")
}

func appendDetailMap(lines []string, title string, values map[string]string, width int, label, value lipgloss.Style) []string {
	if len(values) == 0 {
		return lines
	}
	lines = append(lines, "", lipgloss.NewStyle().Bold(true).Foreground(label.GetForeground()).Render(title))
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		line := label.Render(key+": ") + value.Render(values[key])
		lines = append(lines, truncate(line, width))
	}
	return lines
}

func (m *Model) rebuildObjectSearch() {
	m.objectSearchDocuments = make([]string, len(m.objects))
	for index, object := range m.objects {
		parts := []string{object.Name, object.Description, object.Digest}
		if object.Link != nil {
			parts = append(parts, object.Link.Bucket, object.Link.Name, "link")
		}
		for name, value := range object.Metadata {
			parts = append(parts, name, value)
		}
		for name, values := range object.Headers {
			parts = append(parts, name, strings.Join(values, " "))
		}
		m.objectSearchDocuments[index] = strings.ToLower(strings.Join(parts, "\x00"))
	}
	m.filterObjects()
}

func (m *Model) filterObjects() {
	query := strings.ToLower(strings.TrimSpace(m.objectSearchQuery))
	if query == "" {
		m.filteredObjects = append(m.filteredObjects[:0], m.objects...)
		m.objectObjectCursor = min(m.objectObjectCursor, max(len(m.filteredObjects)-1, 0))
		return
	}
	m.filteredObjects = m.filteredObjects[:0]
	for index, document := range m.objectSearchDocuments {
		if strings.Contains(document, query) {
			m.filteredObjects = append(m.filteredObjects, m.objects[index])
		}
	}
	m.objectObjectCursor = 0
}

func (m Model) displayedObjects() []objectstore.ObjectSummary {
	if strings.TrimSpace(m.objectSearchQuery) == "" {
		return m.objects
	}
	return m.filteredObjects
}

func selectedObjectName(objects []objectstore.ObjectSummary, cursor int) string {
	if cursor < 0 || cursor >= len(objects) {
		return ""
	}
	return objects[cursor].Name
}

func findObject(objects []objectstore.ObjectSummary, name string) int {
	for index, object := range objects {
		if object.Name == name {
			return index
		}
	}
	return 0
}

func (m Model) updateObjectSearch(key tea.KeyMsg) Model {
	switch key.String() {
	case "esc", "enter":
		m.objectSearchMode = false
	case "ctrl+u":
		m.objectSearchQuery = ""
		m.filterObjects()
	case "backspace":
		runes := []rune(m.objectSearchQuery)
		if len(runes) > 0 {
			m.objectSearchQuery = string(runes[:len(runes)-1])
			m.filterObjects()
		}
	default:
		if key.Type == tea.KeyRunes {
			m.objectSearchQuery += string(key.Runes)
			m.filterObjects()
		}
	}
	return m
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
