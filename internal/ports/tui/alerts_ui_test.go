package tui

import (
	"strings"
	"testing"
	"time"

	app "github.com/clcsolutions/nats-tui/internal/app/alerts"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
)

func TestAlertsViewIsResponsiveAndUsesNoBackgroundColor(t *testing.T) {
	t.Parallel()
	model := NewModel("local", "nats://localhost:4222", nil)
	model.alerts.records = []app.Record{{ID: "a", Severity: "critical", Category: "consumer", Title: "Max deliveries reached", ResourcePrimary: "ORDERS", ResourceSecondary: "WORKER", OccurredAt: time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC), Payload: []byte(`{"order":42}`)}}
	for _, width := range []int{72, 160} {
		view := model.alertsView(width, 24)
		if !strings.Contains(view, "Max deliveries") || !strings.Contains(view, "ORDERS/WORKER") {
			t.Fatalf("width %d view=%q", width, view)
		}
		if strings.Contains(view, "\x1b[48;") {
			t.Fatalf("width %d contains ANSI background", width)
		}
	}
}

func TestAlertDeepLinkSelectsAffectedStream(t *testing.T) {
	t.Parallel()
	model := NewModel("local", "nats://localhost:4222", nil)
	model.streams = []streams.Summary{{Name: "OTHER"}, {Name: "ORDERS"}}
	model.jumpFromAlert(app.Record{ResourceKind: "stream", ResourcePrimary: "ORDERS"})
	if model.activeWorkspace != 1 || model.streamCursor != 1 {
		t.Fatalf("workspace=%d cursor=%d", model.activeWorkspace, model.streamCursor)
	}
}
