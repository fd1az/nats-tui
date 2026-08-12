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

func accessible() theme {
	return theme{
		border: lipgloss.Color("#8A8A8A"), primary: lipgloss.Color("#FFFFFF"), secondary: lipgloss.Color("#D7D7D7"),
		muted: lipgloss.Color("#AFAFAF"), accent: lipgloss.Color("#5FD7FF"), cyan: lipgloss.Color("#00FFFF"),
		mint: lipgloss.Color("#5FFF87"), yellow: lipgloss.Color("#FFFF00"), red: lipgloss.Color("#FF5F5F"),
	}
}

func monochrome() theme {
	white, gray, dark := lipgloss.Color("#FFFFFF"), lipgloss.Color("#BFBFBF"), lipgloss.Color("#777777")
	return theme{border: dark, primary: white, secondary: gray, muted: dark, accent: white, cyan: white, mint: white, yellow: gray, red: white}
}
