package requestreply

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/fd1az/nats-tui/internal/domain/requestreply"
)

type transportStub struct {
	published domain.Message
	requested domain.Request
	replies   []domain.Reply
	err       error
}

func (s *transportStub) Publish(_ context.Context, message domain.Message) error {
	s.published = message
	return s.err
}

func (s *transportStub) Request(_ context.Context, request domain.Request) ([]domain.Reply, error) {
	s.requested = request
	return s.replies, s.err
}

type historyMemory struct{ exchanges []domain.Exchange }

func (m *historyMemory) Append(_ context.Context, exchange domain.Exchange) error {
	m.exchanges = append(m.exchanges, exchange)
	return nil
}
func (m *historyMemory) List(context.Context) ([]domain.Exchange, error) {
	return append([]domain.Exchange(nil), m.exchanges...), nil
}
func (m *historyMemory) FindExchange(_ context.Context, id string) (domain.Exchange, error) {
	for _, exchange := range m.exchanges {
		if exchange.ID() == id {
			return exchange, nil
		}
	}
	return domain.Exchange{}, ErrExchangeNotFound
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type fixedIDs struct{ id string }

func (g fixedIDs) Next() string { return g.id }

func testRequest(t *testing.T) domain.Request {
	t.Helper()
	message, err := domain.NewMessage("rpc.lookup", []byte(`{"id":42}`), map[string][]string{"X-Trace": {"abc"}})
	if err != nil {
		t.Fatal(err)
	}
	request, err := domain.NewRequest(message, 2*time.Second, 2)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestPublishHandlerPassesValidatedMessageToTransport(t *testing.T) {
	t.Parallel()
	transport := &transportStub{}
	message := testRequest(t).Message()
	if err := NewPublishHandler(transport).Handle(context.Background(), message); err != nil {
		t.Fatalf("Handle() error=%v", err)
	}
	if transport.published.Subject().String() != "rpc.lookup" {
		t.Fatalf("published=%#v", transport.published)
	}
}

func TestSendRequestRecordsRepliesWithInjectedIdentityAndClock(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	reply, _ := domain.NewReply([]byte(`{"name":"Ada"}`), nil, now.Add(20*time.Millisecond), 20*time.Millisecond)
	transport := &transportStub{replies: []domain.Reply{reply}}
	history := &historyMemory{}
	handler := NewSendRequestHandler(transport, history, fixedClock{now}, fixedIDs{"req-42"}, context.WithTimeout)
	exchange, err := handler.Handle(context.Background(), testRequest(t))
	if err != nil {
		t.Fatalf("Handle() error=%v", err)
	}
	if exchange.ID() != "req-42" || len(exchange.Replies()) != 1 || len(history.exchanges) != 1 || transport.requested.MaxReplies() != 2 {
		t.Fatalf("exchange=%#v history=%#v request=%#v", exchange, history.exchanges, transport.requested)
	}
}

func TestReplayLoadsPreviousRequestAndCreatesNewExchange(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	history := &historyMemory{}
	original, _ := domain.NewExchange("original", now, testRequest(t), nil)
	history.exchanges = append(history.exchanges, original)
	requestHandler := NewSendRequestHandler(&transportStub{}, history, fixedClock{now.Add(time.Minute)}, fixedIDs{"replay"}, context.WithTimeout)
	replayed, err := NewReplayHandler(history, requestHandler).Handle(context.Background(), "original")
	if err != nil {
		t.Fatalf("Handle() error=%v", err)
	}
	if replayed.ID() != "replay" || replayed.Request().Message().Subject().String() != "rpc.lookup" || len(history.exchanges) != 2 {
		t.Fatalf("replayed=%#v history=%d", replayed, len(history.exchanges))
	}
	if _, err := NewReplayHandler(history, requestHandler).Handle(context.Background(), "missing"); !errors.Is(err, ErrExchangeNotFound) {
		t.Fatalf("missing error=%v", err)
	}
}

type templateMemory struct{ templates map[string]domain.Template }

func (m *templateMemory) Save(_ context.Context, template domain.Template) error {
	if m.templates == nil {
		m.templates = make(map[string]domain.Template)
	}
	m.templates[template.Name()] = template
	return nil
}
func (m *templateMemory) ListTemplates(context.Context) ([]domain.Template, error) {
	result := make([]domain.Template, 0, len(m.templates))
	for _, template := range m.templates {
		result = append(result, template)
	}
	return result, nil
}
func (m *templateMemory) FindTemplate(_ context.Context, name string) (domain.Template, error) {
	template, ok := m.templates[name]
	if !ok {
		return domain.Template{}, ErrTemplateNotFound
	}
	return template, nil
}
func (m *templateMemory) Delete(_ context.Context, name string) error {
	if _, ok := m.templates[name]; !ok {
		return ErrTemplateNotFound
	}
	delete(m.templates, name)
	return nil
}

func TestTemplateHandlersSaveListLoadAndDelete(t *testing.T) {
	t.Parallel()
	repository := &templateMemory{}
	template, _ := domain.NewTemplate("lookup", testRequest(t))
	if err := NewSaveTemplateHandler(repository).Handle(context.Background(), template); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewGetTemplateHandler(repository).Handle(context.Background(), "lookup")
	if err != nil || loaded.Name() != "lookup" {
		t.Fatalf("loaded=%#v error=%v", loaded, err)
	}
	listed, _ := NewListTemplatesHandler(repository).Handle(context.Background())
	if len(listed) != 1 {
		t.Fatalf("listed=%#v", listed)
	}
	if err := NewDeleteTemplateHandler(repository).Handle(context.Background(), "lookup"); err != nil {
		t.Fatal(err)
	}
}
