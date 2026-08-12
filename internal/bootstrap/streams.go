package bootstrap

import (
	"context"
	"fmt"

	"github.com/fd1az/nats-tui/internal/adapters/localfiles"
	"github.com/fd1az/nats-tui/internal/app/streams"
	"github.com/fd1az/nats-tui/internal/config"
)

func (r Runner) ListStreamCatalog(ctx context.Context, settings config.Settings) ([]streams.Summary, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return nil, err
	}
	defer monitor.Close()
	queryCtx, cancel := r.withTimeout(ctx, refreshTimeout)
	defer cancel()
	return streams.NewListStreamsHandler(monitor).Handle(queryCtx)
}

func (r Runner) OperateStreamConfig(ctx context.Context, settings config.Settings, command streams.Command) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return streams.NewOperateHandler(monitor).Handle(actionCtx, command)
}

func (r Runner) PurgeStreamMessages(ctx context.Context, settings config.Settings, command streams.PurgeCommand) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return streams.NewMaintenanceHandler(monitor).Purge(actionCtx, command)
}

func (r Runner) DeleteStoredStreamMessage(ctx context.Context, settings config.Settings, command streams.MessageDeleteCommand) error {
	monitor, err := r.connector()(settings)
	if err != nil {
		return err
	}
	defer monitor.Close()
	actionCtx, cancel := r.withTimeout(ctx, streamActionTimeout)
	defer cancel()
	return streams.NewMaintenanceHandler(monitor).DeleteMessage(actionCtx, command)
}

func (r Runner) BackupStream(ctx context.Context, settings config.Settings, command streams.BackupFile, progress streams.TransferProgressFunc) (streams.BackupResult, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return streams.BackupResult{}, err
	}
	defer monitor.Close()
	gateway, ok := monitor.(streams.SnapshotGateway)
	if !ok {
		return streams.BackupResult{}, fmt.Errorf("NATS session does not support stream snapshots")
	}
	archive := r.streamArchives
	if archive == nil {
		value := localfiles.NewOSStreamArchive()
		archive = &value
	}
	return archive.Backup(ctx, gateway, command, progress)
}

func (r Runner) RestoreStream(ctx context.Context, settings config.Settings, command streams.RestoreFile, progress streams.TransferProgressFunc) (streams.RestoreResult, error) {
	monitor, err := r.connector()(settings)
	if err != nil {
		return streams.RestoreResult{}, err
	}
	defer monitor.Close()
	gateway, ok := monitor.(streams.SnapshotGateway)
	if !ok {
		return streams.RestoreResult{}, fmt.Errorf("NATS session does not support stream snapshots")
	}
	archive := r.streamArchives
	if archive == nil {
		value := localfiles.NewOSStreamArchive()
		archive = &value
	}
	return archive.Restore(ctx, gateway, command, progress)
}
