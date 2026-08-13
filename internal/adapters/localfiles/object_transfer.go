// Package localfiles adapts local filesystem paths to object transfer streams.
package localfiles

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fd1az/nats-tui/internal/app/objectstore"
)

type ReadFile interface {
	io.Reader
	Stat() (fs.FileInfo, error)
	Close() error
}

type TempFile interface {
	io.Writer
	Name() string
	Chmod(fs.FileMode) error
	Sync() error
	Close() error
}

// FileSystem is the narrow filesystem capability required by object transfers.
// Tests can provide an in-memory or failure-injecting implementation.
type FileSystem interface {
	Open(string) (ReadFile, error)
	Lstat(string) (fs.FileInfo, error)
	CreateTemp(string, string) (TempFile, error)
	Remove(string) error
	Rename(string, string) error
	Link(string, string) error
}

type osFileSystem struct{}

// #nosec G304 -- the operator explicitly selects the upload source path.
func (osFileSystem) Open(name string) (ReadFile, error)     { return os.Open(name) }
func (osFileSystem) Lstat(name string) (fs.FileInfo, error) { return os.Lstat(name) }
func (osFileSystem) CreateTemp(dir, pattern string) (TempFile, error) {
	return os.CreateTemp(dir, pattern)
}
func (osFileSystem) Remove(name string) error             { return os.Remove(name) }
func (osFileSystem) Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }
func (osFileSystem) Link(oldPath, newPath string) error   { return os.Link(oldPath, newPath) }

// ObjectTransfer adapts explicit filesystem capabilities to object streams.
type ObjectTransfer struct{ files FileSystem }

func NewObjectTransfer(files FileSystem) ObjectTransfer {
	if files == nil {
		panic("nil object transfer filesystem")
	}
	return ObjectTransfer{files: files}
}

func NewOSObjectTransfer() ObjectTransfer { return NewObjectTransfer(osFileSystem{}) }

// UploadObject opens and streams a regular local file.
func (t ObjectTransfer) UploadObject(ctx context.Context, store objectstore.TransferStore, command objectstore.UploadFile, progress objectstore.ProgressFunc) (objectstore.ObjectSummary, error) {
	file, err := t.files.Open(command.Path)
	if err != nil {
		return objectstore.ObjectSummary{}, fmt.Errorf("open upload file %q: %w", command.Path, err)
	}
	status, err := file.Stat()
	if err != nil {
		return objectstore.ObjectSummary{}, errors.Join(fmt.Errorf("inspect upload file %q: %w", command.Path, err), file.Close())
	}
	if !status.Mode().IsRegular() {
		return objectstore.ObjectSummary{}, errors.Join(fmt.Errorf("upload path %q is not a regular file", command.Path), file.Close())
	}
	if status.Size() < 0 {
		return objectstore.ObjectSummary{}, errors.Join(fmt.Errorf("upload path %q has an invalid negative size", command.Path), file.Close())
	}
	name := strings.TrimSpace(command.Name)
	if name == "" {
		name = filepath.Base(command.Path)
	}
	result, transferErr := objectstore.NewUploadHandler(store).Handle(ctx, objectstore.UploadCommand{
		Bucket: command.Bucket, Name: name, Description: command.Description,
		// #nosec G115 -- negative file sizes are rejected above.
		Headers: command.Headers, Metadata: command.Metadata, Reader: file, Size: uint64(status.Size()),
	}, progress)
	if closeErr := file.Close(); closeErr != nil {
		return objectstore.ObjectSummary{}, errors.Join(transferErr, fmt.Errorf("close upload file: %w", closeErr))
	}
	return result, transferErr
}

// DownloadObject verifies a temporary file before atomically publishing it.
func (t ObjectTransfer) DownloadObject(ctx context.Context, store objectstore.TransferStore, command objectstore.DownloadFile, progress objectstore.ProgressFunc) (objectstore.DownloadResult, error) {
	if strings.TrimSpace(command.Path) == "" {
		return objectstore.DownloadResult{}, fmt.Errorf("download destination is required")
	}
	if !command.Force {
		if _, err := t.files.Lstat(command.Path); err == nil {
			return objectstore.DownloadResult{}, fmt.Errorf("destination %q already exists", command.Path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return objectstore.DownloadResult{}, fmt.Errorf("inspect destination %q: %w", command.Path, err)
		}
	}
	directory := filepath.Dir(command.Path)
	temporary, err := t.files.CreateTemp(directory, ".nats-tui-download-*")
	if err != nil {
		return objectstore.DownloadResult{}, fmt.Errorf("create temporary download in %q: %w", directory, err)
	}
	temporaryPath := temporary.Name()
	defer t.files.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return objectstore.DownloadResult{}, fmt.Errorf("secure temporary download: %w", err)
	}
	result, transferErr := objectstore.NewDownloadHandler(store).Handle(ctx, objectstore.DownloadCommand{
		Bucket: command.Bucket, Name: command.Name, Writer: temporary,
	}, progress)
	if transferErr != nil {
		return objectstore.DownloadResult{}, errors.Join(transferErr, temporary.Close())
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return objectstore.DownloadResult{}, fmt.Errorf("sync downloaded object: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return objectstore.DownloadResult{}, fmt.Errorf("close downloaded object: %w", err)
	}
	if command.Force {
		if err := t.files.Rename(temporaryPath, command.Path); err != nil {
			return objectstore.DownloadResult{}, fmt.Errorf("publish download to %q: %w", command.Path, err)
		}
		return result, nil
	}
	if err := t.files.Link(temporaryPath, command.Path); err != nil {
		return objectstore.DownloadResult{}, fmt.Errorf("publish download to %q without overwrite: %w", command.Path, err)
	}
	return result, nil
}
