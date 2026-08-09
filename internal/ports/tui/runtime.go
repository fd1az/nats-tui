package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Runtime isolates wall-clock time and scheduling from the state machine.
type Runtime interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
	Tick(time.Duration, func(time.Time) tea.Msg) tea.Cmd
}

type systemRuntime struct{}

func (systemRuntime) Now() time.Time                             { return time.Now() }
func (systemRuntime) After(delay time.Duration) <-chan time.Time { return time.After(delay) }
func (systemRuntime) Tick(delay time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	return tea.Tick(delay, fn)
}
