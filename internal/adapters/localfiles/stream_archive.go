package localfiles

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fd1az/nats-tui/internal/app/streams"
)

const (
	streamBackupMetadata = "backup.json"
	streamBackupData     = "stream.tar.s2"
)

type ArchiveFile interface {
	ReadFile
	TempFile
}

type StreamArchiveFileSystem interface {
	Lstat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
	MkdirTemp(string, string) (string, error)
	OpenFile(string, int, fs.FileMode) (ArchiveFile, error)
	RemoveAll(string) error
	Rename(string, string) error
}

type osStreamArchiveFileSystem struct{}

func (osStreamArchiveFileSystem) Lstat(name string) (fs.FileInfo, error) { return os.Lstat(name) }
func (osStreamArchiveFileSystem) MkdirAll(name string, mode fs.FileMode) error {
	return os.MkdirAll(name, mode)
}
func (osStreamArchiveFileSystem) MkdirTemp(dir, pattern string) (string, error) {
	return os.MkdirTemp(dir, pattern)
}
func (osStreamArchiveFileSystem) OpenFile(name string, flag int, mode fs.FileMode) (ArchiveFile, error) {
	return os.OpenFile(name, flag, mode)
}
func (osStreamArchiveFileSystem) RemoveAll(name string) error { return os.RemoveAll(name) }
func (osStreamArchiveFileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// StreamArchive owns local persistence while the snapshot gateway owns NATS transport.
type StreamArchive struct{ files StreamArchiveFileSystem }

func NewStreamArchive(files StreamArchiveFileSystem) StreamArchive {
	if files == nil {
		panic("nil stream archive filesystem")
	}
	return StreamArchive{files: files}
}

func NewOSStreamArchive() StreamArchive { return NewStreamArchive(osStreamArchiveFileSystem{}) }

func (a StreamArchive) Backup(ctx context.Context, gateway streams.SnapshotGateway, command streams.BackupFile, progress streams.TransferProgressFunc) (streams.BackupResult, error) {
	if gateway == nil {
		return streams.BackupResult{}, fmt.Errorf("stream snapshot gateway is required")
	}
	destination := filepath.Clean(strings.TrimSpace(command.Directory))
	if destination == "." && strings.TrimSpace(command.Directory) == "" {
		return streams.BackupResult{}, fmt.Errorf("backup directory is required")
	}
	if destination == "." || filepath.Dir(destination) == destination {
		return streams.BackupResult{}, fmt.Errorf("backup destination must be a dedicated subdirectory")
	}
	_, destinationErr := a.files.Lstat(destination)
	destinationExists := destinationErr == nil
	if destinationExists && !command.Force {
		return streams.BackupResult{}, fmt.Errorf("backup destination %q already exists", destination)
	} else if destinationErr != nil && !errors.Is(destinationErr, os.ErrNotExist) {
		return streams.BackupResult{}, fmt.Errorf("inspect backup destination %q: %w", destination, destinationErr)
	}
	parent := filepath.Dir(destination)
	if err := a.files.MkdirAll(parent, 0o700); err != nil {
		return streams.BackupResult{}, fmt.Errorf("create backup parent %q: %w", parent, err)
	}
	staging, err := a.files.MkdirTemp(parent, ".nats-tui-stream-backup-*")
	if err != nil {
		return streams.BackupResult{}, fmt.Errorf("create backup staging directory in %q: %w", parent, err)
	}
	defer a.files.RemoveAll(staging)
	data, err := a.files.OpenFile(filepath.Join(staging, streamBackupData), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return streams.BackupResult{}, fmt.Errorf("create snapshot data: %w", err)
	}
	metadata, err := a.files.OpenFile(filepath.Join(staging, streamBackupMetadata), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		_ = data.Close()
		return streams.BackupResult{}, fmt.Errorf("create snapshot metadata: %w", err)
	}
	result, transferErr := streams.NewBackupHandler(gateway).Handle(ctx, streams.BackupRequest{
		Stream: command.Stream, Data: data, Metadata: metadata, IncludeConsumers: command.IncludeConsumers,
		CheckMessages: command.CheckMessages, ChunkSize: command.ChunkSize, WindowSize: command.WindowSize,
	}, progress)
	closeErr := errors.Join(syncAndClose(data), syncAndClose(metadata))
	if transferErr != nil || closeErr != nil {
		return streams.BackupResult{}, errors.Join(transferErr, closeErr)
	}
	previous := staging + "-previous"
	if destinationExists {
		if err := a.files.Rename(destination, previous); err != nil {
			return streams.BackupResult{}, fmt.Errorf("preserve previous backup %q: %w", destination, err)
		}
		defer a.files.RemoveAll(previous)
	}
	if err := a.files.Rename(staging, destination); err != nil {
		if destinationExists {
			return streams.BackupResult{}, errors.Join(fmt.Errorf("publish backup directory %q: %w", destination, err), a.files.Rename(previous, destination))
		}
		return streams.BackupResult{}, fmt.Errorf("publish backup directory %q: %w", destination, err)
	}
	return result, nil
}

func (a StreamArchive) Restore(ctx context.Context, gateway streams.SnapshotGateway, command streams.RestoreFile, progress streams.TransferProgressFunc) (streams.RestoreResult, error) {
	if gateway == nil {
		return streams.RestoreResult{}, fmt.Errorf("stream snapshot gateway is required")
	}
	directory := filepath.Clean(strings.TrimSpace(command.Directory))
	if directory == "." && strings.TrimSpace(command.Directory) == "" {
		return streams.RestoreResult{}, fmt.Errorf("backup directory is required")
	}
	if directory == "." || filepath.Dir(directory) == directory {
		return streams.RestoreResult{}, fmt.Errorf("backup source must be a dedicated subdirectory")
	}
	data, err := a.files.OpenFile(filepath.Join(directory, streamBackupData), os.O_RDONLY, 0)
	if err != nil {
		return streams.RestoreResult{}, fmt.Errorf("open snapshot data: %w", err)
	}
	status, err := data.Stat()
	if err != nil {
		_ = data.Close()
		return streams.RestoreResult{}, fmt.Errorf("inspect snapshot data: %w", err)
	}
	if !status.Mode().IsRegular() {
		_ = data.Close()
		return streams.RestoreResult{}, fmt.Errorf("snapshot data is not a regular file")
	}
	metadata, err := a.files.OpenFile(filepath.Join(directory, streamBackupMetadata), os.O_RDONLY, 0)
	if err != nil {
		_ = data.Close()
		return streams.RestoreResult{}, fmt.Errorf("open snapshot metadata: %w", err)
	}
	result, restoreErr := streams.NewRestoreHandler(gateway).Handle(ctx, streams.RestoreRequest{
		Stream: command.Stream, Data: data, Metadata: metadata, DataSize: uint64(status.Size()), ChunkSize: command.ChunkSize,
	}, progress)
	return result, errors.Join(restoreErr, data.Close(), metadata.Close())
}

func syncAndClose(file ArchiveFile) error {
	if err := file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	return file.Close()
}
