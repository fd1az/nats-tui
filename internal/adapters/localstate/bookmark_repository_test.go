package localstate

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/fd1az/nats-tui/internal/app/bookmarks"
	domain "github.com/fd1az/nats-tui/internal/domain/bookmarks"
)

func TestBookmarkRepositoryRoundTripAndAtomicRetarget(t *testing.T) {
	t.Parallel()
	repository := NewBookmarkRepository(new(memoryStateFile))
	target, _ := domain.NewTarget(domain.KindConsumer, "ORDERS", "WORKER")
	bookmark, _ := domain.NewBookmark("b-1", target, "Worker", time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC))
	if err := repository.SaveBookmark(context.Background(), bookmark); err != nil {
		t.Fatal(err)
	}
	replacement, _ := domain.NewTarget(domain.KindConsumer, "ORDERS", "WORKER_V2")
	_ = bookmark.Retarget(replacement)
	if err := repository.SaveBookmark(context.Background(), bookmark); err != nil {
		t.Fatal(err)
	}
	values, err := repository.ListBookmarks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].Target().Secondary() != "WORKER_V2" {
		t.Fatalf("values=%v", values)
	}
	if err := repository.RemoveBookmark(context.Background(), bookmark.ID()); err != nil {
		t.Fatal(err)
	}
	if err := repository.RemoveBookmark(context.Background(), bookmark.ID()); !errors.Is(err, app.ErrBookmarkNotFound) {
		t.Fatalf("remove err=%v", err)
	}
}
func TestBookmarkRepositoryPersistsRecentSearches(t *testing.T) {
	t.Parallel()
	file := new(memoryStateFile)
	repository := NewBookmarkRepository(file)
	want := []app.RecentSearch{{Scope: "messages", Query: "order 42", UsedAt: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}}
	if err := repository.SaveSearches(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got, err := repository.ListSearches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Query != want[0].Query {
		t.Fatalf("got=%+v", got)
	}
}
