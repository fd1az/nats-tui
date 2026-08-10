package bookmarks

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	domain "github.com/clcsolutions/nats-tui/internal/domain/bookmarks"
)

var (
	ErrBookmarkNotFound     = errors.New("bookmark not found")
	ErrBookmarkTargetExists = errors.New("bookmark target already exists")
)

type Repository interface {
	SaveBookmark(context.Context, *domain.Bookmark) error
	RemoveBookmark(context.Context, domain.ID) error
	ListBookmarks(context.Context) ([]*domain.Bookmark, error)
	SaveSearches(context.Context, []RecentSearch) error
	ListSearches(context.Context) ([]RecentSearch, error)
}
type IDGenerator interface{ Next() string }
type Clock interface{ Now() time.Time }
type AddCommand struct{ Kind, Primary, Secondary, Label string }
type AddHandler struct {
	repository Repository
	ids        IDGenerator
	clock      Clock
}

func NewAddHandler(repository Repository, ids IDGenerator, clock Clock) AddHandler {
	if repository == nil || ids == nil || clock == nil {
		panic("bookmark add dependencies cannot be nil")
	}
	return AddHandler{repository, ids, clock}
}
func (h AddHandler) Handle(ctx context.Context, command AddCommand) (View, error) {
	kind, err := domain.ParseKind(command.Kind)
	if err != nil {
		return View{}, err
	}
	target, err := domain.NewTarget(kind, command.Primary, command.Secondary)
	if err != nil {
		return View{}, err
	}
	values, err := h.repository.ListBookmarks(ctx)
	if err != nil {
		return View{}, err
	}
	for _, value := range values {
		if value.Target().Key() == target.Key() {
			return view(value, true), nil
		}
	}
	bookmark, err := domain.NewBookmark(h.ids.Next(), target, command.Label, h.clock.Now())
	if err != nil {
		return View{}, err
	}
	if err := h.repository.SaveBookmark(ctx, bookmark); err != nil {
		return View{}, err
	}
	return view(bookmark, true), nil
}

type TargetView struct{ Kind, Primary, Secondary string }
type View struct {
	ID, Kind, Primary, Secondary, Label, Status string
	CreatedAt                                   time.Time
	Available                                   bool
}
type ListHandler struct{ repository Repository }

func NewListHandler(repository Repository) ListHandler {
	if repository == nil {
		panic("nil bookmark repository")
	}
	return ListHandler{repository}
}
func (h ListHandler) Handle(ctx context.Context, available []TargetView) ([]View, error) {
	values, err := h.repository.ListBookmarks(ctx)
	if err != nil {
		return nil, err
	}
	inventory := make(map[string]struct{}, len(available))
	parents := make(map[string]struct{})
	for _, item := range available {
		kind, parseErr := domain.ParseKind(item.Kind)
		if parseErr != nil {
			continue
		}
		target, targetErr := domain.NewTarget(kind, item.Primary, item.Secondary)
		if targetErr == nil {
			inventory[target.Key()] = struct{}{}
			if kind == domain.KindKVBucket || kind == domain.KindObjectBucket {
				parents[string(kind)+"\x00"+target.Primary()] = struct{}{}
			}
		}
	}
	result := make([]View, 0, len(values))
	for _, value := range values {
		if available == nil {
			item := view(value, false)
			item.Status = "unknown"
			result = append(result, item)
			continue
		}
		_, exists := inventory[value.Target().Key()]
		item := view(value, exists)
		if !exists {
			parentKind := domain.Kind("")
			if value.Target().Kind() == domain.KindKVKey {
				parentKind = domain.KindKVBucket
			} else if value.Target().Kind() == domain.KindObject {
				parentKind = domain.KindObjectBucket
			}
			if _, ok := parents[string(parentKind)+"\x00"+value.Target().Primary()]; ok {
				item.Available = true
				item.Status = "unchecked"
			}
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Available != result[j].Available {
			return result[i].Available
		}
		if result[i].Kind == result[j].Kind {
			return result[i].Label < result[j].Label
		}
		return result[i].Kind < result[j].Kind
	})
	return result, nil
}
func view(value *domain.Bookmark, available bool) View {
	status := "stale"
	if available {
		status = "available"
	}
	label := value.Label()
	if label == "" {
		label = value.Target().Primary()
		if value.Target().Secondary() != "" {
			label += " / " + value.Target().Secondary()
		}
	}
	return View{ID: value.ID().String(), Kind: string(value.Target().Kind()), Primary: value.Target().Primary(), Secondary: value.Target().Secondary(), Label: label, Status: status, CreatedAt: value.CreatedAt(), Available: available}
}

type RemoveHandler struct{ repository Repository }

func NewRemoveHandler(repository Repository) RemoveHandler {
	if repository == nil {
		panic("nil bookmark repository")
	}
	return RemoveHandler{repository}
}
func (h RemoveHandler) Handle(ctx context.Context, value string) error {
	id, err := domain.NewID(value)
	if err != nil {
		return err
	}
	return h.repository.RemoveBookmark(ctx, id)
}

type RetargetHandler struct{ repository Repository }

func NewRetargetHandler(repository Repository) RetargetHandler {
	if repository == nil {
		panic("nil bookmark repository")
	}
	return RetargetHandler{repository}
}
func (h RetargetHandler) Handle(ctx context.Context, idValue, primary, secondary string) error {
	id, err := domain.NewID(idValue)
	if err != nil {
		return err
	}
	values, err := h.repository.ListBookmarks(ctx)
	if err != nil {
		return err
	}
	for _, bookmark := range values {
		if bookmark.ID() != id {
			continue
		}
		target, err := domain.NewTarget(bookmark.Target().Kind(), primary, secondary)
		if err != nil {
			return err
		}
		if err := bookmark.Retarget(target); err != nil {
			return err
		}
		return h.repository.SaveBookmark(ctx, bookmark)
	}
	return ErrBookmarkNotFound
}

type RecentSearch struct {
	Scope  string    `json:"scope"`
	Query  string    `json:"query"`
	UsedAt time.Time `json:"used_at"`
}
type SaveSearchHandler struct {
	repository Repository
	clock      Clock
	limit      int
}

func NewSaveSearchHandler(repository Repository, clock Clock, limit int) SaveSearchHandler {
	if repository == nil || clock == nil || limit < 1 {
		panic("recent search dependencies are invalid")
	}
	return SaveSearchHandler{repository, clock, limit}
}
func (h SaveSearchHandler) Handle(ctx context.Context, scope, query string) error {
	scope, query = strings.TrimSpace(scope), strings.TrimSpace(query)
	if scope == "" || query == "" {
		return errors.New("search scope and query are required")
	}
	values, err := h.repository.ListSearches(ctx)
	if err != nil {
		return err
	}
	result := []RecentSearch{{Scope: scope, Query: query, UsedAt: h.clock.Now()}}
	for _, value := range values {
		if value.Scope == scope && strings.EqualFold(value.Query, query) {
			continue
		}
		result = append(result, value)
	}
	counts := map[string]int{}
	filtered := result[:0]
	for _, value := range result {
		if counts[value.Scope] >= h.limit {
			continue
		}
		counts[value.Scope]++
		filtered = append(filtered, value)
	}
	return h.repository.SaveSearches(ctx, filtered)
}

type ListSearchesHandler struct{ repository Repository }

func NewListSearchesHandler(repository Repository) ListSearchesHandler {
	if repository == nil {
		panic("nil recent search repository")
	}
	return ListSearchesHandler{repository}
}
func (h ListSearchesHandler) Handle(ctx context.Context, scope string, limit int) ([]RecentSearch, error) {
	if limit < 0 {
		return nil, errors.New("search limit cannot be negative")
	}
	values, err := h.repository.ListSearches(ctx)
	if err != nil {
		return nil, err
	}
	scope = strings.TrimSpace(scope)
	result := make([]RecentSearch, 0, len(values))
	for _, value := range values {
		if scope == "" || value.Scope == scope {
			result = append(result, value)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].UsedAt.After(result[j].UsedAt) })
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
