package streams

import (
	"context"
	"fmt"
	"io"
	"strings"
)

const (
	DefaultSnapshotChunkSize  = 128 * 1024
	DefaultSnapshotWindowSize = 8 * 1024 * 1024
	DefaultRestoreChunkSize   = 64 * 1024
)

// TransferProgress is a transport-neutral stream backup checkpoint.
type TransferProgress struct {
	Transferred uint64
	Total       uint64
	Chunks      uint32
}

type TransferProgressFunc func(TransferProgress)

// BackupRequest describes a server snapshot without coupling the use case to a filesystem.
type BackupRequest struct {
	Stream           string
	Data             io.Writer
	Metadata         io.Writer
	IncludeConsumers bool
	CheckMessages    bool
	ChunkSize        int
	WindowSize       int
}

// RestoreRequest describes a snapshot upload without coupling the use case to a filesystem.
type RestoreRequest struct {
	Stream    string
	Data      io.Reader
	Metadata  io.Reader
	DataSize  uint64
	ChunkSize int
}

// BackupResult summarizes the consistent state captured by the server.
type BackupResult struct {
	Stream      string
	Messages    uint64
	Bytes       uint64
	Consumers   int
	Transferred uint64
	Chunks      uint32
}

// RestoreResult summarizes the stream state created from an archive.
type RestoreResult struct {
	Stream      string
	Messages    uint64
	Bytes       uint64
	Consumers   int
	Transferred uint64
	Chunks      uint32
}

// BackupFile is the application-facing request for a portable backup directory.
type BackupFile struct {
	Stream           string
	Directory        string
	Force            bool
	IncludeConsumers bool
	CheckMessages    bool
	ChunkSize        int
	WindowSize       int
}

// RestoreFile is the application-facing request for a portable backup directory.
type RestoreFile struct {
	Stream    string
	Directory string
	ChunkSize int
}

// TransferEvent carries progress or terminal state to interactive clients.
type TransferEvent struct {
	Progress  TransferProgress
	Backup    BackupResult
	Restore   RestoreResult
	Directory string
	Err       error
	Done      bool
}

type SnapshotGateway interface {
	BackupSnapshot(context.Context, BackupRequest, TransferProgressFunc) (BackupResult, error)
	RestoreSnapshot(context.Context, RestoreRequest, TransferProgressFunc) (RestoreResult, error)
}

type BackupHandler struct{ gateway SnapshotGateway }

func NewBackupHandler(gateway SnapshotGateway) BackupHandler {
	if gateway == nil {
		panic("nil stream snapshot gateway")
	}
	return BackupHandler{gateway: gateway}
}

func (h BackupHandler) Handle(ctx context.Context, request BackupRequest, progress TransferProgressFunc) (BackupResult, error) {
	if err := validateBackupRequest(request); err != nil {
		return BackupResult{}, err
	}
	if request.ChunkSize == 0 {
		request.ChunkSize = DefaultSnapshotChunkSize
	}
	if request.WindowSize == 0 {
		request.WindowSize = DefaultSnapshotWindowSize
	}
	return h.gateway.BackupSnapshot(ctx, request, progress)
}

type RestoreHandler struct{ gateway SnapshotGateway }

func NewRestoreHandler(gateway SnapshotGateway) RestoreHandler {
	if gateway == nil {
		panic("nil stream snapshot gateway")
	}
	return RestoreHandler{gateway: gateway}
}

func (h RestoreHandler) Handle(ctx context.Context, request RestoreRequest, progress TransferProgressFunc) (RestoreResult, error) {
	if err := validateSnapshotStreamName(request.Stream); err != nil {
		return RestoreResult{}, err
	}
	if request.Data == nil || request.Metadata == nil {
		return RestoreResult{}, fmt.Errorf("snapshot data and metadata are required")
	}
	if request.ChunkSize == 0 {
		request.ChunkSize = DefaultRestoreChunkSize
	}
	if request.ChunkSize < 1024 || request.ChunkSize > 1024*1024 {
		return RestoreResult{}, fmt.Errorf("restore chunk size must be between 1 KiB and 1 MiB")
	}
	return h.gateway.RestoreSnapshot(ctx, request, progress)
}

func validateBackupRequest(request BackupRequest) error {
	if err := validateSnapshotStreamName(request.Stream); err != nil {
		return err
	}
	if request.Data == nil || request.Metadata == nil {
		return fmt.Errorf("snapshot data and metadata destinations are required")
	}
	if request.ChunkSize != 0 && (request.ChunkSize < 1024 || request.ChunkSize > 1024*1024) {
		return fmt.Errorf("snapshot chunk size must be between 1 KiB and 1 MiB")
	}
	if request.WindowSize != 0 && (request.WindowSize < 1024 || request.WindowSize > 32*1024*1024) {
		return fmt.Errorf("snapshot window size must be between 1 KiB and 32 MiB")
	}
	if request.ChunkSize > 0 && request.WindowSize > 0 && request.WindowSize < request.ChunkSize {
		return fmt.Errorf("snapshot window size cannot be smaller than chunk size")
	}
	return nil
}

func validateSnapshotStreamName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("stream name is required")
	}
	if strings.ContainsAny(name, " \t\r\n.*>/\\") {
		return fmt.Errorf("stream name contains unsupported characters")
	}
	return nil
}
