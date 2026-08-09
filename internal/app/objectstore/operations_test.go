package objectstore

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type objectOperatorStub struct {
	objects   map[string]ObjectSummary
	updated   UpdateObject
	deleted   ObjectRef
	updateErr error
	deleteErr error
}

func (s *objectOperatorStub) GetObject(_ context.Context, bucket, name string) (ObjectSummary, error) {
	object, ok := s.objects[bucket+"/"+name]
	if !ok {
		return ObjectSummary{}, errors.New("not found")
	}
	return object, nil
}

func (s *objectOperatorStub) UpdateObject(_ context.Context, command UpdateObject) (ObjectSummary, error) {
	s.updated = command
	return ObjectSummary{Bucket: command.Bucket, Name: command.Name, Description: command.Description, Metadata: command.Metadata, Headers: command.Headers}, s.updateErr
}

func (s *objectOperatorStub) DeleteObject(_ context.Context, bucket, name string) error {
	s.deleted = ObjectRef{Bucket: bucket, Name: name}
	return s.deleteErr
}

func TestEditObjectHandlerMergesPatchWithoutLosingMetadata(t *testing.T) {
	t.Parallel()
	operator := &objectOperatorStub{objects: map[string]ObjectSummary{
		"ARTIFACTS/release.tar": {
			Bucket: "ARTIFACTS", Name: "release.tar", Description: "old",
			Metadata: map[string]string{"env": "prod"}, Headers: map[string][]string{"Content-Type": {"application/tar"}},
		},
	}}
	name, description := "stable.tar", "stable release"
	result, err := NewEditObjectHandler(operator, operator).Handle(context.Background(), EditObject{
		Bucket: " ARTIFACTS ", Name: " release.tar ", NewName: &name, Description: &description,
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Name != name || operator.updated.Name != name || operator.updated.Description != description {
		t.Fatalf("updated object = %#v", operator.updated)
	}
	if !reflect.DeepEqual(operator.updated.Metadata, map[string]string{"env": "prod"}) ||
		!reflect.DeepEqual(operator.updated.Headers, map[string][]string{"Content-Type": {"application/tar"}}) {
		t.Fatalf("metadata was not preserved: %#v", operator.updated)
	}
}

func TestEditObjectHandlerCanClearMapsAndDefensivelyCopiesThem(t *testing.T) {
	t.Parallel()
	operator := &objectOperatorStub{objects: map[string]ObjectSummary{
		"B/item": {Bucket: "B", Name: "item", Metadata: map[string]string{"old": "value"}, Headers: map[string][]string{"X-Old": {"value"}}},
	}}
	metadata := map[string]string{}
	headers := map[string][]string{"X-New": {"one", "two"}}
	_, err := NewEditObjectHandler(operator, operator).Handle(context.Background(), EditObject{Bucket: "B", Name: "item", Metadata: &metadata, Headers: &headers})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	headers["X-New"][0] = "mutated"
	metadata["later"] = "mutation"
	if len(operator.updated.Metadata) != 0 || !reflect.DeepEqual(operator.updated.Headers, map[string][]string{"X-New": {"one", "two"}}) {
		t.Fatalf("mutable command escaped into port: %#v", operator.updated)
	}
}

func TestEditObjectHandlerRejectsInvalidInputBeforeReading(t *testing.T) {
	t.Parallel()
	operator := &objectOperatorStub{}
	empty := " "
	if _, err := NewEditObjectHandler(operator, operator).Handle(context.Background(), EditObject{Bucket: "B", Name: "item", NewName: &empty}); err == nil {
		t.Fatal("Handle() accepted an empty new name")
	}
	if operator.updated.Name != "" {
		t.Fatal("update port called for invalid input")
	}
}

func TestDeleteObjectHandlerValidatesAndCallsPort(t *testing.T) {
	t.Parallel()
	operator := &objectOperatorStub{}
	handler := NewDeleteObjectHandler(operator)
	if err := handler.Handle(context.Background(), "", "item"); err == nil {
		t.Fatal("Handle() accepted an empty bucket")
	}
	if err := handler.Handle(context.Background(), " ARTIFACTS ", " release.tar "); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if operator.deleted != (ObjectRef{Bucket: "ARTIFACTS", Name: "release.tar"}) {
		t.Fatalf("deleted = %#v", operator.deleted)
	}
}
