package bookmarks

import (
	"context"
	"testing"
	"time"

	domain "github.com/clcsolutions/nats-tui/internal/domain/bookmarks"
)

type memoryRepository struct {
	bookmarks []*domain.Bookmark
	searches  []RecentSearch
}

func (r *memoryRepository) SaveBookmark(_ context.Context, b *domain.Bookmark) error {
	for i, current := range r.bookmarks {
		if current.ID() == b.ID() {
			r.bookmarks[i] = b
			return nil
		}
	}
	r.bookmarks = append(r.bookmarks, b)
	return nil
}
func (r *memoryRepository) RemoveBookmark(_ context.Context, id domain.ID) error {
	for i, b := range r.bookmarks {
		if b.ID() == id {
			r.bookmarks = append(r.bookmarks[:i], r.bookmarks[i+1:]...)
			return nil
		}
	}
	return ErrBookmarkNotFound
}
func (r *memoryRepository) ListBookmarks(context.Context) ([]*domain.Bookmark, error) {
	return append([]*domain.Bookmark(nil), r.bookmarks...), nil
}
func (r *memoryRepository) SaveSearches(_ context.Context, s []RecentSearch) error {
	r.searches = append([]RecentSearch(nil), s...)
	return nil
}
func (r *memoryRepository) ListSearches(context.Context) ([]RecentSearch, error) {
	return append([]RecentSearch(nil), r.searches...), nil
}

type fixedID string

func (id fixedID) Next() string { return string(id) }

type fixedClock time.Time

func (c fixedClock) Now() time.Time { return time.Time(c) }
func TestAddDeduplicatesTargetAndResolutionMarksMissing(t *testing.T) {
	t.Parallel()
	repository := new(memoryRepository)
	handler := NewAddHandler(repository, fixedID("b-1"), fixedClock(time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)))
	if _, err := handler.Handle(context.Background(), AddCommand{Kind: "stream", Primary: "ORDERS", Label: "Orders"}); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.Handle(context.Background(), AddCommand{Kind: "stream", Primary: "ORDERS"}); err != nil {
		t.Fatal(err)
	}
	if len(repository.bookmarks) != 1 {
		t.Fatalf("bookmarks=%d", len(repository.bookmarks))
	}
	views, err := NewListHandler(repository).Handle(context.Background(), []TargetView{{Kind: "stream", Primary: "EVENTS"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Available || views[0].Status != "stale" {
		t.Fatalf("views=%+v", views)
	}
}
func TestRecentSearchesDeduplicateAndBoundPerScope(t *testing.T) {
	t.Parallel()
	repository := new(memoryRepository)
	handler := NewSaveSearchHandler(repository, fixedClock(time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)), 2)
	for _, query := range []string{"orders", "payments", "orders"} {
		if err := handler.Handle(context.Background(), "messages", query); err != nil {
			t.Fatal(err)
		}
	}
	values, err := NewListSearchesHandler(repository).Handle(context.Background(), "messages", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0].Query != "orders" {
		t.Fatalf("values=%+v", values)
	}
}
