package alertnotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"

	app "github.com/fd1az/nats-tui/internal/app/alerts"
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}
type Webhook struct {
	endpoint string
	client   HTTPDoer
}

func NewWebhook(endpoint string, client HTTPDoer) (*Webhook, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("webhook URL is required")
	}
	if client == nil {
		panic("nil webhook HTTP client")
	}
	return &Webhook{endpoint, client}, nil
}
func (w *Webhook) Notify(ctx context.Context, record app.Record) error {
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode webhook alert: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, w.endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := w.client.Do(request)
	if err != nil {
		return fmt.Errorf("send alert webhook: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("alert webhook returned %s", response.Status)
	}
	return nil
}

type CommandRunner interface {
	Run(context.Context, string, []string, []byte) error
}
type OSCommandRunner struct{}

func (OSCommandRunner) Run(ctx context.Context, program string, args []string, stdin []byte) error {
	command := exec.CommandContext(ctx, program, args...)
	command.Stdin = bytes.NewReader(stdin)
	return command.Run()
}

type Command struct {
	program string
	args    []string
	runner  CommandRunner
}

func NewCommand(program string, args []string, runner CommandRunner) (*Command, error) {
	program = strings.TrimSpace(program)
	if program == "" {
		return nil, fmt.Errorf("notification program is required")
	}
	if runner == nil {
		panic("nil command runner")
	}
	return &Command{program: program, args: append([]string(nil), args...), runner: runner}, nil
}
func (c *Command) Notify(ctx context.Context, record app.Record) error {
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err := c.runner.Run(ctx, c.program, c.args, payload); err != nil {
		return fmt.Errorf("run alert notification %q: %w", c.program, err)
	}
	return nil
}
