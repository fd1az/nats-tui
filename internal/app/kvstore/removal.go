package kvstore

import (
	"context"
	"fmt"
	"strings"
)

// RemoveAction distinguishes recoverable deletion from destructive purge.
type RemoveAction string

const (
	RemoveDelete RemoveAction = "DELETE"
	RemovePurge  RemoveAction = "PURGE"
)

// RemoveEntry describes removal of one KV key.
type RemoveEntry struct {
	Bucket           string
	Key              string
	Action           RemoveAction
	ExpectedRevision *uint64
}

// EntryRemover removes a KV key.
type EntryRemover interface {
	RemoveKeyValueEntry(context.Context, RemoveEntry) error
}

// RemoveEntryHandler validates KV key removal.
type RemoveEntryHandler struct{ remover EntryRemover }

// NewRemoveEntryHandler constructs the remove-entry command handler.
func NewRemoveEntryHandler(remover EntryRemover) RemoveEntryHandler {
	if remover == nil {
		panic("nil key-value entry remover")
	}
	return RemoveEntryHandler{remover: remover}
}

// Handle removes one KV key.
func (h RemoveEntryHandler) Handle(ctx context.Context, command RemoveEntry) error {
	command.Bucket = strings.TrimSpace(command.Bucket)
	command.Key = strings.TrimSpace(command.Key)
	if !bucketNamePattern.MatchString(command.Bucket) {
		return fmt.Errorf("bucket name must contain only letters, digits, dashes, or underscores")
	}
	if command.Key == "" {
		return fmt.Errorf("key is required")
	}
	if command.Action != RemoveDelete && command.Action != RemovePurge {
		return fmt.Errorf("remove action must be DELETE or PURGE")
	}
	if command.ExpectedRevision != nil {
		revision := *command.ExpectedRevision
		command.ExpectedRevision = &revision
	}
	return h.remover.RemoveKeyValueEntry(ctx, command)
}
