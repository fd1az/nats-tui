package preferences

import "testing"

func TestNewDefaultsAndValidatesBindings(t *testing.T) {
	t.Parallel()
	got, err := New(true, "", Bindings{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Mouse || got.Theme != ThemeMidnight || got.Keys.Activity != "ctrl+g" {
		t.Fatalf("preferences = %#v", got)
	}
	keys := DefaultBindings()
	keys.Help = keys.Quit
	if _, err := New(false, "midnight", keys); err == nil {
		t.Fatal("expected duplicate binding error")
	}
	if _, err := New(false, "neon", Bindings{}); err == nil {
		t.Fatal("expected theme error")
	}
}
