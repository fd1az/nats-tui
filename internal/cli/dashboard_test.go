package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/clcsolutions/nats-tui/internal/app/dashboard"
	"github.com/clcsolutions/nats-tui/internal/config"
)

type stubDashboard struct {
	panels  []dashboard.Panel
	deleted string
}

func (s *stubDashboard) ListDashboardPanels(context.Context, config.Settings) ([]dashboard.Panel, error) {
	return s.panels, nil
}
func (s *stubDashboard) SaveDashboardPanel(_ context.Context, _ config.Settings, id, title string, metric dashboard.Metric) (dashboard.Panel, error) {
	panel, err := dashboard.NewPanel(id, title, metric)
	if err == nil {
		s.panels = []dashboard.Panel{panel}
	}
	return panel, err
}
func (s *stubDashboard) DeleteDashboardPanel(_ context.Context, _ config.Settings, id string) error {
	s.deleted = id
	return nil
}

func TestDashboardPanelCommands(t *testing.T) {
	t.Parallel()
	service := &stubDashboard{}
	add := NewRoot(Dependencies{Dashboard: service})
	output := &strings.Builder{}
	add.SetOut(output)
	add.SetArgs([]string{"dashboard", "panel", "add", "traffic", "message-rate", "--title", "Traffic"})
	if err := add.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(service.panels) != 1 || service.panels[0].Title != "Traffic" {
		t.Fatalf("panels = %#v", service.panels)
	}
	list := NewRoot(Dependencies{Dashboard: service})
	output.Reset()
	list.SetOut(output)
	list.SetArgs([]string{"dashboard", "panel"})
	if err := list.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "traffic\tTraffic\tmessage-rate") {
		t.Fatalf("output = %q", output.String())
	}
	remove := NewRoot(Dependencies{Dashboard: service})
	remove.SetArgs([]string{"dashboard", "panel", "delete", "traffic", "--yes"})
	if err := remove.Execute(); err != nil {
		t.Fatal(err)
	}
	if service.deleted != "traffic" {
		t.Fatalf("deleted = %q", service.deleted)
	}
}
