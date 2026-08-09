package objectstore

import (
	"context"
	"fmt"
	"strings"
)

// LinkTarget identifies an object or, when Name is empty, an entire bucket.
type LinkTarget struct {
	Bucket string
	Name   string
}

// LinkDefinition describes a link object and its target.
type LinkDefinition struct {
	Bucket       string
	Name         string
	TargetBucket string
	TargetObject string
}

// IsBucketLink reports whether the definition targets an entire bucket.
func (d LinkDefinition) IsBucketLink() bool { return d.TargetObject == "" }

func (d LinkDefinition) normalized() LinkDefinition {
	d.Bucket = strings.TrimSpace(d.Bucket)
	d.Name = strings.TrimSpace(d.Name)
	d.TargetBucket = strings.TrimSpace(d.TargetBucket)
	d.TargetObject = strings.TrimSpace(d.TargetObject)
	return d
}

func (d LinkDefinition) validate() error {
	if d.Bucket == "" {
		return fmt.Errorf("destination object-store bucket is required")
	}
	if d.Name == "" {
		return fmt.Errorf("link name is required")
	}
	if d.TargetBucket == "" {
		return fmt.Errorf("target object-store bucket is required")
	}
	return nil
}

// LinkCreator creates object or bucket links in an Object Store.
type LinkCreator interface {
	CreateObjectLink(context.Context, LinkDefinition) (ObjectSummary, error)
}

// CreateLinkHandler validates and creates an Object Store link.
type CreateLinkHandler struct{ creator LinkCreator }

// NewCreateLinkHandler constructs the link creation use case.
func NewCreateLinkHandler(creator LinkCreator) CreateLinkHandler {
	if creator == nil {
		panic("nil object link creator")
	}
	return CreateLinkHandler{creator: creator}
}

// Handle creates a normalized object or bucket link.
func (h CreateLinkHandler) Handle(ctx context.Context, definition LinkDefinition) (ObjectSummary, error) {
	definition = definition.normalized()
	if err := definition.validate(); err != nil {
		return ObjectSummary{}, err
	}
	return h.creator.CreateObjectLink(ctx, definition)
}
