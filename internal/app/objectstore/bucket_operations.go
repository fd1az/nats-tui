package objectstore

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Storage string

const (
	StorageFile   Storage = "FILE"
	StorageMemory Storage = "MEMORY"
)

// Definition describes a new Object Store bucket.
type Definition struct {
	Name        string
	Description string
	Storage     Storage
	TTL         time.Duration
	MaxBytes    int64
	Replicas    int
	Compression bool
	Metadata    map[string]string
	Mirror      string
	Sources     []string
}

var objectBucketNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Validate checks constraints shared by the CLI, TUI, and adapters.
func (d Definition) Validate() error {
	if !objectBucketNamePattern.MatchString(strings.TrimSpace(d.Name)) {
		return fmt.Errorf("bucket name must contain only letters, digits, dashes, or underscores")
	}
	if d.Storage != StorageFile && d.Storage != StorageMemory {
		return fmt.Errorf("storage must be FILE or MEMORY")
	}
	if d.TTL < 0 {
		return fmt.Errorf("TTL cannot be negative")
	}
	if d.MaxBytes < -1 {
		return fmt.Errorf("max bytes must be -1 or greater")
	}
	if d.Replicas < 1 || d.Replicas > 5 {
		return fmt.Errorf("replicas must be between 1 and 5")
	}
	if strings.TrimSpace(d.Mirror) != "" && len(d.Sources) > 0 {
		return fmt.Errorf("mirror and sources are mutually exclusive")
	}
	if d.Mirror != "" {
		mirror := strings.TrimSpace(d.Mirror)
		if !objectBucketNamePattern.MatchString(mirror) {
			return fmt.Errorf("mirror bucket name is invalid")
		}
		if mirror == strings.TrimSpace(d.Name) {
			return fmt.Errorf("bucket cannot mirror itself")
		}
	}
	seenSources := make(map[string]struct{}, len(d.Sources))
	for _, rawSource := range d.Sources {
		source := strings.TrimSpace(rawSource)
		if !objectBucketNamePattern.MatchString(source) {
			return fmt.Errorf("source bucket name %q is invalid", rawSource)
		}
		if source == strings.TrimSpace(d.Name) {
			return fmt.Errorf("bucket cannot source itself")
		}
		if _, exists := seenSources[source]; exists {
			return fmt.Errorf("source bucket %q is duplicated", source)
		}
		seenSources[source] = struct{}{}
	}
	return nil
}

// Changes contains optional mutable Object Store bucket fields.
type Changes struct {
	Description *string
	TTL         *time.Duration
	MaxBytes    *int64
	Replicas    *int
	Compression *bool
	Metadata    *map[string]string
}

// Validate checks every field present in the patch.
func (c Changes) Validate() error {
	definition := Definition{Name: "VALID", Storage: StorageFile, MaxBytes: -1, Replicas: 1}
	if c.TTL != nil {
		definition.TTL = *c.TTL
	}
	if c.MaxBytes != nil {
		definition.MaxBytes = *c.MaxBytes
	}
	if c.Replicas != nil {
		definition.Replicas = *c.Replicas
	}
	return definition.Validate()
}

type BucketCreator interface {
	CreateObjectStoreBucket(context.Context, Definition) error
}

type BucketUpdater interface {
	UpdateObjectStoreBucket(context.Context, string, Changes) error
}

type BucketDeleter interface {
	DeleteObjectStoreBucket(context.Context, string) error
}

type CreateBucketHandler struct{ creator BucketCreator }

func NewCreateBucketHandler(creator BucketCreator) CreateBucketHandler {
	if creator == nil {
		panic("nil object-store bucket creator")
	}
	return CreateBucketHandler{creator: creator}
}

func (h CreateBucketHandler) Handle(ctx context.Context, definition Definition) error {
	definition.Name = strings.TrimSpace(definition.Name)
	definition.Mirror = strings.TrimSpace(definition.Mirror)
	for index := range definition.Sources {
		definition.Sources[index] = strings.TrimSpace(definition.Sources[index])
	}
	if err := definition.Validate(); err != nil {
		return err
	}
	return h.creator.CreateObjectStoreBucket(ctx, definition)
}

type UpdateBucketHandler struct{ updater BucketUpdater }

func NewUpdateBucketHandler(updater BucketUpdater) UpdateBucketHandler {
	if updater == nil {
		panic("nil object-store bucket updater")
	}
	return UpdateBucketHandler{updater: updater}
}

func (h UpdateBucketHandler) Handle(ctx context.Context, name string, changes Changes) error {
	name = strings.TrimSpace(name)
	if !objectBucketNamePattern.MatchString(name) {
		return fmt.Errorf("bucket name must contain only letters, digits, dashes, or underscores")
	}
	if err := changes.Validate(); err != nil {
		return err
	}
	return h.updater.UpdateObjectStoreBucket(ctx, name, changes)
}

type DeleteBucketHandler struct{ deleter BucketDeleter }

func NewDeleteBucketHandler(deleter BucketDeleter) DeleteBucketHandler {
	if deleter == nil {
		panic("nil object-store bucket deleter")
	}
	return DeleteBucketHandler{deleter: deleter}
}

func (h DeleteBucketHandler) Handle(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if !objectBucketNamePattern.MatchString(name) {
		return fmt.Errorf("bucket name must contain only letters, digits, dashes, or underscores")
	}
	return h.deleter.DeleteObjectStoreBucket(ctx, name)
}
