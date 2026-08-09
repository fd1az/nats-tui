package connection

import "testing"

func TestNewProfileRejectsMissingServerURL(t *testing.T) {
	t.Parallel()

	_, err := NewProfile("local", "")

	if err == nil {
		t.Fatal("NewProfile() error = nil, want an error")
	}
}

func TestNewProfileUsesServerAsNameWhenNameIsMissing(t *testing.T) {
	t.Parallel()

	profile, err := NewProfile("", "nats://127.0.0.1:4222")
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}

	if profile.Name() != "127.0.0.1:4222" {
		t.Fatalf("Name() = %q, want %q", profile.Name(), "127.0.0.1:4222")
	}
}
