package tui

import (
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/clcsolutions/nats-tui/internal/app/objectstore"
)

func waitForObjectWatchEvent(generation uint64, events <-chan objectstore.ObjectEvent) tea.Cmd {
	return func() tea.Msg {
		event, open := <-events
		if !open {
			return objectWatchEndedMsg{generation: generation}
		}
		return objectWatchEventMsg{generation: generation, event: event, events: events}
	}
}

func (m *Model) applyObjectWatchEvent(event objectstore.ObjectEvent) {
	selectedID, selectedName := m.selectedObjectIdentity()
	index := findObjectByIdentity(m.objects, event.Object.ID, event.Object.Name)
	if event.Kind == objectstore.ObjectDeleted {
		if index >= 0 {
			m.objects = append(m.objects[:index], m.objects[index+1:]...)
		}
	} else if index >= 0 {
		m.objects[index] = event.Object
	} else {
		m.objects = append(m.objects, event.Object)
	}
	sort.Slice(m.objects, func(left, right int) bool { return m.objects[left].Name < m.objects[right].Name })
	m.rebuildObjectSearch()
	m.objectObjectCursor = findDisplayedObject(m.displayedObjects(), selectedID, selectedName)
	if len(m.displayedObjects()) == 0 {
		m.showObjectDetail = false
	}
}

func (m Model) selectedObjectIdentity() (string, string) {
	objects := m.displayedObjects()
	if m.objectObjectCursor < 0 || m.objectObjectCursor >= len(objects) {
		return "", ""
	}
	return objects[m.objectObjectCursor].ID, objects[m.objectObjectCursor].Name
}

func findObjectByIdentity(objects []objectstore.ObjectSummary, id, name string) int {
	for index, object := range objects {
		if id != "" && object.ID == id {
			return index
		}
		if id == "" && object.Name == name {
			return index
		}
	}
	return -1
}

func findDisplayedObject(objects []objectstore.ObjectSummary, id, name string) int {
	index := findObjectByIdentity(objects, id, name)
	if index < 0 {
		return 0
	}
	return index
}
