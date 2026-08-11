package localfiles

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type MessageFileSystem interface {
	Lstat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
	CreateTemp(string, string) (TempFile, error)
	Remove(string) error
	Rename(string, string) error
	Link(string, string) error
}

type osMessageFileSystem struct{ osFileSystem }

func (osMessageFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}

type MessageFileWriter struct{ files MessageFileSystem }

func NewMessageFileWriter(files MessageFileSystem) MessageFileWriter {
	if files == nil {
		panic("nil message export filesystem")
	}
	return MessageFileWriter{files: files}
}

func NewOSMessageFileWriter() MessageFileWriter { return NewMessageFileWriter(osMessageFileSystem{}) }

func (w MessageFileWriter) WriteMessageFile(ctx context.Context, path string, content []byte, force bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path = filepath.Clean(path)
	if !force {
		if _, err := w.files.Lstat(path); err == nil {
			return fmt.Errorf("export destination %q already exists", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect export destination %q: %w", path, err)
		}
	}
	directory := filepath.Dir(path)
	if err := w.files.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create export directory %q: %w", directory, err)
	}
	temporary, err := w.files.CreateTemp(directory, ".nats-tui-messages-*")
	if err != nil {
		return fmt.Errorf("create temporary message export: %w", err)
	}
	temporaryPath := temporary.Name()
	defer w.files.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure message export: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write message export: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync message export: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close message export: %w", err)
	}
	if force {
		if err := w.files.Rename(temporaryPath, path); err != nil {
			return fmt.Errorf("publish message export: %w", err)
		}
		return nil
	}
	if err := w.files.Link(temporaryPath, path); err != nil {
		return fmt.Errorf("publish message export without overwrite: %w", err)
	}
	return nil
}
