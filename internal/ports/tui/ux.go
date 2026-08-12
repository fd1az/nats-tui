package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fd1az/nats-tui/internal/app/activity"
	"github.com/fd1az/nats-tui/internal/app/preferences"
)

const toastLifetime = 5 * time.Second

type uxState struct {
	preferences preferences.Preferences
	activity    *activity.Log
	open        bool
	cursor      int
	toast       *activity.Event
	toastUntil  time.Time
	lastClick   mouseClick
}
type mouseClick struct {
	workspace, row int
	at             time.Time
}

func defaultPreferences() preferences.Preferences {
	value, _ := preferences.New(false, "midnight", preferences.Bindings{})
	return value
}

func (m Model) WithUX(value preferences.Preferences) Model {
	m.ux.preferences = value
	switch value.Theme {
	case preferences.ThemeAccessible:
		m.theme = accessible()
	case preferences.ThemeMono:
		m.theme = monochrome()
	default:
		m.theme = midnight()
	}
	return m
}

func (m *Model) notify(level activity.Level, title, detail string) {
	event, err := m.ux.activity.Add(level, title, detail, m.runtime.Now())
	if err != nil {
		return
	}
	m.ux.toast, m.ux.toastUntil = &event, event.At.Add(toastLifetime)
}

func (m Model) currentErrorText() string {
	errors := []error{m.err, m.refreshErr, m.messageErr, m.subjectObservationErr, m.consumerActionErr, m.streamActionErr, m.streamTransferErr, m.keyValueCreateErr, m.keyValueKeysErr, m.keyValueEntryErr, m.keyValueEntryWriteErr, m.keyValueHistoryErr, m.objectsErr, m.objectTransferErr, m.objectOperationErr, m.objectWatchErr, m.objectBucketOperationErr, m.requestReply.error, m.topology.err, m.connections.err, m.operator.err, m.alerts.err, m.commandBar.err, m.bookmarks.err}
	for _, err := range errors {
		if err != nil {
			return err.Error()
		}
	}
	return ""
}

func failureFromMessage(message tea.Msg) error {
	switch msg := message.(type) {
	case connectionFailedMsg:
		return msg.err
	case connectionProfilesFailedMsg:
		return msg.err
	case dashboardPanelsFailedMsg:
		return msg.err
	case messagesFailedMsg:
		return msg.err
	case messageExportFailedMsg:
		return msg.err
	case messageDeleteFailedMsg:
		return msg.err
	case watchFailedMsg:
		return msg.err
	case subjectObservationFailedMsg:
		return msg.err
	case refreshFailedMsg:
		return msg.err
	case consumerActionFailedMsg:
		return msg.err
	case consumerConfigFailedMsg:
		return msg.err
	case streamActionFailedMsg:
		return msg.err
	case streamPurgeFailedMsg:
		return msg.err
	case streamTransferFailedMsg:
		return msg.err
	case keyValueCreateFailedMsg:
		return msg.err
	case keyValueUpdateFailedMsg:
		return msg.err
	case keyValueDeleteFailedMsg:
		return msg.err
	case keyValueKeysFailedMsg:
		return msg.err
	case keyValueEntryFailedMsg:
		return msg.err
	case keyValuePutFailedMsg:
		return msg.err
	case keyValueRemoveFailedMsg:
		return msg.err
	case keyValueHistoryFailedMsg:
		return msg.err
	case keyValueRestoreFailedMsg:
		return msg.err
	case keyValueWatchFailedMsg:
		return msg.err
	case objectsFailedMsg:
		return msg.err
	case objectTransferFailedMsg:
		return msg.err
	case objectEditFailedMsg:
		return msg.err
	case objectDeleteFailedMsg:
		return msg.err
	case objectWatchFailedMsg:
		return msg.err
	case objectLinkFailedMsg:
		return msg.err
	case objectBucketCreateFailedMsg:
		return msg.err
	case objectBucketUpdateFailedMsg:
		return msg.err
	case objectBucketDeleteFailedMsg:
		return msg.err
	case corePublishFailedMsg:
		return msg.err
	case coreRequestFailedMsg:
		return msg.err
	case payloadFileFailedMsg:
		return msg.err
	case requestHistoryFailedMsg:
		return msg.err
	case requestTemplatesFailedMsg:
		return msg.err
	case requestTemplateSaveFailedMsg:
		return msg.err
	case requestTemplateDeleteFailedMsg:
		return msg.err
	case topologyFailedMsg:
		return msg.err
	case topologyOperationFailedMsg:
		return msg.err
	case connectionsFailedMsg:
		return msg.err
	case connectionCloseFailedMsg:
		return msg.err
	case operatorFailedMsg:
		return msg.err
	case operatorAccountCreateFailedMsg:
		return msg.err
	case operatorUserCreateFailedMsg:
		return msg.err
	case operatorMutationFailedMsg:
		return msg.err
	case alertsFailedMsg:
		return msg.err
	case alertAckFailedMsg:
		return msg.err
	case commandBarFailedMsg:
		return msg.err
	case bookmarksFailedMsg:
		return msg.err
	}
	return nil
}

func successFromMessage(message tea.Msg) string {
	switch message.(type) {
	case messageExportSucceededMsg:
		return "Messages exported"
	case messageDeleteSucceededMsg:
		return "Stored message deleted"
	case consumerActionSucceededMsg:
		return "Consumer updated"
	case consumerConfigSucceededMsg:
		return "Consumer configuration saved"
	case streamActionSucceededMsg:
		return "Stream updated"
	case streamPurgeSucceededMsg:
		return "Stream purged"
	case keyValueCreateSucceededMsg:
		return "KV bucket created"
	case keyValueUpdateSucceededMsg:
		return "KV bucket updated"
	case keyValueDeleteSucceededMsg:
		return "KV bucket deleted"
	case keyValuePutSucceededMsg:
		return "KV value saved"
	case keyValueRemoveSucceededMsg:
		return "KV value removed"
	case objectEditSucceededMsg:
		return "Object updated"
	case objectDeleteSucceededMsg:
		return "Object deleted"
	case objectLinkSucceededMsg:
		return "Object link created"
	case objectBucketCreateSucceededMsg:
		return "Object bucket created"
	case objectBucketUpdateSucceededMsg:
		return "Object bucket updated"
	case objectBucketDeleteSucceededMsg:
		return "Object bucket deleted"
	case corePublishSucceededMsg:
		return "Message published"
	case topologyOperationSucceededMsg:
		return "Topology operation completed"
	case connectionClosedMsg:
		return "Connection closed"
	case operatorAccountCreatedMsg:
		return "Account created"
	case operatorUserCreatedMsg:
		return "User created"
	case operatorMutationSucceededMsg:
		return "Operator state updated"
	case bookmarkAddedMsg:
		return "Bookmark saved"
	case bookmarkRemovedMsg:
		return "Bookmark removed"
	}
	return ""
}

func (m Model) toastView(width int) string {
	if m.ux.toast == nil || !m.runtime.Now().Before(m.ux.toastUntil) {
		return ""
	}
	color, icon := m.theme.secondary, "●"
	switch m.ux.toast.Level {
	case activity.Success:
		color, icon = m.theme.mint, "✓"
	case activity.Warning:
		color, icon = m.theme.yellow, "!"
	case activity.Error:
		color, icon = m.theme.red, "×"
	}
	text := icon + " " + m.ux.toast.Title
	if m.ux.toast.Detail != "" {
		text += " — " + m.ux.toast.Detail
	}
	return lipgloss.NewStyle().Foreground(color).Bold(true).Render(truncate(text, max(width, 1)))
}

func (m Model) activityView(width, height int) string {
	events := m.ux.activity.Events()
	title := lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Render("Activity & errors")
	lines := []string{title, lipgloss.NewStyle().Foreground(m.theme.muted).Render("Newest operational events for this session."), ""}
	if len(events) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.muted).Render("No activity yet."))
	} else {
		available := max(height-9, 1)
		start := max(len(events)-available, 0)
		for index := len(events) - 1; index >= start; index-- {
			event := events[index]
			color := m.theme.secondary
			if event.Level == activity.Error {
				color = m.theme.red
			} else if event.Level == activity.Success {
				color = m.theme.mint
			} else if event.Level == activity.Warning {
				color = m.theme.yellow
			}
			line := fmt.Sprintf("%s  %-7s  %s", event.At.Format("15:04:05"), strings.ToUpper(string(event.Level)), event.Title)
			if event.Detail != "" {
				line += " — " + event.Detail
			}
			lines = append(lines, lipgloss.NewStyle().Foreground(color).Render(truncate(line, max(width-10, 1))))
		}
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(m.theme.muted).Render("esc/"+m.ux.preferences.Keys.Activity+" close   d clear log"))
	card := lipgloss.NewStyle().Width(min(max(width-12, 36), 110)).Padding(1, 3).BorderStyle(lipgloss.RoundedBorder()).BorderForeground(m.theme.border).Render(strings.Join(lines, "\n"))
	return m.centeredCanvas(width, height, card)
}

func (m Model) updateActivity(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", m.ux.preferences.Keys.Activity:
		m.ux.open = false
	case "d":
		m.ux.activity.Clear()
		m.ux.toast = nil
	}
	return m, nil
}
