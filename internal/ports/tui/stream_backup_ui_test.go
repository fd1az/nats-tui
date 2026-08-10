package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
)

func TestStreamBackupFormIsResponsiveAndSubmitsSafeDefaults(t *testing.T) {
	t.Parallel()
	var submitted streams.BackupFile
	model := dashboardModel(t, 80, 24).WithStreamTransfers(func(command streams.BackupFile) tea.Cmd {
		submitted = command
		return func() tea.Msg { return StreamTransferFailed(nil) }
	}, nil, nil)
	model.activeWorkspace = 1
	model.streams = []streams.Summary{{Name: "ORDERS", Storage: streams.StorageFile}}
	opened, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	form := opened.(Model)
	if form.streamTransferForm == nil || form.streamTransferForm.directory != "backups/ORDERS" || !form.streamTransferForm.includeConsumers {
		t.Fatalf("form=%+v", form.streamTransferForm)
	}
	for _, size := range [][2]int{{80, 24}, {160, 44}} {
		view := form.streamTransferFormView(size[0], size[1])
		if lipgloss.Width(view) > size[0] || !strings.Contains(view, "compatible with the official NATS CLI") || strings.Contains(view, "\x1b[48;") {
			t.Fatalf("invalid %dx%d view (%d columns):\n%s", size[0], size[1], lipgloss.Width(view), view)
		}
	}
	starting, command := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if command == nil || submitted.Stream != "ORDERS" || submitted.Directory != "backups/ORDERS" || !submitted.IncludeConsumers {
		t.Fatalf("submitted=%+v", submitted)
	}
	if !starting.(Model).streamTransferStarting || starting.(Model).streamTransferForm != nil {
		t.Fatal("backup did not enter starting state")
	}
}

func TestStreamRestoreWorksWithEmptyCatalogAndRequiresExplicitConfirmation(t *testing.T) {
	t.Parallel()
	var submitted streams.RestoreFile
	model := dashboardModel(t, 100, 28).WithStreamTransfers(nil, func(command streams.RestoreFile) tea.Cmd {
		submitted = command
		return func() tea.Msg { return StreamTransferFailed(nil) }
	}, nil)
	model.activeWorkspace = 1
	opened, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	form := opened.(Model)
	if form.streamTransferForm == nil || form.streamTransferForm.mode != streamRestore {
		t.Fatal("restore form did not open without streams")
	}
	withoutConfirmation, command := form.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if command != nil || withoutConfirmation.(Model).streamTransferErr == nil {
		t.Fatal("restore started without confirmation")
	}
	ready := withoutConfirmation.(Model)
	ready.streamTransferForm.stream = "ORDERS"
	ready.streamTransferForm.directory = "/tmp/orders"
	ready.streamTransferForm.field = 2
	toggled, _ := ready.Update(tea.KeyMsg{Type: tea.KeySpace})
	started, command := toggled.(Model).Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if command == nil || submitted.Stream != "ORDERS" || submitted.Directory != "/tmp/orders" || !started.(Model).streamTransferStarting {
		t.Fatalf("submitted=%+v started=%+v", submitted, started.(Model).streamTransferStarting)
	}
}

func TestStreamTransferProgressCanBeCanceled(t *testing.T) {
	t.Parallel()
	canceled := false
	model := dashboardModel(t, 100, 28).WithStreamTransfers(nil, nil, func() { canceled = true })
	model.activeWorkspace = 1
	model.streamTransferActive = true
	model.streamTransferDirection = "backup"
	model.streamTransferName = "ORDERS"
	model.streamTransferDirectory = "backups/ORDERS"
	model.streamTransferStartedAt = model.runtime.Now()
	model.streamTransferProgress = streams.TransferProgress{Transferred: 128 * 1024, Chunks: 1}
	if view := model.View(); !strings.Contains(view, "KB transferred") || strings.Contains(view, "\x1b[48;") {
		t.Fatalf("progress view=%s", view)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !canceled || !updated.(Model).streamTransferCanceling {
		t.Fatal("escape did not cancel stream transfer")
	}
}
