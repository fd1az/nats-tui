package commandbar

import (
	"context"
	"errors"
	"sort"
	"time"

	domain "github.com/clcsolutions/nats-tui/internal/domain/commandbar"
)

var ErrDefinitionNotFound = errors.New("custom command not found")

type Repository interface {
	SaveDefinition(context.Context, domain.Definition) error
	RemoveDefinition(context.Context, string) error
	ListDefinitions(context.Context) ([]domain.Definition, error)
	AppendHistory(context.Context, *domain.HistoryEntry) error
	ListHistory(context.Context) ([]*domain.HistoryEntry, error)
}
type IDGenerator interface{ Next() string }

type HistoryView struct {
	ID        string        `json:"id"`
	CommandID string        `json:"command_id"`
	Title     string        `json:"title"`
	Status    string        `json:"status"`
	Error     string        `json:"error,omitempty"`
	StartedAt time.Time     `json:"started_at"`
	Duration  time.Duration `json:"duration"`
}
type RecordExecutionHandler struct {
	repository Repository
	ids        IDGenerator
}

func NewRecordExecutionHandler(repository Repository, ids IDGenerator) RecordExecutionHandler {
	if repository == nil || ids == nil {
		panic("command history dependencies cannot be nil")
	}
	return RecordExecutionHandler{repository, ids}
}
func (h RecordExecutionHandler) Handle(ctx context.Context, commandID, title string, startedAt, completedAt time.Time, executionErr error) error {
	entry, err := domain.NewHistoryEntry(h.ids.Next(), commandID, title, startedAt)
	if err != nil {
		return err
	}
	message := ""
	if executionErr != nil {
		message = executionErr.Error()
	}
	if err := entry.Complete(completedAt, message); err != nil {
		return err
	}
	return h.repository.AppendHistory(ctx, entry)
}

type ListHistoryHandler struct{ repository Repository }

func NewListHistoryHandler(repository Repository) ListHistoryHandler {
	if repository == nil {
		panic("nil command history repository")
	}
	return ListHistoryHandler{repository}
}
func (h ListHistoryHandler) Handle(ctx context.Context, limit int) ([]HistoryView, error) {
	if limit < 0 {
		return nil, errors.New("command history limit cannot be negative")
	}
	entries, err := h.repository.ListHistory(ctx)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].StartedAt().After(entries[j].StartedAt()) })
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	result := make([]HistoryView, len(entries))
	for i, entry := range entries {
		result[i] = HistoryView{ID: entry.ID(), CommandID: entry.CommandID(), Title: entry.Title(), Status: string(entry.Status()), Error: entry.Error(), StartedAt: entry.StartedAt(), Duration: entry.Duration()}
	}
	return result, nil
}

type SaveDefinitionHandler struct{ repository Repository }

func NewSaveDefinitionHandler(repository Repository) SaveDefinitionHandler {
	if repository == nil {
		panic("nil command definition repository")
	}
	return SaveDefinitionHandler{repository}
}
func (h SaveDefinitionHandler) Handle(ctx context.Context, input domain.DefinitionInput) error {
	input.Custom = true
	definition, err := domain.NewDefinition(input)
	if err != nil {
		return err
	}
	return h.repository.SaveDefinition(ctx, definition)
}

type ListDefinitionsHandler struct{ repository Repository }

func NewListDefinitionsHandler(repository Repository) ListDefinitionsHandler {
	if repository == nil {
		panic("nil command definition repository")
	}
	return ListDefinitionsHandler{repository}
}
func (h ListDefinitionsHandler) Handle(ctx context.Context) ([]Entry, error) {
	values, err := h.repository.ListDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Entry, len(values))
	for i, value := range values {
		result[i] = EntryFrom(value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Title < result[j].Title })
	return result, nil
}

type RemoveDefinitionHandler struct{ repository Repository }

func NewRemoveDefinitionHandler(repository Repository) RemoveDefinitionHandler {
	if repository == nil {
		panic("nil command definition repository")
	}
	return RemoveDefinitionHandler{repository}
}
func (h RemoveDefinitionHandler) Handle(ctx context.Context, id string) error {
	return h.repository.RemoveDefinition(ctx, id)
}
