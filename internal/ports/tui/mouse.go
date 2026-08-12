package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type hitRegion struct{ start, end, workspace int }

func (m Model) updateMouse(message tea.MouseMsg) (tea.Model, tea.Cmd) {
	if !m.ux.preferences.Mouse {
		return m, nil
	}
	event := tea.MouseEvent(message)
	if event.Button == tea.MouseButtonWheelUp {
		return m.updateWorkspace(tea.KeyMsg{Type: tea.KeyUp})
	}
	if event.Button == tea.MouseButtonWheelDown {
		return m.updateWorkspace(tea.KeyMsg{Type: tea.KeyDown})
	}
	if event.Button == tea.MouseButtonBackward && event.Action == tea.MouseActionPress {
		return m.updateWorkspace(tea.KeyMsg{Type: tea.KeyEsc})
	}
	if event.Button == tea.MouseButtonForward && event.Action == tea.MouseActionPress {
		return m.updateWorkspace(tea.KeyMsg{Type: tea.KeyEnter})
	}
	if event.Button != tea.MouseButtonLeft || event.Action != tea.MouseActionPress {
		return m, nil
	}

	if m.screen == screenWelcome {
		return m.showConnection()
	}
	if m.screen == screenConnection {
		if event.Y >= m.height-8 {
			return m.startConnection()
		}
		return m, nil
	}
	if m.screen != screenWorkspace {
		return m, nil
	}
	if m.ux.open {
		m.ux.open = false
		return m, nil
	}
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}
	if m.mouseOverlayOpen() {
		return m, nil
	}

	contentX := event.X - 2
	if event.Y >= 2 && event.Y <= 4 {
		for _, region := range workspaceTabRegions(max(m.width-4, 1)) {
			if contentX >= region.start && contentX < region.end {
				m.activeWorkspace = region.workspace
				m.showStreamDetail, m.showConsumerDetail, m.showMessageDetail, m.showObjectDetail = false, false, false, false
				return m, nil
			}
		}
	}
	if event.Y >= m.height-3 {
		footer := workspaceFooter(max(m.width-4, 1), m.ux.preferences.Keys)
		for _, key := range []string{m.ux.preferences.Keys.Commands, m.ux.preferences.Keys.Bookmarks, m.ux.preferences.Keys.Activity, m.ux.preferences.Keys.Help, m.ux.preferences.Keys.Quit} {
			if start := strings.Index(footer, key+" "); start >= 0 && contentX >= start && contentX < start+len(key)+12 {
				return m.updateWorkspace(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
			}
		}
	}
	row, hit := m.mouseRowIndex(event.Y)
	if !hit {
		return m, nil
	}
	selected, ok := m.selectMouseRow(row)
	if !ok {
		return m, nil
	}
	now := m.runtime.Now()
	double := !m.ux.lastClick.at.IsZero() && m.ux.lastClick.workspace == m.activeWorkspace && m.ux.lastClick.row == selected && now.Sub(m.ux.lastClick.at) <= 500*time.Millisecond
	m.ux.lastClick = mouseClick{workspace: m.activeWorkspace, row: selected, at: now}
	if double {
		return m.updateWorkspace(tea.KeyMsg{Type: tea.KeyEnter})
	}
	return m, nil
}

func (m Model) mouseOverlayOpen() bool {
	return m.commandBar.open || m.bookmarks.open || m.editingForm() || m.pendingConsumerAction != 0 ||
		m.confirmStreamDelete || m.confirmMessageDelete || m.confirmKeyValueDelete || m.confirmObjectDelete ||
		m.confirmObjectBucketDelete || m.pendingKeyValueEntryRemoval != "" || m.pendingKeyValueRestore != nil ||
		m.topology.pending != nil || m.connections.pending != nil || m.operator.confirm != nil
}

func (m Model) mouseRowIndex(y int) (int, bool) {
	rowStart := 0
	switch m.activeWorkspace {
	case 1:
		rowStart = 8
		if m.width-4 < 90 {
			rowStart = 7
		}
	case 2, 4, 6, 9:
		rowStart = 9
	case 3, 5:
		rowStart = 10
		if m.activeWorkspace == 5 && !m.browsingObjects {
			rowStart = 9
		}
	case 8:
		rowStart = 8
	case 10:
		rowStart = 9
	case 11:
		rowStart = 7
	default:
		return 0, false
	}
	displayed := y - rowStart
	if displayed < 0 {
		return 0, false
	}
	start := 0
	capacity := max(m.height-rowStart-3, 1)
	switch m.activeWorkspace {
	case 2:
		start, _ = visibleWindow(len(m.consumers), m.consumerCursor, capacity)
	case 3:
		start, _ = visibleWindow(len(m.displayedMessages()), m.messageCursor, capacity)
	case 4:
		if m.browsingKeyValue {
			start, _ = visibleWindow(len(m.displayedKeyValueKeys()), m.keyValueKeyCursor, capacity)
		} else {
			start, _ = visibleWindow(len(m.keyValueBuckets), m.keyValueCursor, capacity)
		}
	case 5:
		if m.browsingObjects {
			start, _ = visibleWindow(len(m.displayedObjects()), m.objectObjectCursor, capacity)
		} else {
			start, _ = visibleWindow(len(m.objectBuckets), m.objectCursor, capacity)
		}
	case 6:
		start, _ = visibleWindow(len(m.displayedSubjects()), m.subjectCursor, capacity)
	case 11:
		start = max(m.alerts.cursor-capacity+1, 0)
	}
	return start + displayed, true
}

func workspaceTabRegions(width int) []hitRegion {
	regions := make([]hitRegion, 0, len(workspaces))
	cursor := 0
	for index, item := range workspaces {
		label := item.tab
		padding := 2
		if width < 160 {
			label = item.short
			padding = 0
		}
		shortcut := string(rune('1' + index))
		if index == 9 {
			shortcut = "0"
		}
		if index == 10 {
			shortcut = "A"
		}
		if index == 11 {
			shortcut = "!"
		}
		tabWidth := lipgloss.Width(shortcut+" "+label) + padding
		regions = append(regions, hitRegion{start: cursor, end: cursor + tabWidth, workspace: index})
		cursor += tabWidth + 2
	}
	return regions
}

func (m *Model) selectMouseRow(row int) (int, bool) {
	switch m.activeWorkspace {
	case 1:
		if len(m.streams) == 0 {
			return 0, false
		}
		row = min(row, len(m.streams)-1)
		m.streamCursor = row
	case 2:
		if len(m.consumers) == 0 {
			return 0, false
		}
		row = min(row, len(m.consumers)-1)
		m.consumerCursor = row
	case 3:
		values := m.displayedMessages()
		if len(values) == 0 {
			return 0, false
		}
		row = min(row, len(values)-1)
		m.messageCursor = row
	case 4:
		if m.browsingKeyValue {
			values := m.displayedKeyValueKeys()
			if len(values) == 0 {
				return 0, false
			}
			row = min(row, len(values)-1)
			m.keyValueKeyCursor = row
		} else {
			if len(m.keyValueBuckets) == 0 {
				return 0, false
			}
			row = min(row, len(m.keyValueBuckets)-1)
			m.keyValueCursor = row
		}
	case 5:
		if m.browsingObjects {
			values := m.displayedObjects()
			if len(values) == 0 {
				return 0, false
			}
			row = min(row, len(values)-1)
			m.objectObjectCursor = row
		} else {
			if len(m.objectBuckets) == 0 {
				return 0, false
			}
			row = min(row, len(m.objectBuckets)-1)
			m.objectCursor = row
		}
	case 6:
		values := m.displayedSubjects()
		if len(values) == 0 {
			return 0, false
		}
		row = min(row, len(values)-1)
		m.subjectCursor = row
	case 8:
		values := m.topology.snapshot.Servers()
		if len(values) == 0 {
			return 0, false
		}
		row = min(row, len(values)-1)
		m.topology.cursor = row
	case 9:
		values := m.displayedConnections()
		if len(values) == 0 {
			return 0, false
		}
		row = min(row, len(values)-1)
		m.connections.cursor = row
	case 10:
		length := len(m.operator.catalog.Accounts())
		if m.operator.users {
			length = len(m.operator.catalog.Users())
		}
		if length == 0 {
			return 0, false
		}
		row = min(row, length-1)
		m.operator.cursor = row
	case 11:
		values := m.displayedAlerts()
		if len(values) == 0 {
			return 0, false
		}
		row = min(row, len(values)-1)
		m.alerts.cursor = row
	default:
		return 0, false
	}
	return row, true
}
