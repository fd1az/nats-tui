// Package localstate persists local operator state independently from NATS.
package localstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	app "github.com/fd1az/nats-tui/internal/app/requestreply"
	domain "github.com/fd1az/nats-tui/internal/domain/requestreply"
)

type StateFile interface {
	Read() ([]byte, error)
	Write([]byte) error
}

type OSStateFile struct{ Path string }

func (f OSStateFile) Read() ([]byte, error) { return os.ReadFile(f.Path) }

func (f OSStateFile) Write(data []byte) error {
	directory := filepath.Dir(f.Path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create local state directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".nats-tui-state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary local state: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, f.Path); err != nil {
		return fmt.Errorf("replace local state: %w", err)
	}
	return nil
}

type RequestRepository struct {
	mu         sync.Mutex
	file       StateFile
	maxHistory int
}

func NewRequestRepository(file StateFile, maxHistory int) *RequestRepository {
	if file == nil {
		panic("nil request state file")
	}
	if maxHistory < 1 {
		panic("request history limit must be positive")
	}
	return &RequestRepository{file: file, maxHistory: maxHistory}
}

func (r *RequestRepository) Append(_ context.Context, exchange domain.Exchange) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return err
	}
	state.History = append(state.History, exchangeToDTO(exchange))
	if overflow := len(state.History) - r.maxHistory; overflow > 0 {
		state.History = append([]exchangeDTO(nil), state.History[overflow:]...)
	}
	return r.save(state)
}

func (r *RequestRepository) List(_ context.Context) ([]domain.Exchange, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return nil, err
	}
	result := make([]domain.Exchange, 0, len(state.History))
	for _, item := range state.History {
		exchange, err := item.exchange()
		if err != nil {
			return nil, err
		}
		result = append(result, exchange)
	}
	return result, nil
}

func (r *RequestRepository) FindExchange(ctx context.Context, id string) (domain.Exchange, error) {
	exchanges, err := r.List(ctx)
	if err != nil {
		return domain.Exchange{}, err
	}
	for _, exchange := range exchanges {
		if exchange.ID() == id {
			return exchange, nil
		}
	}
	return domain.Exchange{}, app.ErrExchangeNotFound
}

func (r *RequestRepository) Save(_ context.Context, template domain.Template) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return err
	}
	if state.Templates == nil {
		state.Templates = make(map[string]requestDTO)
	}
	state.Templates[template.Name()] = requestToDTO(template.Request())
	return r.save(state)
}

func (r *RequestRepository) ListTemplates(_ context.Context) ([]domain.Template, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return nil, err
	}
	result := make([]domain.Template, 0, len(state.Templates))
	for name, item := range state.Templates {
		request, err := item.request()
		if err != nil {
			return nil, err
		}
		template, err := domain.NewTemplate(name, request)
		if err != nil {
			return nil, err
		}
		result = append(result, template)
	}
	return result, nil
}

func (r *RequestRepository) FindTemplate(_ context.Context, name string) (domain.Template, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return domain.Template{}, err
	}
	item, ok := state.Templates[name]
	if !ok {
		return domain.Template{}, app.ErrTemplateNotFound
	}
	request, err := item.request()
	if err != nil {
		return domain.Template{}, err
	}
	return domain.NewTemplate(name, request)
}

func (r *RequestRepository) Delete(_ context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return err
	}
	if _, ok := state.Templates[name]; !ok {
		return app.ErrTemplateNotFound
	}
	delete(state.Templates, name)
	return r.save(state)
}

type localState struct {
	History   []exchangeDTO         `json:"history,omitempty"`
	Templates map[string]requestDTO `json:"templates,omitempty"`
}

type requestDTO struct {
	Subject    string              `json:"subject"`
	Payload    []byte              `json:"payload,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Timeout    time.Duration       `json:"timeout"`
	MaxReplies int                 `json:"max_replies"`
}

type replyDTO struct {
	Payload    []byte              `json:"payload,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
	ReceivedAt time.Time           `json:"received_at"`
	Latency    time.Duration       `json:"latency"`
}

type exchangeDTO struct {
	ID        string     `json:"id"`
	StartedAt time.Time  `json:"started_at"`
	Request   requestDTO `json:"request"`
	Replies   []replyDTO `json:"replies,omitempty"`
}

func (r *RequestRepository) load() (localState, error) {
	data, err := r.file.Read()
	if errors.Is(err, fs.ErrNotExist) {
		return localState{Templates: make(map[string]requestDTO)}, nil
	}
	if err != nil {
		return localState{}, fmt.Errorf("read request state: %w", err)
	}
	var state localState
	if err := json.Unmarshal(data, &state); err != nil {
		return localState{}, fmt.Errorf("decode request state: %w", err)
	}
	if state.Templates == nil {
		state.Templates = make(map[string]requestDTO)
	}
	return state, nil
}

func (r *RequestRepository) save(state localState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode request state: %w", err)
	}
	if err := r.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write request state: %w", err)
	}
	return nil
}

func requestToDTO(request domain.Request) requestDTO {
	message := request.Message()
	return requestDTO{Subject: message.Subject().String(), Payload: message.Payload(), Headers: message.Headers(), Timeout: request.Timeout(), MaxReplies: request.MaxReplies()}
}

func (d requestDTO) request() (domain.Request, error) {
	message, err := domain.NewMessage(d.Subject, d.Payload, d.Headers)
	if err != nil {
		return domain.Request{}, err
	}
	return domain.NewRequest(message, d.Timeout, d.MaxReplies)
}

func exchangeToDTO(exchange domain.Exchange) exchangeDTO {
	replies := exchange.Replies()
	dto := exchangeDTO{ID: exchange.ID(), StartedAt: exchange.StartedAt(), Request: requestToDTO(exchange.Request()), Replies: make([]replyDTO, 0, len(replies))}
	for _, reply := range replies {
		dto.Replies = append(dto.Replies, replyDTO{Payload: reply.Payload(), Headers: reply.Headers(), ReceivedAt: reply.ReceivedAt(), Latency: reply.Latency()})
	}
	return dto
}

func (d exchangeDTO) exchange() (domain.Exchange, error) {
	request, err := d.Request.request()
	if err != nil {
		return domain.Exchange{}, err
	}
	replies := make([]domain.Reply, 0, len(d.Replies))
	for _, item := range d.Replies {
		reply, err := domain.NewReply(item.Payload, item.Headers, item.ReceivedAt, item.Latency)
		if err != nil {
			return domain.Exchange{}, err
		}
		replies = append(replies, reply)
	}
	return domain.NewExchange(d.ID, d.StartedAt, request, replies)
}
