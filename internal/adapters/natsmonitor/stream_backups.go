package natsmonitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/fd1az/nats-tui/internal/app/streams"
	"github.com/nats-io/nats.go"
)

const maxSnapshotMetadataSize = 16 << 20

type streamStateWire struct {
	Messages  uint64 `json:"messages"`
	Bytes     uint64 `json:"bytes"`
	Consumers int    `json:"consumer_count"`
}

type snapshotResponseWire struct {
	Type   string          `json:"type"`
	Config json.RawMessage `json:"config"`
	State  json.RawMessage `json:"state"`
	Error  *apiErrorWire   `json:"error,omitempty"`
}

type restoreResponseWire struct {
	Type           string        `json:"type"`
	DeliverSubject string        `json:"deliver_subject"`
	Error          *apiErrorWire `json:"error,omitempty"`
}

type streamCreateResponseWire struct {
	Type  string          `json:"type"`
	State streamStateWire `json:"state"`
	Error *apiErrorWire   `json:"error,omitempty"`
}

type snapshotRequestWire struct {
	DeliverSubject string `json:"deliver_subject"`
	NoConsumers    bool   `json:"no_consumers,omitempty"`
	ChunkSize      int    `json:"chunk_size,omitempty"`
	WindowSize     int    `json:"window_size,omitempty"`
	CheckMessages  bool   `json:"jsck,omitempty"`
}

func (m *Monitor) BackupSnapshot(ctx context.Context, request streams.BackupRequest, progress streams.TransferProgressFunc) (streams.BackupResult, error) {
	inbox := m.connection.NewRespInbox()
	subscription, err := m.connection.SubscribeSync(inbox)
	if err != nil {
		return streams.BackupResult{}, fmt.Errorf("subscribe for stream snapshot: %w", err)
	}
	defer func() { _ = subscription.Unsubscribe() }()
	if err := subscription.SetPendingLimits(-1, -1); err != nil {
		return streams.BackupResult{}, fmt.Errorf("configure stream snapshot subscription: %w", err)
	}
	if err := m.connection.FlushWithContext(ctx); err != nil {
		return streams.BackupResult{}, fmt.Errorf("activate stream snapshot subscription: %w", err)
	}
	payload, err := json.Marshal(snapshotRequestWire{
		DeliverSubject: inbox, NoConsumers: !request.IncludeConsumers, ChunkSize: request.ChunkSize,
		WindowSize: request.WindowSize, CheckMessages: request.CheckMessages,
	})
	if err != nil {
		return streams.BackupResult{}, fmt.Errorf("encode stream snapshot request: %w", err)
	}
	message, err := m.connection.RequestWithContext(ctx, fmt.Sprintf("$JS.API.STREAM.SNAPSHOT.%s", request.Stream), payload)
	if err != nil {
		return streams.BackupResult{}, fmt.Errorf("request stream snapshot: %w", err)
	}
	var response snapshotResponseWire
	if err := json.Unmarshal(message.Data, &response); err != nil {
		return streams.BackupResult{}, fmt.Errorf("decode stream snapshot response: %w", err)
	}
	if err := jetStreamAPIError(response.Error); err != nil {
		return streams.BackupResult{}, fmt.Errorf("snapshot stream %q: %w", request.Stream, err)
	}
	var snapshotState streamStateWire
	if err := json.Unmarshal(response.State, &snapshotState); err != nil {
		return streams.BackupResult{}, fmt.Errorf("decode stream snapshot state: %w", err)
	}
	metadata, err := json.MarshalIndent(struct {
		Config json.RawMessage `json:"config"`
		State  json.RawMessage `json:"state"`
	}{Config: response.Config, State: response.State}, "", "  ")
	if err != nil {
		return streams.BackupResult{}, fmt.Errorf("encode stream snapshot metadata: %w", err)
	}

	var transferred uint64
	var chunks uint32
	for {
		chunk, err := subscription.NextMsgWithContext(ctx)
		if err != nil {
			return streams.BackupResult{}, fmt.Errorf("receive stream snapshot: %w", err)
		}
		if len(chunk.Data) == 0 {
			if status := chunk.Header.Get("Status"); status != "" && status != "204" {
				return streams.BackupResult{}, fmt.Errorf("stream snapshot failed: %s %s", status, chunk.Header.Get("Description"))
			}
			break
		}
		if chunk.Reply != "" {
			if err := chunk.Respond(nil); err != nil {
				return streams.BackupResult{}, fmt.Errorf("acknowledge stream snapshot chunk: %w", err)
			}
		}
		written, err := request.Data.Write(chunk.Data)
		if err != nil {
			return streams.BackupResult{}, fmt.Errorf("write stream snapshot data: %w", err)
		}
		if written != len(chunk.Data) {
			return streams.BackupResult{}, io.ErrShortWrite
		}
		// #nosec G115 -- written equals the non-negative length of chunk.Data after the short-write check.
		transferred += uint64(written)
		chunks++
		reportStreamTransfer(progress, streams.TransferProgress{Transferred: transferred, Chunks: chunks})
	}
	if _, err := request.Metadata.Write(metadata); err != nil {
		return streams.BackupResult{}, fmt.Errorf("write stream snapshot metadata: %w", err)
	}
	result := streams.BackupResult{Stream: request.Stream, Messages: snapshotState.Messages, Bytes: snapshotState.Bytes, Consumers: snapshotState.Consumers, Transferred: transferred, Chunks: chunks}
	reportStreamTransfer(progress, streams.TransferProgress{Transferred: transferred, Total: transferred, Chunks: chunks})
	return result, nil
}

func (m *Monitor) RestoreSnapshot(ctx context.Context, request streams.RestoreRequest, progress streams.TransferProgressFunc) (streams.RestoreResult, error) {
	metadata, err := io.ReadAll(io.LimitReader(request.Metadata, maxSnapshotMetadataSize+1))
	if err != nil {
		return streams.RestoreResult{}, fmt.Errorf("read stream snapshot metadata: %w", err)
	}
	if len(metadata) > maxSnapshotMetadataSize {
		return streams.RestoreResult{}, fmt.Errorf("stream snapshot metadata exceeds %d bytes", maxSnapshotMetadataSize)
	}
	var manifest struct {
		Config struct {
			Name    string `json:"name"`
			Storage string `json:"storage"`
		} `json:"config"`
	}
	if err := json.Unmarshal(metadata, &manifest); err != nil {
		return streams.RestoreResult{}, fmt.Errorf("decode stream snapshot metadata: %w", err)
	}
	if manifest.Config.Name != request.Stream {
		return streams.RestoreResult{}, fmt.Errorf("snapshot belongs to stream %q, not %q", manifest.Config.Name, request.Stream)
	}
	if strings.EqualFold(manifest.Config.Storage, "memory") {
		return streams.RestoreResult{}, fmt.Errorf("memory streams do not support snapshot restore")
	}
	message, err := m.connection.RequestWithContext(ctx, fmt.Sprintf("$JS.API.STREAM.RESTORE.%s", request.Stream), metadata)
	if err != nil {
		return streams.RestoreResult{}, fmt.Errorf("request stream restore: %w", err)
	}
	var response restoreResponseWire
	if err := json.Unmarshal(message.Data, &response); err != nil {
		return streams.RestoreResult{}, fmt.Errorf("decode stream restore response: %w", err)
	}
	if err := jetStreamAPIError(response.Error); err != nil {
		return streams.RestoreResult{}, fmt.Errorf("restore stream %q: %w", request.Stream, err)
	}
	if response.DeliverSubject == "" {
		return streams.RestoreResult{}, fmt.Errorf("stream restore response omitted delivery subject")
	}

	buffer := make([]byte, request.ChunkSize)
	var transferred uint64
	var chunks uint32
	for {
		if err := ctx.Err(); err != nil {
			return streams.RestoreResult{}, err
		}
		read, readErr := request.Data.Read(buffer)
		if read > 0 {
			ack, err := m.connection.RequestWithContext(ctx, response.DeliverSubject, buffer[:read])
			if err != nil {
				return streams.RestoreResult{}, fmt.Errorf("send stream restore chunk: %w", err)
			}
			if err := restoreChunkError(ack); err != nil {
				return streams.RestoreResult{}, err
			}
			transferred += uint64(read)
			chunks++
			reportStreamTransfer(progress, streams.TransferProgress{Transferred: transferred, Total: request.DataSize, Chunks: chunks})
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return streams.RestoreResult{}, fmt.Errorf("read stream snapshot data: %w", readErr)
		}
	}
	final, err := m.connection.RequestWithContext(ctx, response.DeliverSubject, nil)
	if err != nil {
		return streams.RestoreResult{}, fmt.Errorf("finalize stream restore: %w", err)
	}
	var created streamCreateResponseWire
	if err := json.Unmarshal(final.Data, &created); err != nil {
		return streams.RestoreResult{}, fmt.Errorf("decode final stream restore response: %w", err)
	}
	if err := jetStreamAPIError(created.Error); err != nil {
		return streams.RestoreResult{}, fmt.Errorf("finalize stream restore: %w", err)
	}
	result := streams.RestoreResult{Stream: request.Stream, Messages: created.State.Messages, Bytes: created.State.Bytes, Consumers: created.State.Consumers, Transferred: transferred, Chunks: chunks}
	reportStreamTransfer(progress, streams.TransferProgress{Transferred: transferred, Total: transferred, Chunks: chunks})
	return result, nil
}

func jetStreamAPIError(value *apiErrorWire) error {
	if value == nil {
		return nil
	}
	return fmt.Errorf("server rejected request (%d/%d): %s", value.Code, value.ErrCode, value.Description)
}

func restoreChunkError(message *nats.Msg) error {
	if message == nil || len(message.Data) == 0 {
		return nil
	}
	var response apiResponse
	if err := json.Unmarshal(message.Data, &response); err != nil {
		return fmt.Errorf("restore chunk rejected: %s", strings.TrimSpace(string(message.Data)))
	}
	if err := jetStreamAPIError(response.Error); err != nil {
		return fmt.Errorf("restore chunk rejected: %w", err)
	}
	return nil
}

func reportStreamTransfer(progress streams.TransferProgressFunc, value streams.TransferProgress) {
	if progress != nil {
		progress(value)
	}
}
