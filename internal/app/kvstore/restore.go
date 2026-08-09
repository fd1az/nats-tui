package kvstore

import (
	"context"
	"fmt"
)

// RestoreEntry selects a retained PUT revision to republish.
type RestoreEntry struct {
	Bucket   string
	Key      string
	Revision uint64
}

// RestoredEntry reports the new revision and restored value.
type RestoredEntry struct {
	Revision uint64
	Value    []byte
}

// RestoreHandler restores retained values with compare-and-set semantics.
type RestoreHandler struct {
	history HistoryReader
	writer  EntryWriter
}

// NewRestoreHandler constructs the restore command handler.
func NewRestoreHandler(history HistoryReader, writer EntryWriter) RestoreHandler {
	if history == nil || writer == nil {
		panic("nil key-value restore dependency")
	}
	return RestoreHandler{history: history, writer: writer}
}

// Handle republishes a historical PUT value against the current revision.
func (h RestoreHandler) Handle(ctx context.Context, command RestoreEntry) (RestoredEntry, error) {
	entries, err := NewListHistoryHandler(h.history).Handle(ctx, command.Bucket, command.Key)
	if err != nil {
		return RestoredEntry{}, err
	}
	if len(entries) == 0 {
		return RestoredEntry{}, fmt.Errorf("no retained revisions found")
	}
	source, found := findRevision(entries, command.Revision)
	if !found {
		return RestoredEntry{}, fmt.Errorf("revision %d was not found", command.Revision)
	}
	if source.Operation != OperationPut {
		return RestoredEntry{}, fmt.Errorf("revision %d is %s and has no restorable value", command.Revision, source.Operation)
	}
	latest := entries[len(entries)-1].Revision
	revision, err := h.writer.PutKeyValueEntry(ctx, WriteEntry{Bucket: command.Bucket, Key: command.Key, Value: source.Value, ExpectedRevision: &latest})
	if err != nil {
		return RestoredEntry{}, err
	}
	return RestoredEntry{Revision: revision, Value: append([]byte(nil), source.Value...)}, nil
}

func findRevision(entries []Entry, revision uint64) (Entry, bool) {
	for _, entry := range entries {
		if entry.Revision == revision {
			return entry, true
		}
	}
	return Entry{}, false
}
