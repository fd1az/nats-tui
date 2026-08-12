package bootstrap

import (
	"context"
	"time"

	"github.com/fd1az/nats-tui/internal/adapters/localfiles"
	"github.com/fd1az/nats-tui/internal/app/messages"
	"github.com/fd1az/nats-tui/internal/config"
)

// BrowseMessageHistory executes the message history query through the
// application layer. It is shared by Cobra and Bubble Tea adapters.
func (r Runner) BrowseMessageHistory(ctx context.Context, session Session, query messages.HistoryQuery) ([]messages.Record, error) {
	ctx, cancel := r.timeout(ctx, 15*time.Second)
	defer cancel()
	return messages.NewBrowseHandler(session).Handle(ctx, query)
}

// ExportMessages exports either a supplied snapshot or a history query.
func (r Runner) ExportMessages(ctx context.Context, session Session, request messages.ExportRequest) (messages.ExportResult, error) {
	ctx, cancel := r.timeout(ctx, 30*time.Second)
	defer cancel()
	writer := r.messageFiles
	if writer == nil {
		value := localfiles.NewOSMessageFileWriter()
		writer = &value
	}
	return messages.NewExportHandler(session, writer).Handle(ctx, request)
}

// BrowseStoredMessages opens a short-lived NATS session for Cobra adapters.
func (r Runner) BrowseStoredMessages(ctx context.Context, settings config.Settings, query messages.HistoryQuery) ([]messages.Record, error) {
	session, err := r.connector()(settings)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return r.BrowseMessageHistory(ctx, session, query)
}

// ExportStoredMessages opens a short-lived NATS session and writes atomically.
func (r Runner) ExportStoredMessages(ctx context.Context, settings config.Settings, request messages.ExportRequest) (messages.ExportResult, error) {
	session, err := r.connector()(settings)
	if err != nil {
		return messages.ExportResult{}, err
	}
	defer session.Close()
	return r.ExportMessages(ctx, session, request)
}
