package localstate

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"time"

	app "github.com/clcsolutions/nats-tui/internal/app/requestreply"
	domain "github.com/clcsolutions/nats-tui/internal/domain/requestreply"
)

type memoryStateFile struct {
	data []byte
	err  error
}

func (f *memoryStateFile) Read() ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.data == nil {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), f.data...), nil
}
func (f *memoryStateFile) Write(data []byte) error {
	f.data = append([]byte(nil), data...)
	return f.err
}

func repositoryRequest(t *testing.T) domain.Request {
	t.Helper()
	message, err := domain.NewMessage("rpc.lookup", []byte(`{"id":42}`), map[string][]string{"X-Trace": {"abc"}})
	if err != nil {
		t.Fatal(err)
	}
	request, err := domain.NewRequest(message, 1500*time.Millisecond, 3)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestRequestRepositoryRoundTripsHistoryAndTemplates(t *testing.T) {
	t.Parallel()
	file := &memoryStateFile{}
	repository := NewRequestRepository(file, 100)
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	reply, _ := domain.NewReply([]byte(`{"name":"Ada"}`), map[string][]string{"X-Node": {"a"}}, now.Add(time.Millisecond), time.Millisecond)
	exchange, _ := domain.NewExchange("req-1", now, repositoryRequest(t), []domain.Reply{reply})
	if err := repository.Append(context.Background(), exchange); err != nil {
		t.Fatal(err)
	}
	template, _ := domain.NewTemplate("lookup", repositoryRequest(t))
	if err := repository.Save(context.Background(), template); err != nil {
		t.Fatal(err)
	}

	reloaded := NewRequestRepository(file, 100)
	history, err := reloaded.List(context.Background())
	if err != nil || len(history) != 1 || history[0].ID() != "req-1" || string(history[0].Replies()[0].Payload()) != `{"name":"Ada"}` {
		t.Fatalf("history=%#v error=%v", history, err)
	}
	loaded, err := reloaded.FindTemplate(context.Background(), "lookup")
	if err != nil || loaded.Request().MaxReplies() != 3 {
		t.Fatalf("template=%#v error=%v", loaded, err)
	}
}

func TestRequestRepositoryCapsNewestHistoryAndMapsNotFound(t *testing.T) {
	t.Parallel()
	file := &memoryStateFile{}
	repository := NewRequestRepository(file, 2)
	for index, id := range []string{"one", "two", "three"} {
		exchange, _ := domain.NewExchange(id, time.Date(2026, 8, 9, 12, index, 0, 0, time.UTC), repositoryRequest(t), nil)
		if err := repository.Append(context.Background(), exchange); err != nil {
			t.Fatal(err)
		}
	}
	history, _ := repository.List(context.Background())
	if len(history) != 2 || history[0].ID() != "two" || history[1].ID() != "three" {
		t.Fatalf("history=%#v", history)
	}
	if _, err := repository.FindExchange(context.Background(), "one"); !errors.Is(err, app.ErrExchangeNotFound) {
		t.Fatalf("exchange error=%v", err)
	}
	if _, err := repository.FindTemplate(context.Background(), "missing"); !errors.Is(err, app.ErrTemplateNotFound) {
		t.Fatalf("template error=%v", err)
	}
}
