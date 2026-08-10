package localstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"time"

	app "github.com/clcsolutions/nats-tui/internal/app/commandbar"
	domain "github.com/clcsolutions/nats-tui/internal/domain/commandbar"
)

type CommandBarRepository struct {
	mu         sync.Mutex
	file       StateFile
	maxHistory int
}

func NewCommandBarRepository(file StateFile, maxHistory int) *CommandBarRepository {
	if file == nil {
		panic("nil command bar state file")
	}
	if maxHistory < 1 {
		panic("command history limit must be positive")
	}
	return &CommandBarRepository{file: file, maxHistory: maxHistory}
}

type commandBarState struct {
	Definitions map[string]definitionDTO `json:"definitions,omitempty"`
	History     []historyDTO             `json:"history,omitempty"`
}
type definitionDTO struct {
	ID, Title, Description, Group, Action string
	Risk                                  domain.Risk
	Parameters                            []domain.ParameterInput
	Arguments                             map[string]string
	Equivalent                            []string
	Custom                                bool
}
type historyDTO struct {
	ID, CommandID, Title, Status, Error string
	StartedAt, CompletedAt              time.Time
}

func (r *CommandBarRepository) SaveDefinition(_ context.Context, definition domain.Definition) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return err
	}
	input := definition.Input()
	state.Definitions[input.ID] = definitionDTO{ID: input.ID, Title: input.Title, Description: input.Description, Group: input.Group, Action: input.Action, Risk: input.Risk, Parameters: input.Parameters, Arguments: input.Arguments, Equivalent: input.Equivalent, Custom: input.Custom}
	return r.save(state)
}
func (r *CommandBarRepository) RemoveDefinition(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return err
	}
	if _, ok := state.Definitions[id]; !ok {
		return app.ErrDefinitionNotFound
	}
	delete(state.Definitions, id)
	return r.save(state)
}
func (r *CommandBarRepository) ListDefinitions(context.Context) ([]domain.Definition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return nil, err
	}
	result := make([]domain.Definition, 0, len(state.Definitions))
	for _, dto := range state.Definitions {
		definition, mapErr := domain.NewDefinition(domain.DefinitionInput{ID: dto.ID, Title: dto.Title, Description: dto.Description, Group: dto.Group, Action: dto.Action, Risk: dto.Risk, Parameters: dto.Parameters, Arguments: dto.Arguments, Equivalent: dto.Equivalent, Custom: dto.Custom})
		if mapErr != nil {
			return nil, mapErr
		}
		result = append(result, definition)
	}
	return result, nil
}
func (r *CommandBarRepository) AppendHistory(_ context.Context, entry *domain.HistoryEntry) error {
	if entry == nil {
		return errors.New("command history entry is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return err
	}
	state.History = append(state.History, historyDTO{ID: entry.ID(), CommandID: entry.CommandID(), Title: entry.Title(), Status: string(entry.Status()), Error: entry.Error(), StartedAt: entry.StartedAt(), CompletedAt: entry.CompletedAt()})
	if overflow := len(state.History) - r.maxHistory; overflow > 0 {
		state.History = append([]historyDTO(nil), state.History[overflow:]...)
	}
	return r.save(state)
}
func (r *CommandBarRepository) ListHistory(context.Context) ([]*domain.HistoryEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return nil, err
	}
	result := make([]*domain.HistoryEntry, 0, len(state.History))
	for _, dto := range state.History {
		entry, mapErr := domain.RehydrateHistoryEntry(dto.ID, dto.CommandID, dto.Title, dto.Error, dto.StartedAt, dto.CompletedAt, domain.Status(dto.Status))
		if mapErr != nil {
			return nil, mapErr
		}
		result = append(result, entry)
	}
	return result, nil
}
func (r *CommandBarRepository) load() (commandBarState, error) {
	data, err := r.file.Read()
	if errors.Is(err, fs.ErrNotExist) {
		return commandBarState{Definitions: map[string]definitionDTO{}}, nil
	}
	if err != nil {
		return commandBarState{}, fmt.Errorf("read command bar state: %w", err)
	}
	var state commandBarState
	if err := json.Unmarshal(data, &state); err != nil {
		return commandBarState{}, fmt.Errorf("decode command bar state: %w", err)
	}
	if state.Definitions == nil {
		state.Definitions = map[string]definitionDTO{}
	}
	return state, nil
}
func (r *CommandBarRepository) save(state commandBarState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode command bar state: %w", err)
	}
	if err := r.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write command bar state: %w", err)
	}
	return nil
}
