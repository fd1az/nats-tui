package localstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"sync"

	"github.com/clcsolutions/nats-tui/internal/app/dashboard"
)

type DashboardRepository struct {
	mu   sync.Mutex
	file StateFile
}

func NewDashboardRepository(file StateFile) *DashboardRepository {
	if file == nil {
		panic("nil dashboard state file")
	}
	return &DashboardRepository{file: file}
}

type dashboardState struct {
	Panels []dashboard.Panel `json:"panels,omitempty"`
}

func (r *DashboardRepository) ListPanels(context.Context) ([]dashboard.Panel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return nil, err
	}
	result := append([]dashboard.Panel(nil), state.Panels...)
	sort.SliceStable(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}
func (r *DashboardRepository) SavePanel(_ context.Context, panel dashboard.Panel) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return err
	}
	for i := range state.Panels {
		if state.Panels[i].ID == panel.ID {
			state.Panels[i] = panel
			return r.save(state)
		}
	}
	state.Panels = append(state.Panels, panel)
	return r.save(state)
}
func (r *DashboardRepository) DeletePanel(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return err
	}
	for i := range state.Panels {
		if state.Panels[i].ID == id {
			state.Panels = append(state.Panels[:i], state.Panels[i+1:]...)
			return r.save(state)
		}
	}
	return errors.New("dashboard panel not found")
}
func (r *DashboardRepository) load() (dashboardState, error) {
	data, err := r.file.Read()
	if errors.Is(err, fs.ErrNotExist) {
		return dashboardState{}, nil
	}
	if err != nil {
		return dashboardState{}, fmt.Errorf("read dashboard state: %w", err)
	}
	var state dashboardState
	if err := json.Unmarshal(data, &state); err != nil {
		return dashboardState{}, fmt.Errorf("decode dashboard state: %w", err)
	}
	for i, panel := range state.Panels {
		validated, validationErr := dashboard.NewPanel(panel.ID, panel.Title, panel.Metric)
		if validationErr != nil {
			return dashboardState{}, fmt.Errorf("decode dashboard panel %d: %w", i, validationErr)
		}
		state.Panels[i] = validated
	}
	return state, nil
}
func (r *DashboardRepository) save(state dashboardState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode dashboard state: %w", err)
	}
	if err := r.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write dashboard state: %w", err)
	}
	return nil
}
