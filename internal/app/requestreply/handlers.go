// Package requestreply contains application use cases for Core NATS publish and request/reply.
package requestreply

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	domain "github.com/clcsolutions/nats-tui/internal/domain/requestreply"
)

var (
	ErrExchangeNotFound = errors.New("request exchange not found")
	ErrTemplateNotFound = errors.New("request template not found")
	ErrNoResponders     = errors.New("no responders available for subject")
)

type Publisher interface {
	Publish(context.Context, domain.Message) error
}

type Requester interface {
	Request(context.Context, domain.Request) ([]domain.Reply, error)
}

type HistoryRepository interface {
	Append(context.Context, domain.Exchange) error
	List(context.Context) ([]domain.Exchange, error)
	FindExchange(context.Context, string) (domain.Exchange, error)
}

type TemplateRepository interface {
	Save(context.Context, domain.Template) error
	ListTemplates(context.Context) ([]domain.Template, error)
	FindTemplate(context.Context, string) (domain.Template, error)
	Delete(context.Context, string) error
}

type Clock interface{ Now() time.Time }
type IDGenerator interface{ Next() string }
type TimeoutFactory func(context.Context, time.Duration) (context.Context, context.CancelFunc)

type PublishHandler struct{ publisher Publisher }

func NewPublishHandler(publisher Publisher) PublishHandler {
	if publisher == nil {
		panic("nil Core NATS publisher")
	}
	return PublishHandler{publisher: publisher}
}

func (h PublishHandler) Handle(ctx context.Context, message domain.Message) error {
	if message.IsZero() {
		return errors.New("publish message is required")
	}
	return h.publisher.Publish(ctx, message)
}

type SendRequestHandler struct {
	requester Requester
	history   HistoryRepository
	clock     Clock
	ids       IDGenerator
	timeout   TimeoutFactory
}

func NewSendRequestHandler(requester Requester, history HistoryRepository, clock Clock, ids IDGenerator, timeout TimeoutFactory) SendRequestHandler {
	if requester == nil || history == nil || clock == nil || ids == nil || timeout == nil {
		panic("request handler dependencies cannot be nil")
	}
	return SendRequestHandler{requester: requester, history: history, clock: clock, ids: ids, timeout: timeout}
}

func (h SendRequestHandler) Handle(ctx context.Context, request domain.Request) (domain.Exchange, error) {
	startedAt := h.clock.Now()
	requestCtx, cancel := h.timeout(ctx, request.Timeout())
	defer cancel()
	replies, err := h.requester.Request(requestCtx, request)
	if err != nil {
		return domain.Exchange{}, err
	}
	exchange, err := domain.NewExchange(h.ids.Next(), startedAt, request, replies)
	if err != nil {
		return domain.Exchange{}, err
	}
	if err := h.history.Append(ctx, exchange); err != nil {
		return domain.Exchange{}, err
	}
	return exchange, nil
}

type ListHistoryHandler struct{ history HistoryRepository }

func NewListHistoryHandler(history HistoryRepository) ListHistoryHandler {
	if history == nil {
		panic("nil request history")
	}
	return ListHistoryHandler{history: history}
}

func (h ListHistoryHandler) Handle(ctx context.Context) ([]domain.Exchange, error) {
	exchanges, err := h.history.List(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(exchanges, func(i, j int) bool { return exchanges[i].StartedAt().After(exchanges[j].StartedAt()) })
	return exchanges, nil
}

type ReplayHandler struct {
	history  HistoryRepository
	requests SendRequestHandler
}

func NewReplayHandler(history HistoryRepository, requests SendRequestHandler) ReplayHandler {
	if history == nil {
		panic("nil request history")
	}
	return ReplayHandler{history: history, requests: requests}
}

func (h ReplayHandler) Handle(ctx context.Context, id string) (domain.Exchange, error) {
	exchange, err := h.history.FindExchange(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.Exchange{}, err
	}
	return h.requests.Handle(ctx, exchange.Request())
}

type SaveTemplateHandler struct{ templates TemplateRepository }

func NewSaveTemplateHandler(templates TemplateRepository) SaveTemplateHandler {
	if templates == nil {
		panic("nil template repository")
	}
	return SaveTemplateHandler{templates}
}
func (h SaveTemplateHandler) Handle(ctx context.Context, template domain.Template) error {
	return h.templates.Save(ctx, template)
}

type ListTemplatesHandler struct{ templates TemplateRepository }

func NewListTemplatesHandler(templates TemplateRepository) ListTemplatesHandler {
	if templates == nil {
		panic("nil template repository")
	}
	return ListTemplatesHandler{templates}
}
func (h ListTemplatesHandler) Handle(ctx context.Context) ([]domain.Template, error) {
	templates, err := h.templates.ListTemplates(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(templates, func(i, j int) bool { return templates[i].Name() < templates[j].Name() })
	return templates, nil
}

type GetTemplateHandler struct{ templates TemplateRepository }

func NewGetTemplateHandler(templates TemplateRepository) GetTemplateHandler {
	if templates == nil {
		panic("nil template repository")
	}
	return GetTemplateHandler{templates}
}
func (h GetTemplateHandler) Handle(ctx context.Context, name string) (domain.Template, error) {
	return h.templates.FindTemplate(ctx, strings.TrimSpace(name))
}

type DeleteTemplateHandler struct{ templates TemplateRepository }

func NewDeleteTemplateHandler(templates TemplateRepository) DeleteTemplateHandler {
	if templates == nil {
		panic("nil template repository")
	}
	return DeleteTemplateHandler{templates}
}
func (h DeleteTemplateHandler) Handle(ctx context.Context, name string) error {
	return h.templates.Delete(ctx, strings.TrimSpace(name))
}
