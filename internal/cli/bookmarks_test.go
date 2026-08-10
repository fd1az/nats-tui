package cli

import (
	"bytes"
	"context"
	app "github.com/clcsolutions/nats-tui/internal/app/bookmarks"
	"github.com/clcsolutions/nats-tui/internal/config"
	"testing"
)

type stubBookmarks struct {
	added   app.AddCommand
	deleted string
}

func (s *stubBookmarks) AddBookmark(_ context.Context, _ config.Settings, c app.AddCommand) (app.View, error) {
	s.added = c
	return app.View{ID: "b1", Kind: c.Kind, Primary: c.Primary, Secondary: c.Secondary, Status: "available"}, nil
}
func (s *stubBookmarks) ListBookmarks(context.Context, config.Settings, []app.TargetView) ([]app.View, error) {
	return []app.View{{ID: "b1", Kind: "consumer", Primary: "ORDERS", Secondary: "WORKER", Label: "Worker", Status: "unknown"}}, nil
}
func (s *stubBookmarks) DeleteBookmark(_ context.Context, _ config.Settings, id string) error {
	s.deleted = id
	return nil
}
func (s *stubBookmarks) RetargetBookmark(context.Context, config.Settings, string, string, string) error {
	return nil
}
func (s *stubBookmarks) SaveRecentSearch(context.Context, config.Settings, string, string) error {
	return nil
}
func (s *stubBookmarks) ListRecentSearches(context.Context, config.Settings, string, int) ([]app.RecentSearch, error) {
	return nil, nil
}
func TestBookmarksCLIAddsTypedTargetAndRequiresDeleteConfirmation(t *testing.T) {
	t.Parallel()
	service := new(stubBookmarks)
	root := NewRoot(Dependencies{Bookmarks: service})
	root.SetArgs([]string{"bookmarks", "add", "consumer", "ORDERS", "WORKER", "--label", "Payments worker"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.added.Primary != "ORDERS" || service.added.Secondary != "WORKER" {
		t.Fatalf("added=%+v", service.added)
	}
	unsafe := NewRoot(Dependencies{Bookmarks: service})
	unsafe.SetArgs([]string{"bookmarks", "delete", "b1"})
	if err := unsafe.ExecuteContext(context.Background()); err == nil {
		t.Fatal("unsafe delete succeeded")
	}
	safe := NewRoot(Dependencies{Bookmarks: service})
	safe.SetArgs([]string{"bookmarks", "delete", "b1", "--yes"})
	if err := safe.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.deleted != "b1" {
		t.Fatalf("deleted=%q", service.deleted)
	}
	output := new(bytes.Buffer)
	list := NewRoot(Dependencies{Bookmarks: service})
	list.SetOut(output)
	list.SetArgs([]string{"bookmarks"})
	if err := list.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte("ORDERS/WORKER")) {
		t.Fatalf("output=%s", output)
	}
}
