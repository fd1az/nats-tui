package localfiles

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/fd1az/nats-tui/internal/app/streams"
)

type archiveGatewayStub struct {
	restoredData     string
	restoredMetadata string
}

func (s *archiveGatewayStub) BackupSnapshot(_ context.Context, request streams.BackupRequest, progress streams.TransferProgressFunc) (streams.BackupResult, error) {
	_, _ = request.Data.Write([]byte("snapshot"))
	_, _ = request.Metadata.Write([]byte(`{"config":{"name":"ORDERS","storage":"file"},"state":{}}`))
	if progress != nil {
		progress(streams.TransferProgress{Transferred: 8, Total: 8, Chunks: 1})
	}
	return streams.BackupResult{Stream: request.Stream, Transferred: 8, Chunks: 1}, nil
}

func (s *archiveGatewayStub) RestoreSnapshot(_ context.Context, request streams.RestoreRequest, _ streams.TransferProgressFunc) (streams.RestoreResult, error) {
	data, err := io.ReadAll(request.Data)
	if err != nil {
		return streams.RestoreResult{}, err
	}
	metadata, err := io.ReadAll(request.Metadata)
	if err != nil {
		return streams.RestoreResult{}, err
	}
	s.restoredData, s.restoredMetadata = string(data), string(metadata)
	return streams.RestoreResult{Stream: request.Stream, Transferred: uint64(len(data))}, nil
}

func TestStreamArchivePublishesCompatibleDirectoryAndRestoresIt(t *testing.T) {
	t.Parallel()
	directory := filepath.Join(t.TempDir(), "ORDERS")
	gateway := new(archiveGatewayStub)
	archive := NewOSStreamArchive()
	if _, err := archive.Backup(context.Background(), gateway, streams.BackupFile{Stream: "ORDERS", Directory: directory, IncludeConsumers: true}, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{streamBackupMetadata, streamBackupData} {
		status, err := os.Stat(filepath.Join(directory, name))
		if err != nil || !status.Mode().IsRegular() || status.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s status=%v err=%v", name, status, err)
		}
	}
	if _, err := archive.Restore(context.Background(), gateway, streams.RestoreFile{Stream: "ORDERS", Directory: directory}, nil); err != nil {
		t.Fatal(err)
	}
	if gateway.restoredData != "snapshot" || gateway.restoredMetadata == "" {
		t.Fatalf("data=%q metadata=%q", gateway.restoredData, gateway.restoredMetadata)
	}
}

func TestStreamArchiveDoesNotOverwriteWithoutForceOrPublishFailedBackup(t *testing.T) {
	t.Parallel()
	directory := filepath.Join(t.TempDir(), "ORDERS")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	archive := NewOSStreamArchive()
	if _, err := archive.Backup(context.Background(), new(archiveGatewayStub), streams.BackupFile{Stream: "ORDERS", Directory: directory}, nil); err == nil {
		t.Fatal("existing destination was overwritten")
	}
	failed := &failingArchiveGateway{}
	target := filepath.Join(filepath.Dir(directory), "FAILED")
	if _, err := archive.Backup(context.Background(), failed, streams.BackupFile{Stream: "ORDERS", Directory: target}, nil); err == nil {
		t.Fatal("failed backup succeeded")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("failed backup was published: %v", err)
	}
}

func TestStreamArchiveRejectsBroadDestination(t *testing.T) {
	t.Parallel()
	archive := NewOSStreamArchive()
	if _, err := archive.Backup(context.Background(), new(archiveGatewayStub), streams.BackupFile{Stream: "ORDERS", Directory: string(filepath.Separator), Force: true}, nil); err == nil {
		t.Fatal("filesystem root was accepted as backup destination")
	}
}

func TestStreamArchiveCreatesParentsAndReplacesOnlyAfterSuccessfulSnapshot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	directory := filepath.Join(root, "nested", "backups", "ORDERS")
	archive := NewOSStreamArchive()
	if _, err := archive.Backup(context.Background(), new(archiveGatewayStub), streams.BackupFile{Stream: "ORDERS", Directory: directory}, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "obsolete"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Backup(context.Background(), new(archiveGatewayStub), streams.BackupFile{Stream: "ORDERS", Directory: directory, Force: true}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, "obsolete")); !os.IsNotExist(err) {
		t.Fatalf("old backup content survived replacement: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(directory, streamBackupData)); err != nil || string(data) != "snapshot" {
		t.Fatalf("replacement data=%q err=%v", data, err)
	}
}

type failingArchiveGateway struct{ archiveGatewayStub }

func (failingArchiveGateway) BackupSnapshot(context.Context, streams.BackupRequest, streams.TransferProgressFunc) (streams.BackupResult, error) {
	return streams.BackupResult{}, fmt.Errorf("injected snapshot failure")
}
