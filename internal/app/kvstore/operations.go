package kvstore

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var bucketNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Definition describes a KV bucket to create.
type Definition struct {
	Name         string
	Description  string
	Storage      Storage
	History      int
	TTL          time.Duration
	MaxBytes     int64
	MaxValueSize int32
	Replicas     int
	Compression  bool
	Mirror       string
	Sources      []string
}

// Validate checks constraints shared by every input port.
func (d Definition) Validate() error {
	if !bucketNamePattern.MatchString(strings.TrimSpace(d.Name)) {
		return fmt.Errorf("bucket name must contain only letters, digits, dashes, or underscores")
	}
	if d.Storage != StorageFile && d.Storage != StorageMemory {
		return fmt.Errorf("storage must be FILE or MEMORY")
	}
	if d.History < 1 || d.History > 64 {
		return fmt.Errorf("history must be between 1 and 64")
	}
	if d.TTL < 0 {
		return fmt.Errorf("TTL cannot be negative")
	}
	if d.MaxBytes < -1 {
		return fmt.Errorf("max bytes must be -1 or greater")
	}
	if d.MaxValueSize < -1 {
		return fmt.Errorf("max value size must be -1 or greater")
	}
	if d.Replicas < 1 || d.Replicas > 5 {
		return fmt.Errorf("replicas must be between 1 and 5")
	}
	if strings.TrimSpace(d.Mirror) != "" && len(d.Sources) > 0 {
		return fmt.Errorf("mirror and sources are mutually exclusive")
	}
	if d.Mirror != "" && !bucketNamePattern.MatchString(strings.TrimSpace(d.Mirror)) {
		return fmt.Errorf("mirror bucket name is invalid")
	}
	for _, source := range d.Sources {
		if !bucketNamePattern.MatchString(strings.TrimSpace(source)) {
			return fmt.Errorf("source bucket name %q is invalid", source)
		}
	}
	return nil
}

// BucketCreator creates a KV bucket.
type BucketCreator interface {
	CreateKeyValueBucket(context.Context, Definition) error
}

// Changes contains optional mutable KV bucket configuration fields.
type Changes struct {
	Description  *string
	History      *int
	TTL          *time.Duration
	MaxBytes     *int64
	MaxValueSize *int32
	Replicas     *int
	Compression  *bool
}

// Validate checks every field present in the patch.
func (c Changes) Validate() error {
	definition := Definition{Name: "VALID", Storage: StorageFile, History: 1, MaxBytes: -1, MaxValueSize: -1, Replicas: 1}
	if c.History != nil {
		definition.History = *c.History
	}
	if c.TTL != nil {
		definition.TTL = *c.TTL
	}
	if c.MaxBytes != nil {
		definition.MaxBytes = *c.MaxBytes
	}
	if c.MaxValueSize != nil {
		definition.MaxValueSize = *c.MaxValueSize
	}
	if c.Replicas != nil {
		definition.Replicas = *c.Replicas
	}
	return definition.Validate()
}

// BucketUpdater updates mutable KV bucket configuration.
type BucketUpdater interface {
	UpdateKeyValueBucket(context.Context, string, Changes) error
}

// BucketDeleter permanently deletes a KV bucket and all its values.
type BucketDeleter interface {
	DeleteKeyValueBucket(context.Context, string) error
}

// DeleteBucketHandler validates and deletes KV buckets.
type DeleteBucketHandler struct{ deleter BucketDeleter }

// NewDeleteBucketHandler constructs the delete-bucket use case.
func NewDeleteBucketHandler(deleter BucketDeleter) DeleteBucketHandler {
	if deleter == nil {
		panic("nil key-value bucket deleter")
	}
	return DeleteBucketHandler{deleter: deleter}
}

// Handle permanently deletes a named KV bucket.
func (h DeleteBucketHandler) Handle(ctx context.Context, name string) error {
	if !bucketNamePattern.MatchString(strings.TrimSpace(name)) {
		return fmt.Errorf("bucket name must contain only letters, digits, dashes, or underscores")
	}
	return h.deleter.DeleteKeyValueBucket(ctx, name)
}

// UpdateBucketHandler validates and updates a KV bucket.
type UpdateBucketHandler struct{ updater BucketUpdater }

// NewUpdateBucketHandler constructs the update-bucket use case.
func NewUpdateBucketHandler(updater BucketUpdater) UpdateBucketHandler {
	if updater == nil {
		panic("nil key-value bucket updater")
	}
	return UpdateBucketHandler{updater: updater}
}

// Handle updates a bucket with the requested changes only.
func (h UpdateBucketHandler) Handle(ctx context.Context, name string, changes Changes) error {
	if !bucketNamePattern.MatchString(strings.TrimSpace(name)) {
		return fmt.Errorf("bucket name must contain only letters, digits, dashes, or underscores")
	}
	if err := changes.Validate(); err != nil {
		return err
	}
	return h.updater.UpdateKeyValueBucket(ctx, name, changes)
}

// CreateBucketHandler validates and creates KV buckets.
type CreateBucketHandler struct{ creator BucketCreator }

// NewCreateBucketHandler constructs the create-bucket use case.
func NewCreateBucketHandler(creator BucketCreator) CreateBucketHandler {
	if creator == nil {
		panic("nil key-value bucket creator")
	}
	return CreateBucketHandler{creator: creator}
}

// Handle creates a validated KV bucket.
func (h CreateBucketHandler) Handle(ctx context.Context, definition Definition) error {
	if err := definition.Validate(); err != nil {
		return err
	}
	return h.creator.CreateKeyValueBucket(ctx, definition)
}
