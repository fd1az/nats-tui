package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clcsolutions/nats-tui/internal/app/kvstore"
)

func (m Model) updateKeyValueHistoryNavigation(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	page := max(m.height/2, 1)
	switch key.String() {
	case "q":
		return m, tea.Quit
	case "esc":
		if m.keyValueHistoryErr != nil {
			m.keyValueHistoryErr = nil
			return m, nil
		}
		if m.showKeyValueHistoryEntry {
			m.showKeyValueHistoryEntry = false
			m.keyValueHistoryOffset = 0
			return m, nil
		}
		m.showKeyValueHistory = false
	case "down", "j":
		if m.showKeyValueHistoryEntry {
			m.keyValueHistoryOffset++
			return m, nil
		}
		m.keyValueHistoryCursor = min(m.keyValueHistoryCursor+1, max(len(m.keyValueHistory)-1, 0))
	case "up", "k":
		if m.showKeyValueHistoryEntry {
			m.keyValueHistoryOffset = max(m.keyValueHistoryOffset-1, 0)
			return m, nil
		}
		m.keyValueHistoryCursor = max(m.keyValueHistoryCursor-1, 0)
	case "pgdown", "ctrl+d":
		if m.showKeyValueHistoryEntry {
			m.keyValueHistoryOffset += page
			return m, nil
		}
		m.keyValueHistoryCursor = min(m.keyValueHistoryCursor+page, max(len(m.keyValueHistory)-1, 0))
	case "pgup", "ctrl+u":
		if m.showKeyValueHistoryEntry {
			m.keyValueHistoryOffset = max(m.keyValueHistoryOffset-page, 0)
			return m, nil
		}
		m.keyValueHistoryCursor = max(m.keyValueHistoryCursor-page, 0)
	case "home", "g":
		m.keyValueHistoryCursor = 0
		m.keyValueHistoryOffset = 0
	case "end", "G":
		m.keyValueHistoryCursor = max(len(m.keyValueHistory)-1, 0)
	case "enter":
		if !m.showKeyValueHistoryEntry && len(m.keyValueHistory) > 0 {
			m.showKeyValueHistoryEntry = true
			m.keyValueHistoryOffset = 0
		}
	case "R":
		if m.showKeyValueHistoryEntry && len(m.keyValueHistory) > 0 {
			entry := m.keyValueHistory[m.keyValueHistoryCursor]
			if entry.Operation == kvstore.OperationPut {
				m.pendingKeyValueRestore = &entry
			}
		}
	case "C":
		if m.showKeyValueHistoryEntry && len(m.keyValueHistory) > 0 && m.copyValue != nil {
			entry := m.keyValueHistory[m.keyValueHistoryCursor]
			if err := m.copyValue(string(entry.Value)); err != nil {
				m.keyValueEntryWriteErr = fmt.Errorf("copy value: %w", err)
			} else {
				m.keyValueCopyNotice = fmt.Sprintf("Revision %d copied to clipboard", entry.Revision)
			}
		}
	}
	return m, nil
}

func (m Model) keyValueHistoryView(width, height int) string {
	if m.showKeyValueHistoryEntry && len(m.keyValueHistory) > 0 {
		return m.keyValueHistoricalEntryView(width, height, m.keyValueHistory[m.keyValueHistoryCursor])
	}
	heading := "Key-Value Stores  ›  Revision History"
	if m.keyValueEntry != nil {
		heading = "Key-Value Stores  ›  " + m.keyValueEntry.Bucket + "  ›  " + m.keyValueEntry.Key + "  ›  Revision History"
	}
	title := lipgloss.NewStyle().Foreground(m.theme.muted).Render(heading)
	if len(m.keyValueHistory) == 0 {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.secondary).Render("No retained revisions found.") + "\n\n" + lipgloss.NewStyle().Foreground(m.theme.muted).Render("esc current detail")
	}
	capacity := max(height-5, 1)
	start, end := visibleWindow(len(m.keyValueHistory), m.keyValueHistoryCursor, capacity)
	rows := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		rows = append(rows, m.keyValueHistoryRow(m.keyValueHistory[index], index == m.keyValueHistoryCursor, width))
	}
	count := lipgloss.NewStyle().Foreground(m.theme.muted).Render(fmt.Sprintf("%d-%d / %d revisions", start+1, end, len(m.keyValueHistory)))
	header := lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("TIMELINE") + "  " + count
	hint := lipgloss.NewStyle().Foreground(m.theme.muted).Render("↑/↓ select  •  enter inspect value  •  esc current detail")
	return title + "\n\n" + header + "\n" + strings.Join(rows, "\n") + "\n" + hint
}

func (m Model) keyValueHistoryRow(entry kvstore.Entry, selected bool, width int) string {
	marker := "  "
	style := lipgloss.NewStyle().Foreground(m.theme.secondary)
	if selected {
		marker = "› "
		style = style.Bold(true).Foreground(m.theme.accent).Underline(true)
	}
	if width < 72 {
		return style.Render(truncate(fmt.Sprintf("%s#%-6d %-7s %s", marker, entry.Revision, entry.Operation, entry.Created.Local().Format("15:04:05")), width))
	}
	line := fmt.Sprintf("%s#%-8d %-7s  %s  delta %-4d  %8s", marker, entry.Revision, entry.Operation, entry.Created.Local().Format("2006-01-02 15:04:05.000"), entry.Delta, humanBytes(uint64(len(entry.Value))))
	return style.Render(truncate(line, width))
}

func (m Model) keyValueHistoricalEntryView(width, height int, entry kvstore.Entry) string {
	label := lipgloss.NewStyle().Foreground(m.theme.muted)
	heading := label.Render(fmt.Sprintf("Key-Value Stores  ›  %s  ›  %s  ›  Historical revision %d", entry.Bucket, entry.Key, entry.Revision))
	metadata := []string{
		label.Render("Operation") + "  " + lipgloss.NewStyle().Bold(true).Foreground(m.theme.mint).Render(string(entry.Operation)),
		label.Render("Created") + "    " + lipgloss.NewStyle().Foreground(m.theme.secondary).Render(entry.Created.Local().Format("2006-01-02 15:04:05.000 MST")),
		label.Render("Delta") + "      " + lipgloss.NewStyle().Foreground(m.theme.secondary).Render(fmt.Sprintf("%d", entry.Delta)),
	}
	payloadHeight := max(height-len(metadata)-7, 1)
	lines := wrapLines(prettyPayload(entry.Value), max(width-2, 1))
	offset := min(m.keyValueHistoryOffset, max(len(lines)-payloadHeight, 0))
	end := min(offset+payloadHeight, len(lines))
	visible := make([]string, 0, end-offset)
	for _, line := range lines[offset:end] {
		visible = append(visible, m.highlightJSONLine(line))
	}
	hint := label.Render("↑/↓ scroll  •  C copy  •  R restore this revision  •  esc timeline")
	return heading + "\n\n" + strings.Join(metadata, "\n") + "\n\n" + lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("VALUE") + "\n" + strings.Join(visible, "\n") + "\n" + hint
}

func (m Model) updateKeyValueRestoreConfirmation(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "n", "esc":
		m.pendingKeyValueRestore = nil
	case "y", "enter":
		if m.keyValueRestore == nil {
			m.pendingKeyValueRestore = nil
			m.keyValueEntryWriteErr = fmt.Errorf("Key-Value restore is unavailable")
			return m, nil
		}
		entry := *m.pendingKeyValueRestore
		input := KeyValueRestoreInput{Generation: m.refreshGeneration, Command: kvstore.RestoreEntry{Bucket: entry.Bucket, Key: entry.Key, Revision: entry.Revision}}
		m.pendingKeyValueRestore = nil
		m.keyValueEntrySaving = true
		return m, m.keyValueRestore(input)
	}
	return m, nil
}

func (m Model) keyValueRestoreConfirmationView(width, height int) string {
	entry := *m.pendingKeyValueRestore
	content := strings.Join([]string{
		lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("Restore historical revision?"), "",
		lipgloss.NewStyle().Foreground(m.theme.primary).Render(fmt.Sprintf("%s / %s  •  revision %d", entry.Bucket, entry.Key, entry.Revision)), "",
		lipgloss.NewStyle().Foreground(m.theme.secondary).Render("The historical value becomes a new latest revision."),
		lipgloss.NewStyle().Foreground(m.theme.mint).Render("CAS protects against concurrent changes."), "",
		lipgloss.NewStyle().Foreground(m.theme.muted).Render("y restore   n/esc cancel"),
	}, "\n")
	card := lipgloss.NewStyle().Width(min(max(width-12, 38), 80)).Padding(2, 3).BorderStyle(lipgloss.RoundedBorder()).BorderForeground(m.theme.accent).Render(content)
	return m.centeredCanvas(width, height, card)
}
