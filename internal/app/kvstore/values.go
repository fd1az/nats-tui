package kvstore

import (
	"context"
	"fmt"
	"strings"
)

// WriteEntry describes a KV value mutation. A nil expected revision is an
// unconditional put; a non-nil revision enables compare-and-set semantics.
type WriteEntry struct {
	Bucket           string
	Key              string
	Value            []byte
	ExpectedRevision *uint64
}

// EntryWriter persists a KV value mutation.
type EntryWriter interface {
	PutKeyValueEntry(context.Context, WriteEntry) (uint64, error)
}

// PutEntryHandler validates and writes KV values.
type PutEntryHandler struct{ writer EntryWriter }

// NewPutEntryHandler constructs the put-entry command handler.
func NewPutEntryHandler(writer EntryWriter) PutEntryHandler {
	if writer == nil {
		panic("nil key-value entry writer")
	}
	return PutEntryHandler{writer: writer}
}

// Handle writes a value and returns its new revision.
func (h PutEntryHandler) Handle(ctx context.Context, command WriteEntry) (uint64, error) {
	command.Bucket = strings.TrimSpace(command.Bucket)
	command.Key = strings.TrimSpace(command.Key)
	if !bucketNamePattern.MatchString(command.Bucket) {
		return 0, fmt.Errorf("bucket name must contain only letters, digits, dashes, or underscores")
	}
	if command.Key == "" {
		return 0, fmt.Errorf("key is required")
	}
	command.Value = append([]byte(nil), command.Value...)
	if command.ExpectedRevision != nil {
		revision := *command.ExpectedRevision
		command.ExpectedRevision = &revision
	}
	return h.writer.PutKeyValueEntry(ctx, command)
}
