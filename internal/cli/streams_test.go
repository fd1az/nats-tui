package cli

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/fd1az/nats-tui/internal/app/streams"
	"github.com/fd1az/nats-tui/internal/config"
)

type streamManagerStub struct {
	settings config.Settings
	items    []streams.Summary
	command  streams.Command
	purge    streams.PurgeCommand
	deleted  streams.MessageDeleteCommand
	backup   streams.BackupFile
	restore  streams.RestoreFile
}

func (s *streamManagerStub) BackupStream(_ context.Context, settings config.Settings, command streams.BackupFile, progress streams.TransferProgressFunc) (streams.BackupResult, error) {
	s.settings, s.backup = settings, command
	progress(streams.TransferProgress{Transferred: 7, Total: 7, Chunks: 1})
	return streams.BackupResult{Stream: command.Stream, Messages: 2, Transferred: 7, Chunks: 1}, nil
}

func (s *streamManagerStub) RestoreStream(_ context.Context, settings config.Settings, command streams.RestoreFile, progress streams.TransferProgressFunc) (streams.RestoreResult, error) {
	s.settings, s.restore = settings, command
	progress(streams.TransferProgress{Transferred: 7, Total: 7, Chunks: 1})
	return streams.RestoreResult{Stream: command.Stream, Messages: 2, Bytes: 12, Transferred: 7, Chunks: 1}, nil
}

func (s *streamManagerStub) ListStreamCatalog(_ context.Context, settings config.Settings) ([]streams.Summary, error) {
	s.settings = settings
	return s.items, nil
}
func (s *streamManagerStub) OperateStreamConfig(_ context.Context, settings config.Settings, command streams.Command) error {
	s.settings, s.command = settings, command
	return nil
}
func (s *streamManagerStub) PurgeStreamMessages(_ context.Context, settings config.Settings, command streams.PurgeCommand) error {
	s.settings, s.purge = settings, command
	return nil
}
func (s *streamManagerStub) DeleteStoredStreamMessage(_ context.Context, settings config.Settings, command streams.MessageDeleteCommand) error {
	s.settings, s.deleted = settings, command
	return nil
}

func TestStreamCreateSupportsAdvancedMirrorConfiguration(t *testing.T) {
	t.Parallel()
	service := new(streamManagerStub)
	root := NewRoot(Dependencies{Streams: service})
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"--profile", "production", "stream", "create", "ORDERS_MIRROR", "--mirror", "ORDERS", "--mirror-start-sequence", "42", "--mirror-filter", "orders.eu.>", "--storage", "file", "--replicas", "3", "--compression", "--mirror-direct", "--metadata", "owner=platform"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	d := service.command.Definition
	if service.settings.Profile != "production" || d.Mirror == nil || d.Mirror.Name != "ORDERS" || d.Mirror.StartSequence != 42 || d.Mirror.FilterSubject != "orders.eu.>" {
		t.Fatalf("definition=%+v settings=%+v", d, service.settings)
	}
	if d.Storage != streams.StorageFile || d.Replicas != 3 || !d.Compression || !d.MirrorDirect || d.Metadata["owner"] != "platform" {
		t.Fatalf("advanced definition=%+v", d)
	}
}

func TestSourceConfigJSONSupportsFiltersTransformsAndExternalPrefixes(t *testing.T) {
	t.Parallel()
	source, err := parseStreamSourceJSON(`{"name":"ORDERS","start_sequence":42,"filter_subject":"orders.eu.>","external_api":"$JS.EU.API","deliver_prefix":"deliver.eu","transforms":[{"source":"orders.*","destination":"eu.$1"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if source.Name != "ORDERS" || source.StartSequence != 42 || source.FilterSubject != "orders.eu.>" || source.External == nil || source.External.APIPrefix != "$JS.EU.API" || len(source.SubjectTransforms) != 1 {
		t.Fatalf("source=%+v", source)
	}
}

func TestStreamEditPreservesOmittedAdvancedConfiguration(t *testing.T) {
	t.Parallel()
	service := &streamManagerStub{items: []streams.Summary{{
		Name: "ORDERS", Subjects: []string{"orders.>"}, Storage: streams.StorageFile, Retention: streams.RetentionLimits,
		MaxMessages: -1, MaxBytes: -1, Replicas: 1, MaxAge: 24 * time.Hour, AllowDirect: true,
		Metadata: map[string]string{"owner": "platform"},
	}}}
	root := NewRoot(Dependencies{Streams: service})
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"stream", "edit", "ORDERS", "--description", "regional orders", "--max-messages", "500"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	d := service.command.Definition
	if d.Description != "regional orders" || d.MaxMessages != 500 || d.MaxAge != 24*time.Hour || !d.AllowDirect || d.Metadata["owner"] != "platform" {
		t.Fatalf("definition=%+v", d)
	}
}

func TestStreamPurgeAndMessageDeleteRequireConfirmation(t *testing.T) {
	t.Parallel()
	service := new(streamManagerStub)
	unsafe := NewRoot(Dependencies{Streams: service})
	unsafe.SetArgs([]string{"stream", "purge", "ORDERS"})
	if err := unsafe.ExecuteContext(context.Background()); err == nil {
		t.Fatal("purge without confirmation succeeded")
	}
	safe := NewRoot(Dependencies{Streams: service})
	safe.SetOut(new(bytes.Buffer))
	safe.SetArgs([]string{"stream", "purge", "ORDERS", "--subject", "orders.eu.>", "--keep", "10", "--yes"})
	if err := safe.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.purge.Subject != "orders.eu.>" || service.purge.Keep != 10 {
		t.Fatalf("purge=%+v", service.purge)
	}
	deleteMessage := NewRoot(Dependencies{Streams: service})
	deleteMessage.SetOut(new(bytes.Buffer))
	deleteMessage.SetArgs([]string{"stream", "delete-message", "ORDERS", "42", "--secure", "--yes"})
	if err := deleteMessage.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.deleted.Sequence != 42 || !service.deleted.Secure {
		t.Fatalf("deleted=%+v", service.deleted)
	}
}

func TestStreamBackupKeepsMachineResultOnStdoutAndProgressOnStderr(t *testing.T) {
	t.Parallel()
	service := new(streamManagerStub)
	root := NewRoot(Dependencies{Streams: service, Clock: fixedClock{now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}})
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"--profile", "staging", "stream", "backup", "ORDERS", "/tmp/orders", "--check", "--consumers=false", "--chunk-size", "65536", "--window-size", "1048576", "--force"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.settings.Profile != "staging" || service.backup.Stream != "ORDERS" || service.backup.Directory != "/tmp/orders" || service.backup.IncludeConsumers || !service.backup.CheckMessages || !service.backup.Force {
		t.Fatalf("settings=%+v backup=%+v", service.settings, service.backup)
	}
	if got := stdout.String(); got != "backed up stream ORDERS to /tmp/orders (2 messages, 7 compressed bytes)\n" {
		t.Fatalf("stdout=%q", got)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("receiving")) || !bytes.Contains(stderr.Bytes(), []byte("1 chunks")) {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestStreamRestoreRequiresConfirmationAndReportsResult(t *testing.T) {
	t.Parallel()
	service := new(streamManagerStub)
	unsafe := NewRoot(Dependencies{Streams: service})
	unsafe.SetArgs([]string{"stream", "restore", "ORDERS", "/tmp/orders"})
	if err := unsafe.ExecuteContext(context.Background()); err == nil {
		t.Fatal("restore without confirmation succeeded")
	}
	root := NewRoot(Dependencies{Streams: service, Clock: fixedClock{now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}})
	stdout := new(bytes.Buffer)
	root.SetOut(stdout)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"stream", "restore", "ORDERS", "/tmp/orders", "--yes", "--chunk-size", "32768"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.restore.ChunkSize != 32768 || stdout.String() != "restored stream ORDERS from /tmp/orders (2 messages, 12 bytes)\n" {
		t.Fatalf("restore=%+v stdout=%q", service.restore, stdout.String())
	}
}
