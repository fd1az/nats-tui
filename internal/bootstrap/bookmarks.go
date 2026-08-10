package bootstrap

import (
	"context"

	app "github.com/clcsolutions/nats-tui/internal/app/bookmarks"
	"github.com/clcsolutions/nats-tui/internal/config"
)

func (r Runner) bookmarkStore(profile string) app.Repository {
	if r.bookmarkRepository != nil {
		return r.bookmarkRepository
	}
	profile = localStateProfile(profile)
	if r.bookmarkStateMu == nil || r.bookmarkStates == nil {
		return defaultBookmarkRepository(profile)
	}
	r.bookmarkStateMu.Lock()
	defer r.bookmarkStateMu.Unlock()
	store, ok := r.bookmarkStates[profile]
	if !ok {
		store = defaultBookmarkRepository(profile)
		r.bookmarkStates[profile] = store
	}
	return store
}
func (r Runner) AddBookmark(ctx context.Context, settings config.Settings, command app.AddCommand) (app.View, error) {
	return app.NewAddHandler(r.bookmarkStore(settings.Profile), r.ids, r.clock).Handle(ctx, command)
}
func (r Runner) ListBookmarks(ctx context.Context, settings config.Settings, available []app.TargetView) ([]app.View, error) {
	return app.NewListHandler(r.bookmarkStore(settings.Profile)).Handle(ctx, available)
}
func (r Runner) DeleteBookmark(ctx context.Context, settings config.Settings, id string) error {
	return app.NewRemoveHandler(r.bookmarkStore(settings.Profile)).Handle(ctx, id)
}
func (r Runner) RetargetBookmark(ctx context.Context, settings config.Settings, id, primary, secondary string) error {
	return app.NewRetargetHandler(r.bookmarkStore(settings.Profile)).Handle(ctx, id, primary, secondary)
}
func (r Runner) SaveRecentSearch(ctx context.Context, settings config.Settings, scope, query string) error {
	return app.NewSaveSearchHandler(r.bookmarkStore(settings.Profile), r.clock, 20).Handle(ctx, scope, query)
}
func (r Runner) ListRecentSearches(ctx context.Context, settings config.Settings, scope string, limit int) ([]app.RecentSearch, error) {
	return app.NewListSearchesHandler(r.bookmarkStore(settings.Profile)).Handle(ctx, scope, limit)
}
