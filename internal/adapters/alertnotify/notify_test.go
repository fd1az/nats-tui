package alertnotify

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	app "github.com/fd1az/nats-tui/internal/app/alerts"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }
func TestWebhookSendsJSON(t *testing.T) {
	t.Parallel()
	called := false
	notifier, err := NewWebhook("https://alerts.invalid/hook", doerFunc(func(request *http.Request) (*http.Response, error) {
		called = true
		if request.Header.Get("Content-Type") != "application/json" {
			t.Fatal("missing JSON content type")
		}
		return &http.Response{StatusCode: 204, Status: "204 No Content", Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := notifier.Notify(context.Background(), app.Record{ID: "a-1"}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("webhook not called")
	}
}

type captureRunner struct {
	program string
	args    []string
	stdin   []byte
}

func (r *captureRunner) Run(_ context.Context, program string, args []string, stdin []byte) error {
	r.program = program
	r.args = append([]string(nil), args...)
	r.stdin = append([]byte(nil), stdin...)
	return nil
}
func TestCommandDoesNotInvokeShell(t *testing.T) {
	t.Parallel()
	runner := new(captureRunner)
	notifier, err := NewCommand("notify", []string{"--channel", "ops; rm -rf nope"}, runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := notifier.Notify(context.Background(), app.Record{ID: "a-1"}); err != nil {
		t.Fatal(err)
	}
	if runner.program != "notify" || len(runner.args) != 2 || runner.args[1] != "ops; rm -rf nope" || !strings.Contains(string(runner.stdin), `"id":"a-1"`) {
		t.Fatalf("capture=%+v", runner)
	}
}
