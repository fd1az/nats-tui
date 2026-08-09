package tui

import "github.com/charmbracelet/lipgloss"

type theme struct {
	border    lipgloss.Color
	primary   lipgloss.Color
	secondary lipgloss.Color
	muted     lipgloss.Color
	accent    lipgloss.Color
	cyan      lipgloss.Color
	mint      lipgloss.Color
	yellow    lipgloss.Color
	red       lipgloss.Color
}

func midnight() theme {
	return theme{
		border:    lipgloss.Color("#30364F"),
		primary:   lipgloss.Color("#E8EAF2"),
		secondary: lipgloss.Color("#9AA3BD"),
		muted:     lipgloss.Color("#626B85"),
		accent:    lipgloss.Color("#7C9DFF"),
		cyan:      lipgloss.Color("#67D4E8"),
		mint:      lipgloss.Color("#6ED6A8"),
		yellow:    lipgloss.Color("#F1C75B"),
		red:       lipgloss.Color("#F07178"),
	}
}
