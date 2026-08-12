package activity

import (
	"testing"
	"time"
)

func TestLogBoundsEventsAndReturnsCopies(t *testing.T) {
	t.Parallel()
	log := NewLog(2)
	for _, title := range []string{"one", "two", "three"} {
		if _, err := log.Add(Info, title, "", time.Time{}); err != nil {
			t.Fatal(err)
		}
	}
	events := log.Events()
	if len(events) != 2 || events[0].Title != "two" || events[1].ID != 3 {
		t.Fatalf("events = %#v", events)
	}
	events[0].Title = "changed"
	if log.Events()[0].Title != "two" {
		t.Fatal("Events leaked internal storage")
	}
}

func TestLogRejectsInvalidEvents(t *testing.T) {
	t.Parallel()
	log := NewLog(1)
	if _, err := log.Add(Info, " ", "", time.Time{}); err == nil {
		t.Fatal("expected empty title error")
	}
	if _, err := log.Add(Level("bad"), "title", "", time.Time{}); err == nil {
		t.Fatal("expected level error")
	}
}
