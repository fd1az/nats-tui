package objectstore

import (
	"context"
	"errors"
	"testing"
)

type linkCreatorStub struct {
	definition LinkDefinition
	result     ObjectSummary
	err        error
}

func (s *linkCreatorStub) CreateObjectLink(_ context.Context, definition LinkDefinition) (ObjectSummary, error) {
	s.definition = definition
	return s.result, s.err
}

func TestCreateLinkHandlerNormalizesObjectLink(t *testing.T) {
	t.Parallel()
	creator := &linkCreatorStub{result: ObjectSummary{Name: "latest"}}
	definition := LinkDefinition{Bucket: " ARTIFACTS ", Name: " latest ", TargetBucket: " RELEASES ", TargetObject: " v1.tar "}
	result, err := NewCreateLinkHandler(creator).Handle(context.Background(), definition)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	want := LinkDefinition{Bucket: "ARTIFACTS", Name: "latest", TargetBucket: "RELEASES", TargetObject: "v1.tar"}
	if creator.definition != want || result.Name != "latest" {
		t.Fatalf("definition=%#v result=%#v", creator.definition, result)
	}
}

func TestCreateLinkHandlerAllowsBucketLink(t *testing.T) {
	t.Parallel()
	creator := &linkCreatorStub{}
	if _, err := NewCreateLinkHandler(creator).Handle(context.Background(), LinkDefinition{Bucket: "ROOT", Name: "releases", TargetBucket: "RELEASES"}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !creator.definition.IsBucketLink() {
		t.Fatalf("definition = %#v", creator.definition)
	}
}

func TestCreateLinkHandlerRejectsMissingRequiredNames(t *testing.T) {
	t.Parallel()
	tests := []LinkDefinition{
		{Name: "link", TargetBucket: "TARGET"},
		{Bucket: "ROOT", TargetBucket: "TARGET"},
		{Bucket: "ROOT", Name: "link"},
	}
	for _, definition := range tests {
		creator := &linkCreatorStub{}
		if _, err := NewCreateLinkHandler(creator).Handle(context.Background(), definition); err == nil {
			t.Fatalf("invalid definition accepted: %#v", definition)
		}
		if creator.definition != (LinkDefinition{}) {
			t.Fatal("creator called for invalid definition")
		}
	}
}

func TestCreateLinkHandlerPreservesAdapterError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("target is another link")
	creator := &linkCreatorStub{err: wantErr}
	_, err := NewCreateLinkHandler(creator).Handle(context.Background(), LinkDefinition{Bucket: "ROOT", Name: "latest", TargetBucket: "ROOT", TargetObject: "other"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Handle() error = %v, want %v", err, wantErr)
	}
}
