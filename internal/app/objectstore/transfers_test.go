package objectstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

type transferStoreStub struct {
	upload UploadCommand
	value  []byte
	info   ObjectSummary
	verify error
}

func (s *transferStoreStub) PutObject(_ context.Context, command UploadCommand) (ObjectSummary, error) {
	s.upload = command
	value, err := io.ReadAll(command.Reader)
	if err != nil {
		return ObjectSummary{}, err
	}
	s.value = value
	return s.info, nil
}

func (s *transferStoreStub) OpenObject(context.Context, string, string) (DownloadSource, error) {
	return &downloadSourceStub{Reader: bytes.NewReader(s.value), info: s.info, verify: s.verify}, nil
}

type downloadSourceStub struct {
	*bytes.Reader
	info   ObjectSummary
	verify error
}

func (s *downloadSourceStub) Close() error        { return nil }
func (s *downloadSourceStub) Info() ObjectSummary { return s.info }
func (s *downloadSourceStub) Verify() error       { return s.verify }

func TestUploadHandlerStreamsBytesAndReportsProgress(t *testing.T) {
	t.Parallel()
	store := &transferStoreStub{info: ObjectSummary{Name: "release.tar", Digest: digestFor([]byte("release"))}}
	var progress []Progress
	command := UploadCommand{Bucket: "ARTIFACTS", Name: "release.tar", Reader: bytes.NewBufferString("release"), Size: 7}
	got, err := NewUploadHandler(store).Handle(context.Background(), command, func(update Progress) { progress = append(progress, update) })
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if string(store.value) != "release" || got.Name != "release.tar" {
		t.Fatalf("stored=%q result=%#v", store.value, got)
	}
	if len(progress) == 0 || progress[len(progress)-1] != (Progress{Transferred: 7, Total: 7}) {
		t.Fatalf("progress = %#v", progress)
	}
}

func TestDownloadHandlerStreamsBytesAndVerifiesDigest(t *testing.T) {
	t.Parallel()
	value := []byte("verified payload")
	store := &transferStoreStub{value: value, info: ObjectSummary{Name: "artifact.bin", Size: uint64(len(value)), Digest: digestFor(value)}}
	output := new(bytes.Buffer)
	var progress Progress
	got, err := NewDownloadHandler(store).Handle(context.Background(), DownloadCommand{Bucket: "ARTIFACTS", Name: "artifact.bin", Writer: output}, func(update Progress) { progress = update })
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !got.Verified || !bytes.Equal(output.Bytes(), value) || progress.Transferred != uint64(len(value)) {
		t.Fatalf("result=%#v output=%q progress=%#v", got, output.Bytes(), progress)
	}
}

func TestDownloadHandlerRejectsDigestMismatchAndSourceVerificationFailure(t *testing.T) {
	t.Parallel()
	store := &transferStoreStub{value: []byte("corrupt"), info: ObjectSummary{Name: "artifact.bin", Size: 7, Digest: digestFor([]byte("expected"))}}
	_, err := NewDownloadHandler(store).Handle(context.Background(), DownloadCommand{Bucket: "ARTIFACTS", Name: "artifact.bin", Writer: io.Discard}, nil)
	if !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("digest error = %v", err)
	}
	store.info.Digest = digestFor(store.value)
	store.verify = errors.New("NATS verification failed")
	_, err = NewDownloadHandler(store).Handle(context.Background(), DownloadCommand{Bucket: "ARTIFACTS", Name: "artifact.bin", Writer: io.Discard}, nil)
	if err == nil || err.Error() != "verify object download: NATS verification failed" {
		t.Fatalf("source verification error = %v", err)
	}
}
