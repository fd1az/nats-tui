package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/clcsolutions/nats-tui/internal/config"
	domain "github.com/clcsolutions/nats-tui/internal/domain/requestreply"
)

type coreServiceStub struct {
	published domain.Message
	requested domain.Request
	exchange  domain.Exchange
}

type requestStateServiceStub struct {
	coreServiceStub
	history   []domain.Exchange
	templates map[string]domain.Template
	deleted   string
}

func (s *requestStateServiceStub) ListRequestHistory(context.Context) ([]domain.Exchange, error) {
	return s.history, nil
}
func (s *requestStateServiceStub) ReplayCoreRequest(_ context.Context, _ config.Settings, _ string) (domain.Exchange, error) {
	return s.exchange, nil
}
func (s *requestStateServiceStub) SaveRequestTemplate(_ context.Context, template domain.Template) error {
	if s.templates == nil {
		s.templates = make(map[string]domain.Template)
	}
	s.templates[template.Name()] = template
	return nil
}
func (s *requestStateServiceStub) ListRequestTemplates(context.Context) ([]domain.Template, error) {
	result := make([]domain.Template, 0, len(s.templates))
	for _, template := range s.templates {
		result = append(result, template)
	}
	return result, nil
}
func (s *requestStateServiceStub) GetRequestTemplate(_ context.Context, name string) (domain.Template, error) {
	return s.templates[name], nil
}
func (s *requestStateServiceStub) DeleteRequestTemplate(_ context.Context, name string) error {
	s.deleted = name
	delete(s.templates, name)
	return nil
}

type requestFileSystem struct{ content []byte }

func (f requestFileSystem) ReadFile(string) ([]byte, error) {
	return append([]byte(nil), f.content...), nil
}
func (requestFileSystem) OpenFile(string, int, os.FileMode) (io.WriteCloser, error) {
	return nil, errors.New("not implemented")
}

func (s *coreServiceStub) PublishCoreMessage(_ context.Context, _ config.Settings, message domain.Message) error {
	s.published = message
	return nil
}
func (s *coreServiceStub) SendCoreRequest(_ context.Context, _ config.Settings, request domain.Request) (domain.Exchange, error) {
	s.requested = request
	return s.exchange, nil
}

func TestPublishCommandSupportsHeadersAndPayloadFile(t *testing.T) {
	t.Parallel()
	service := &coreServiceStub{}
	root := NewRoot(Dependencies{CorePublisher: service, FileSystem: requestFileSystem{content: []byte(`{"id":42}`)}})
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"publish", "events.created", "--file", "payload.json", "-H", "Content-Type=application/json", "-H", "X-Trace=abc"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if string(service.published.Payload()) != `{"id":42}` || service.published.Headers()["X-Trace"][0] != "abc" {
		t.Fatalf("message=%#v", service.published)
	}
}

func TestRequestCommandDisplaysMultipleRepliesAndBuildsCLIEquivalent(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	message, _ := domain.NewMessage("rpc.lookup", []byte(`{"id":42}`), map[string][]string{"X-Trace": {"a b"}})
	request, _ := domain.NewRequest(message, 3*time.Second, 2)
	replyA, _ := domain.NewReply([]byte(`{"node":"a"}`), nil, now, time.Millisecond)
	replyB, _ := domain.NewReply([]byte("plain"), nil, now, 2*time.Millisecond)
	exchange, _ := domain.NewExchange("req-1", now, request, []domain.Reply{replyA, replyB})
	service := &coreServiceStub{exchange: exchange}
	output := new(bytes.Buffer)
	root := NewRoot(Dependencies{CoreRequester: service})
	root.SetOut(output)
	root.SetArgs([]string{"request", "rpc.lookup", `{"id":42}`, "--timeout", "3s", "--max-replies", "2", "-H", "X-Trace=a b"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.requested.MaxReplies() != 2 || !strings.Contains(output.String(), `"node": "a"`) || !strings.Contains(output.String(), `"plain"`) {
		t.Fatalf("request=%#v output=%s", service.requested, output)
	}
	equivalent := CLIEquivalent(request)
	for _, want := range []string{"nats-tui request rpc.lookup", "'X-Trace=a b'", "--max-replies 2"} {
		if !strings.Contains(equivalent, want) {
			t.Fatalf("equivalent missing %q: %s", want, equivalent)
		}
	}
}

func TestRequestHistoryReplayAndTemplateCommands(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	message, _ := domain.NewMessage("rpc.lookup", []byte(`{"id":42}`), nil)
	request, _ := domain.NewRequest(message, time.Second, 1)
	exchange, _ := domain.NewExchange("req-1", now, request, nil)
	service := &requestStateServiceStub{history: []domain.Exchange{exchange}}
	service.exchange = exchange

	run := func(args ...string) string {
		t.Helper()
		output := new(bytes.Buffer)
		root := NewRoot(Dependencies{CoreRequester: service, RequestHistory: service, RequestTemplates: service})
		root.SetOut(output)
		root.SetArgs(args)
		if err := root.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return output.String()
	}
	if output := run("request", "history"); !strings.Contains(output, "req-1") || !strings.Contains(output, "rpc.lookup") {
		t.Fatalf("history=%s", output)
	}
	if output := run("request", "replay", "req-1"); !strings.Contains(output, `"id": "req-1"`) {
		t.Fatalf("replay=%s", output)
	}
	run("request", "template", "save", "lookup", "rpc.lookup", `{"id":42}`)
	if output := run("request", "template", "list"); !strings.Contains(output, "lookup\trpc.lookup") {
		t.Fatalf("templates=%s", output)
	}
	if output := run("request", "template", "run", "lookup"); !strings.Contains(output, `"subject": "rpc.lookup"`) {
		t.Fatalf("run=%s", output)
	}
	run("request", "template", "delete", "lookup", "--yes")
	if service.deleted != "lookup" {
		t.Fatalf("deleted=%q", service.deleted)
	}
}
