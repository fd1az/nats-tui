package localfiles

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/fd1az/nats-tui/internal/app/objectstore"
)

func objectDigest(value []byte) string {
	sum := sha256.Sum256(value)
	return "SHA-256=" + base64.URLEncoding.EncodeToString(sum[:])
}

type transferStoreStub struct{ value []byte }

func (s transferStoreStub) PutObject(_ context.Context, command objectstore.UploadCommand) (objectstore.ObjectSummary, error) {
	value, err := io.ReadAll(command.Reader)
	return objectstore.ObjectSummary{Name: command.Name, Size: uint64(len(value))}, err
}

func (s transferStoreStub) OpenObject(context.Context, string, string) (objectstore.DownloadSource, error) {
	return &sourceStub{Reader: bytes.NewReader(s.value), info: objectstore.ObjectSummary{Size: uint64(len(s.value)), Digest: objectDigest(s.value)}}, nil
}

type sourceStub struct {
	*bytes.Reader
	info objectstore.ObjectSummary
}

func (s *sourceStub) Close() error                    { return nil }
func (s *sourceStub) Info() objectstore.ObjectSummary { return s.info }
func (s *sourceStub) Verify() error                   { return nil }

func TestUploadFileUsesBasenameByDefault(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	path := filepath.Join(directory, "release.tar")
	if err := os.WriteFile(path, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := NewOSObjectTransfer().UploadObject(context.Background(), transferStoreStub{}, objectstore.UploadFile{Bucket: "ARTIFACTS", Path: path}, nil)
	if err != nil || result.Name != "release.tar" || result.Size != 7 {
		t.Fatalf("UploadObject() = %#v, %v", result, err)
	}
}

func TestDownloadFilePublishesOnlyVerifiedBytesAndRefusesOverwrite(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	destination := filepath.Join(directory, "artifact.bin")
	store := transferStoreStub{value: []byte("verified")}
	transfer := NewOSObjectTransfer()
	result, err := transfer.DownloadObject(context.Background(), store, objectstore.DownloadFile{Bucket: "ARTIFACTS", Name: "artifact.bin", Path: destination}, nil)
	if err != nil || !result.Verified {
		t.Fatalf("DownloadObject() = %#v, %v", result, err)
	}
	value, err := os.ReadFile(destination)
	if err != nil || string(value) != "verified" {
		t.Fatalf("destination=%q error=%v", value, err)
	}
	if _, err := transfer.DownloadObject(context.Background(), store, objectstore.DownloadFile{Bucket: "ARTIFACTS", Name: "artifact.bin", Path: destination}, nil); err == nil {
		t.Fatal("DownloadObject() overwrote destination without Force")
	}
}

type rejectingFileSystem struct {
	opened string
	err    error
}

func (f *rejectingFileSystem) Open(name string) (ReadFile, error) { f.opened = name; return nil, f.err }
func (*rejectingFileSystem) Lstat(string) (fs.FileInfo, error)    { panic("unexpected Lstat") }
func (*rejectingFileSystem) CreateTemp(string, string) (TempFile, error) {
	panic("unexpected CreateTemp")
}
func (*rejectingFileSystem) Remove(string) error         { panic("unexpected Remove") }
func (*rejectingFileSystem) Rename(string, string) error { panic("unexpected Rename") }
func (*rejectingFileSystem) Link(string, string) error   { panic("unexpected Link") }

func TestObjectTransferUsesInjectedFileSystem(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("virtual filesystem rejected path")
	files := &rejectingFileSystem{err: wantErr}
	_, err := NewObjectTransfer(files).UploadObject(context.Background(), transferStoreStub{}, objectstore.UploadFile{Bucket: "ARTIFACTS", Path: "/virtual/release.tar"}, nil)
	if !errors.Is(err, wantErr) || files.opened != "/virtual/release.tar" {
		t.Fatalf("UploadObject() error=%v opened=%q", err, files.opened)
	}
}
