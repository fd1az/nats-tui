package localstate

import (
	"strings"
	"testing"

	domain "github.com/fd1az/nats-tui/internal/domain/connection"
)

func TestProfileRepositoryRoundTripAndDelete(t *testing.T) {
	file := new(memoryStateFile)
	repository := NewProfileRepository(file)
	auth, _ := domain.UserPasswordAuthentication("ops", "secret")
	profile, _ := domain.NewProfile("prod", []string{"nats://a:4222", "nats://b:4222"}, "http://a:8222", auth)
	if err := repository.Save(t.Context(), profile); err != nil {
		t.Fatal(err)
	}
	got, err := repository.Find(t.Context(), "prod")
	if err != nil {
		t.Fatal(err)
	}
	if got.Authentication().Password() != "secret" || len(got.Servers()) != 2 {
		t.Fatalf("profile=%+v", got)
	}
	if !strings.Contains(string(file.data), "secret") {
		t.Fatal("test adapter did not persist secret")
	}
	if err := repository.Delete(t.Context(), "prod"); err != nil {
		t.Fatal(err)
	}
}
