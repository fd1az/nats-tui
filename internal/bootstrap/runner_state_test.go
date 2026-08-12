package bootstrap

import (
	"context"
	"testing"
	"time"

	appbookmarks "github.com/fd1az/nats-tui/internal/app/bookmarks"
	"github.com/fd1az/nats-tui/internal/config"
	domain "github.com/fd1az/nats-tui/internal/domain/requestreply"
)

func TestRequestStateIsIsolatedByProfile(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("HOME", configRoot)
	t.Setenv("XDG_CONFIG_HOME", configRoot)

	message, err := domain.NewMessage("rpc.lookup", []byte(`{"id":42}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	request, err := domain.NewRequest(message, time.Second, 1)
	if err != nil {
		t.Fatal(err)
	}
	template, err := domain.NewTemplate("lookup", request)
	if err != nil {
		t.Fatal(err)
	}

	runner := NewRunner()
	ctx := context.Background()
	if err := runner.SaveRequestTemplate(ctx, config.Settings{Profile: "production/eu"}, template); err != nil {
		t.Fatal(err)
	}
	production, err := runner.ListRequestTemplates(ctx, config.Settings{Profile: "production/eu"})
	if err != nil {
		t.Fatal(err)
	}
	staging, err := runner.ListRequestTemplates(ctx, config.Settings{Profile: "staging"})
	if err != nil {
		t.Fatal(err)
	}
	if len(production) != 1 || production[0].Name() != "lookup" {
		t.Fatalf("production templates=%+v", production)
	}
	if len(staging) != 0 {
		t.Fatalf("staging templates=%+v", staging)
	}
}

func TestBookmarksAreIsolatedByProfile(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("HOME", configRoot)
	t.Setenv("XDG_CONFIG_HOME", configRoot)

	runner := NewRunner()
	ctx := context.Background()
	production := config.Settings{Profile: "production"}
	if _, err := runner.AddBookmark(ctx, production, appbookmarks.AddCommand{Kind: "stream", Primary: "ORDERS"}); err != nil {
		t.Fatal(err)
	}
	productionItems, err := runner.ListBookmarks(ctx, production, nil)
	if err != nil {
		t.Fatal(err)
	}
	stagingItems, err := runner.ListBookmarks(ctx, config.Settings{Profile: "staging"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(productionItems) != 1 || productionItems[0].Primary != "ORDERS" {
		t.Fatalf("production bookmarks=%+v", productionItems)
	}
	if len(stagingItems) != 0 {
		t.Fatalf("staging bookmarks=%+v", stagingItems)
	}
}

func TestLocalStateProfileIsSafeAndStable(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"":            "local",
		"local_dev-1": "local_dev-1",
	}
	for input, want := range tests {
		if got := localStateProfile(input); got != want {
			t.Errorf("localStateProfile(%q)=%q, want %q", input, got, want)
		}
	}
	unsafe := localStateProfile("production/eu")
	if unsafe == "production-eu" || unsafe == localStateProfile("production-eu") {
		t.Fatalf("unsafe profile collided after sanitization: %q", unsafe)
	}
	if unsafe != localStateProfile("production/eu") {
		t.Fatalf("profile sanitization is not stable: %q", unsafe)
	}
}
