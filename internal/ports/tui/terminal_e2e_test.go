//go:build e2e

package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clcsolutions/nats-tui/internal/app/activity"
	"github.com/clcsolutions/nats-tui/internal/app/preferences"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
)

type terminalChunks struct{ values []string }

func (r *terminalChunks) Read(buffer []byte) (int, error) {
	if len(r.values) == 0 {
		return 0, io.EOF
	}
	value := r.values[0]
	r.values = r.values[1:]
	return copy(buffer, []byte(value)), nil
}

func runTerminalE2E(t *testing.T, model Model, input []string, options ...tea.ProgramOption) (Model, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output := &bytes.Buffer{}
	base := []tea.ProgramOption{tea.WithContext(ctx), tea.WithInput(&terminalChunks{values: input}), tea.WithOutput(output), tea.WithoutSignalHandler()}
	program := tea.NewProgram(model, append(base, options...)...)
	result, err := program.Run()
	if err != nil {
		t.Fatalf("terminal program: %v\noutput:\n%s", err, output.String())
	}
	final, ok := result.(Model)
	if !ok {
		t.Fatalf("final model type = %T", result)
	}
	return final, output.String()
}

func TestE2EKeyboardWorkspaceNavigationAndRendering(t *testing.T) {
	model := dashboardModel(t, 100, 30)
	final, output := runTerminalE2E(t, model, []string{"3", "q"})
	if final.activeWorkspace != 2 {
		t.Fatalf("workspace = %d, want consumers", final.activeWorkspace)
	}
	for _, expected := range []string{"NATS", "Dashboard", "Consumer"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("terminal output missing %q", expected)
		}
	}
}

func TestE2ELateSplashTimerCannotReplaceWorkspace(t *testing.T) {
	runtime := &controlledRuntime{now: time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)}
	model := dashboardModel(t, 100, 30).WithRuntime(runtime)
	final, _ := runTerminalE2E(t, model, []string{"q"})
	if final.screen != screenWorkspace {
		t.Fatalf("screen = %v, want workspace", final.screen)
	}
}

func TestE2ESGRMouseClicksWorkspaceTab(t *testing.T) {
	prefs, err := preferences.New(true, "midnight", preferences.Bindings{})
	if err != nil {
		t.Fatal(err)
	}
	model := dashboardModel(t, 120, 32).WithUX(prefs)
	region := workspaceTabRegions(116)[3]
	x, y := 2+region.start+1, 4 // SGR coordinates are one-based; model tab row is zero-based 3.
	input := fmt.Sprintf("\x1b[<0;%d;%dM", x, y)
	final, output := runTerminalE2E(t, model, []string{input, "q"}, tea.WithMouseCellMotion())
	if final.activeWorkspace != 3 {
		t.Fatalf("workspace = %d, want messages\noutput:\n%s", final.activeWorkspace, output)
	}
	if !strings.Contains(output, "\x1b[?1002h") {
		t.Fatal("terminal mouse tracking was not enabled")
	}
}

func TestE2ESGRMouseWheelMovesSelection(t *testing.T) {
	prefs, _ := preferences.New(true, "midnight", preferences.Bindings{})
	model := dashboardModel(t, 100, 30).WithUX(prefs)
	model.activeWorkspace = 1
	model.streams = []streams.Summary{{Name: "ONE"}, {Name: "TWO"}, {Name: "THREE"}}
	final, _ := runTerminalE2E(t, model, []string{"\x1b[<65;10;10M", "q"}, tea.WithMouseCellMotion())
	if final.streamCursor != 1 {
		t.Fatalf("stream cursor = %d, want 1", final.streamCursor)
	}
}

func TestE2ESGRMouseDoubleClickOpensResource(t *testing.T) {
	prefs, _ := preferences.New(true, "midnight", preferences.Bindings{})
	model := dashboardModel(t, 100, 30).WithUX(prefs)
	model.activeWorkspace = 1
	model.streams = []streams.Summary{{Name: "ORDERS"}}
	click := "\x1b[<0;10;9M"
	final, _ := runTerminalE2E(t, model, []string{click, click, "\x03"}, tea.WithMouseCellMotion())
	if !final.showStreamDetail {
		t.Fatal("double click did not open stream detail")
	}
}

func TestE2EActivityOverlayKeyboardJourney(t *testing.T) {
	model := dashboardModel(t, 100, 30)
	model.notify(activity.Error, "Permission denied", "cannot inspect $SYS")
	final, _ := runTerminalE2E(t, model, []string{"\x07", "\x03"}) // ctrl+g, ctrl+c
	if !final.ux.open {
		t.Fatal("activity overlay did not open")
	}
	output := final.View()
	for _, expected := range []string{"Activity & errors", "Permission denied", "cannot inspect $SYS"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("terminal output missing %q", expected)
		}
	}
}

func TestE2EResponsiveFramesRemainExactSize(t *testing.T) {
	for _, viewport := range []struct {
		name          string
		width, height int
	}{{"wide", 160, 40}, {"medium", 80, 24}, {"narrow", 42, 20}} {
		t.Run(viewport.name, func(t *testing.T) {
			model := dashboardModel(t, viewport.width, viewport.height)
			frame := model.View()
			if got := lipgloss.Width(frame); got != viewport.width {
				t.Fatalf("width=%d want=%d", got, viewport.width)
			}
			if got := lipgloss.Height(frame); got != viewport.height {
				t.Fatalf("height=%d want=%d", got, viewport.height)
			}
		})
	}
}
