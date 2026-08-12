package dashboard

import (
	"context"
	"errors"
	"sort"
	"strings"
)

type Metric string

const (
	MetricMessageRate   Metric = "message-rate"
	MetricByteRate      Metric = "byte-rate"
	MetricStorage       Metric = "storage"
	MetricMemoryStorage Metric = "memory-storage"
	MetricFileStorage   Metric = "file-storage"
	MetricStreams       Metric = "streams"
	MetricConsumers     Metric = "consumers"
	MetricCPU           Metric = "cpu"
	MetricServerMemory  Metric = "server-memory"
	MetricConnections   Metric = "connections"
	MetricSlowConsumers Metric = "slow-consumers"
)

var validMetrics = map[Metric]struct{}{
	MetricMessageRate: {}, MetricByteRate: {}, MetricStorage: {}, MetricMemoryStorage: {},
	MetricFileStorage: {}, MetricStreams: {}, MetricConsumers: {}, MetricCPU: {},
	MetricServerMemory: {}, MetricConnections: {}, MetricSlowConsumers: {},
}

type Panel struct {
	ID, Title string
	Metric    Metric
}

func NewPanel(id, title string, metric Metric) (Panel, error) {
	id, title = strings.TrimSpace(id), strings.TrimSpace(title)
	if id == "" || strings.ContainsAny(id, " \t\r\n") {
		return Panel{}, errors.New("dashboard panel ID must be a non-empty word")
	}
	if _, ok := validMetrics[metric]; !ok {
		return Panel{}, errors.New("unsupported dashboard metric")
	}
	if title == "" {
		title = strings.ReplaceAll(strings.ToUpper(string(metric)), "-", " ")
	}
	return Panel{ID: id, Title: title, Metric: metric}, nil
}

func Metrics() []Metric {
	result := make([]Metric, 0, len(validMetrics))
	for metric := range validMetrics {
		result = append(result, metric)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

type PanelRepository interface {
	ListPanels(context.Context) ([]Panel, error)
	SavePanel(context.Context, Panel) error
	DeletePanel(context.Context, string) error
}

type PanelService struct{ repository PanelRepository }

func NewPanelService(repository PanelRepository) PanelService {
	if repository == nil {
		panic("nil dashboard panel repository")
	}
	return PanelService{repository: repository}
}
func (s PanelService) List(ctx context.Context) ([]Panel, error) { return s.repository.ListPanels(ctx) }
func (s PanelService) Save(ctx context.Context, id, title string, metric Metric) (Panel, error) {
	panel, err := NewPanel(id, title, metric)
	if err != nil {
		return Panel{}, err
	}
	return panel, s.repository.SavePanel(ctx, panel)
}
func (s PanelService) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("dashboard panel ID is required")
	}
	return s.repository.DeletePanel(ctx, id)
}
