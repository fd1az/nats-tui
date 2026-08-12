package localstate

import (
	"context"
	"testing"

	"github.com/fd1az/nats-tui/internal/app/dashboard"
)

func TestDashboardRepositoryPersistsAndReplacesPanels(t *testing.T) {
	t.Parallel()
	file := &memoryStateFile{}
	repository := NewDashboardRepository(file)
	service := dashboard.NewPanelService(repository)
	if _, err := service.Save(context.Background(), "traffic", "Traffic", dashboard.MetricMessageRate); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Save(context.Background(), "traffic", "CPU", dashboard.MetricCPU); err != nil {
		t.Fatal(err)
	}
	panels, err := repository.ListPanels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(panels) != 1 || panels[0].Metric != dashboard.MetricCPU {
		t.Fatalf("panels = %#v", panels)
	}
	if err := repository.DeletePanel(context.Background(), "traffic"); err != nil {
		t.Fatal(err)
	}
	panels, _ = repository.ListPanels(context.Background())
	if len(panels) != 0 {
		t.Fatalf("panels after delete = %#v", panels)
	}
}
