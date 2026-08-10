package streams

import (
	"bytes"
	"context"
	"testing"
)

type snapshotGatewayStub struct {
	backup  BackupRequest
	restore RestoreRequest
}

func (s *snapshotGatewayStub) BackupSnapshot(_ context.Context, request BackupRequest, progress TransferProgressFunc) (BackupResult, error) {
	s.backup = request
	if progress != nil {
		progress(TransferProgress{Transferred: 4, Total: 4, Chunks: 1})
	}
	return BackupResult{Stream: request.Stream, Transferred: 4, Chunks: 1}, nil
}

func (s *snapshotGatewayStub) RestoreSnapshot(_ context.Context, request RestoreRequest, progress TransferProgressFunc) (RestoreResult, error) {
	s.restore = request
	if progress != nil {
		progress(TransferProgress{Transferred: request.DataSize, Total: request.DataSize, Chunks: 1})
	}
	return RestoreResult{Stream: request.Stream, Transferred: request.DataSize, Chunks: 1}, nil
}

func TestBackupHandlerDefaultsTransportAndReportsProgress(t *testing.T) {
	t.Parallel()
	gateway := new(snapshotGatewayStub)
	var progress TransferProgress
	result, err := NewBackupHandler(gateway).Handle(context.Background(), BackupRequest{Stream: "ORDERS", Data: new(bytes.Buffer), Metadata: new(bytes.Buffer), IncludeConsumers: true}, func(value TransferProgress) { progress = value })
	if err != nil {
		t.Fatal(err)
	}
	if gateway.backup.ChunkSize != DefaultSnapshotChunkSize || gateway.backup.WindowSize != DefaultSnapshotWindowSize {
		t.Fatalf("request=%+v", gateway.backup)
	}
	if result.Stream != "ORDERS" || progress.Transferred != 4 {
		t.Fatalf("result=%+v progress=%+v", result, progress)
	}
}

func TestBackupHandlerRejectsInvalidFlowControl(t *testing.T) {
	t.Parallel()
	_, err := NewBackupHandler(new(snapshotGatewayStub)).Handle(context.Background(), BackupRequest{Stream: "ORDERS", Data: new(bytes.Buffer), Metadata: new(bytes.Buffer), ChunkSize: 2048, WindowSize: 1024}, nil)
	if err == nil {
		t.Fatal("smaller window was accepted")
	}
}

func TestBackupHandlerRejectsSubjectLikeStreamName(t *testing.T) {
	t.Parallel()
	_, err := NewBackupHandler(new(snapshotGatewayStub)).Handle(context.Background(), BackupRequest{Stream: "ORDERS.>", Data: new(bytes.Buffer), Metadata: new(bytes.Buffer)}, nil)
	if err == nil {
		t.Fatal("subject-like stream name was accepted")
	}
}

func TestRestoreHandlerDefaultsChunkSizeAndValidatesInputs(t *testing.T) {
	t.Parallel()
	gateway := new(snapshotGatewayStub)
	result, err := NewRestoreHandler(gateway).Handle(context.Background(), RestoreRequest{Stream: "ORDERS", Data: bytes.NewReader([]byte("data")), Metadata: bytes.NewReader([]byte("{}")), DataSize: 4}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if gateway.restore.ChunkSize != DefaultRestoreChunkSize || result.Transferred != 4 {
		t.Fatalf("request=%+v result=%+v", gateway.restore, result)
	}
	if _, err := NewRestoreHandler(gateway).Handle(context.Background(), RestoreRequest{}, nil); err == nil {
		t.Fatal("invalid restore was accepted")
	}
}
