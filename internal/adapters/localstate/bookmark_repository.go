package localstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"time"

	app "github.com/fd1az/nats-tui/internal/app/bookmarks"
	domain "github.com/fd1az/nats-tui/internal/domain/bookmarks"
)

type BookmarkRepository struct {
	mu   sync.Mutex
	file StateFile
}

func NewBookmarkRepository(file StateFile) *BookmarkRepository {
	if file == nil {
		panic("nil bookmark state file")
	}
	return &BookmarkRepository{file: file}
}

type bookmarkState struct {
	Bookmarks []bookmarkDTO      `json:"bookmarks,omitempty"`
	Searches  []app.RecentSearch `json:"recent_searches,omitempty"`
}
type bookmarkDTO struct {
	ID        string      `json:"id"`
	Kind      domain.Kind `json:"kind"`
	Primary   string      `json:"primary"`
	Secondary string      `json:"secondary,omitempty"`
	Label     string      `json:"label,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

func (r *BookmarkRepository) SaveBookmark(_ context.Context, bookmark *domain.Bookmark) error {
	if bookmark == nil {
		return errors.New("bookmark is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return err
	}
	dto := bookmarkDTO{ID: bookmark.ID().String(), Kind: bookmark.Target().Kind(), Primary: bookmark.Target().Primary(), Secondary: bookmark.Target().Secondary(), Label: bookmark.Label(), CreatedAt: bookmark.CreatedAt()}
	for i, current := range state.Bookmarks {
		if current.ID == dto.ID {
			state.Bookmarks[i] = dto
			return r.save(state)
		}
		if current.Kind == dto.Kind && current.Primary == dto.Primary && current.Secondary == dto.Secondary {
			return app.ErrBookmarkTargetExists
		}
	}
	state.Bookmarks = append(state.Bookmarks, dto)
	return r.save(state)
}
func (r *BookmarkRepository) RemoveBookmark(_ context.Context, id domain.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return err
	}
	for i, current := range state.Bookmarks {
		if current.ID == id.String() {
			state.Bookmarks = append(state.Bookmarks[:i], state.Bookmarks[i+1:]...)
			return r.save(state)
		}
	}
	return app.ErrBookmarkNotFound
}
func (r *BookmarkRepository) ListBookmarks(context.Context) ([]*domain.Bookmark, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return nil, err
	}
	result := make([]*domain.Bookmark, 0, len(state.Bookmarks))
	for _, dto := range state.Bookmarks {
		target, mapErr := domain.NewTarget(dto.Kind, dto.Primary, dto.Secondary)
		if mapErr != nil {
			return nil, mapErr
		}
		bookmark, mapErr := domain.NewBookmark(dto.ID, target, dto.Label, dto.CreatedAt)
		if mapErr != nil {
			return nil, mapErr
		}
		result = append(result, bookmark)
	}
	return result, nil
}
func (r *BookmarkRepository) SaveSearches(_ context.Context, searches []app.RecentSearch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return err
	}
	state.Searches = append([]app.RecentSearch(nil), searches...)
	return r.save(state)
}
func (r *BookmarkRepository) ListSearches(context.Context) ([]app.RecentSearch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return nil, err
	}
	return append([]app.RecentSearch(nil), state.Searches...), nil
}
func (r *BookmarkRepository) load() (bookmarkState, error) {
	data, err := r.file.Read()
	if errors.Is(err, fs.ErrNotExist) {
		return bookmarkState{}, nil
	}
	if err != nil {
		return bookmarkState{}, fmt.Errorf("read bookmark state: %w", err)
	}
	var state bookmarkState
	if err := json.Unmarshal(data, &state); err != nil {
		return bookmarkState{}, fmt.Errorf("decode bookmark state: %w", err)
	}
	return state, nil
}
func (r *BookmarkRepository) save(state bookmarkState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode bookmark state: %w", err)
	}
	if err := r.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write bookmark state: %w", err)
	}
	return nil
}
