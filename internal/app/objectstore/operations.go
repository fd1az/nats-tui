package objectstore

import (
	"context"
	"fmt"
	"strings"
)

// ObjectRef identifies an object without exposing provider-specific identifiers.
type ObjectRef struct {
	Bucket string
	Name   string
}

// EditObject is a patch: nil fields preserve the current value and non-nil
// empty maps explicitly clear metadata or headers.
type EditObject struct {
	Bucket      string
	Name        string
	NewName     *string
	Description *string
	Metadata    *map[string]string
	Headers     *map[string][]string
}

// UpdateObject contains the complete metadata document expected by storage.
type UpdateObject struct {
	Bucket      string
	CurrentName string
	Name        string
	Description string
	Metadata    map[string]string
	Headers     map[string][]string
}

// ObjectReader retrieves current metadata for patch semantics.
type ObjectReader interface {
	GetObject(context.Context, string, string) (ObjectSummary, error)
}

// ObjectUpdater persists a complete object metadata document.
type ObjectUpdater interface {
	UpdateObject(context.Context, UpdateObject) (ObjectSummary, error)
}

// ObjectDeleter permanently removes an object's chunks and marks it deleted.
type ObjectDeleter interface {
	DeleteObject(context.Context, string, string) error
}

// EditObjectHandler applies a patch without silently discarding existing data.
type EditObjectHandler struct {
	reader  ObjectReader
	updater ObjectUpdater
}

func NewEditObjectHandler(reader ObjectReader, updater ObjectUpdater) EditObjectHandler {
	if reader == nil || updater == nil {
		panic("nil object editor dependency")
	}
	return EditObjectHandler{reader: reader, updater: updater}
}

func (h EditObjectHandler) Handle(ctx context.Context, patch EditObject) (ObjectSummary, error) {
	bucket, name, err := validateObjectRef(patch.Bucket, patch.Name)
	if err != nil {
		return ObjectSummary{}, err
	}
	if patch.NewName != nil && strings.TrimSpace(*patch.NewName) == "" {
		return ObjectSummary{}, fmt.Errorf("new object name is required")
	}
	current, err := h.reader.GetObject(ctx, bucket, name)
	if err != nil {
		return ObjectSummary{}, err
	}
	command := UpdateObject{
		Bucket: bucket, CurrentName: name, Name: current.Name, Description: current.Description,
		Metadata: cloneStringMap(current.Metadata), Headers: cloneStringSliceMap(current.Headers),
	}
	if patch.NewName != nil {
		command.Name = strings.TrimSpace(*patch.NewName)
	}
	if patch.Description != nil {
		command.Description = *patch.Description
	}
	if patch.Metadata != nil {
		command.Metadata = cloneStringMap(*patch.Metadata)
	}
	if patch.Headers != nil {
		command.Headers = cloneStringSliceMap(*patch.Headers)
	}
	return h.updater.UpdateObject(ctx, command)
}

type DeleteObjectHandler struct{ deleter ObjectDeleter }

func NewDeleteObjectHandler(deleter ObjectDeleter) DeleteObjectHandler {
	if deleter == nil {
		panic("nil object deleter")
	}
	return DeleteObjectHandler{deleter: deleter}
}

func (h DeleteObjectHandler) Handle(ctx context.Context, bucket, name string) error {
	bucket, name, err := validateObjectRef(bucket, name)
	if err != nil {
		return err
	}
	return h.deleter.DeleteObject(ctx, bucket, name)
}

func validateObjectRef(bucket, name string) (string, string, error) {
	bucket, name = strings.TrimSpace(bucket), strings.TrimSpace(name)
	if bucket == "" {
		return "", "", fmt.Errorf("object-store bucket is required")
	}
	if name == "" {
		return "", "", fmt.Errorf("object name is required")
	}
	return bucket, name, nil
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneStringSliceMap(values map[string][]string) map[string][]string {
	if values == nil {
		return nil
	}
	result := make(map[string][]string, len(values))
	for key, value := range values {
		result[key] = append([]string(nil), value...)
	}
	return result
}
