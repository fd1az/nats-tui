package connection

import (
	"errors"
	"testing"
	"time"
)

func TestProfilePreservesUniqueFailoverServers(t *testing.T) {
	t.Parallel()
	profile, err := NewProfile("edge", []string{"nats://a:4222,nats://b:4222", "nats://a:4222"}, "", NoAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	if got := profile.Server(); got != "nats://a:4222,nats://b:4222" {
		t.Fatalf("Server()=%q", got)
	}
}

func TestProfileRejectsInvalidServer(t *testing.T) {
	t.Parallel()
	_, err := NewProfile("edge", []string{"http://not-nats"}, "", NoAuthentication())
	if !errors.Is(err, ErrInvalidServer) {
		t.Fatalf("error=%v", err)
	}
}

func TestDirectProfileDerivesName(t *testing.T) {
	t.Parallel()
	profile, err := NewDirectProfile("", "nats://127.0.0.1:4222")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Name() != "127.0.0.1:4222" {
		t.Fatalf("Name()=%q", profile.Name())
	}
}

func TestSortByRecent(t *testing.T) {
	t.Parallel()
	a, _ := RestoreProfile("a", []string{"nats://a:4222"}, "", NoAuthentication(), time.Unix(1, 0))
	b, _ := RestoreProfile("b", []string{"nats://b:4222"}, "", NoAuthentication(), time.Unix(2, 0))
	items := []Profile{a, b}
	SortByRecent(items)
	if items[0].Name() != "b" {
		t.Fatalf("items=%v", items)
	}
}
