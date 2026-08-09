package objectstore

import (
	"context"
	"testing"
	"time"
)

type bucketOperatorStub struct {
	definition Definition
	name       string
	changes    Changes
	deleted    string
}

func (s *bucketOperatorStub) CreateObjectStoreBucket(_ context.Context, definition Definition) error {
	s.definition = definition
	return nil
}
func (s *bucketOperatorStub) UpdateObjectStoreBucket(_ context.Context, name string, changes Changes) error {
	s.name, s.changes = name, changes
	return nil
}
func (s *bucketOperatorStub) DeleteObjectStoreBucket(_ context.Context, name string) error {
	s.deleted = name
	return nil
}

func validObjectStoreDefinition() Definition {
	return Definition{Name: "ARTIFACTS", Storage: StorageFile, TTL: time.Hour, MaxBytes: -1, Replicas: 1}
}

func TestObjectStoreDefinitionValidatesOperationalConstraints(t *testing.T) {
	t.Parallel()
	valid := validObjectStoreDefinition()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid definition rejected: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*Definition)
	}{
		{name: "invalid name", mutate: func(d *Definition) { d.Name = "bad.bucket" }},
		{name: "invalid storage", mutate: func(d *Definition) { d.Storage = "DISK" }},
		{name: "negative ttl", mutate: func(d *Definition) { d.TTL = -time.Second }},
		{name: "invalid max bytes", mutate: func(d *Definition) { d.MaxBytes = -2 }},
		{name: "zero replicas", mutate: func(d *Definition) { d.Replicas = 0 }},
		{name: "too many replicas", mutate: func(d *Definition) { d.Replicas = 6 }},
		{name: "mirror and sources", mutate: func(d *Definition) { d.Mirror = "PRIMARY"; d.Sources = []string{"SECONDARY"} }},
		{name: "invalid mirror", mutate: func(d *Definition) { d.Mirror = "bad.bucket" }},
		{name: "invalid source", mutate: func(d *Definition) { d.Sources = []string{"bad.bucket"} }},
		{name: "mirror self", mutate: func(d *Definition) { d.Mirror = "ARTIFACTS" }},
		{name: "source self", mutate: func(d *Definition) { d.Sources = []string{"ARTIFACTS"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := valid
			test.mutate(&definition)
			if err := definition.Validate(); err == nil {
				t.Fatal("invalid definition accepted")
			}
		})
	}
}

func TestObjectStoreDefinitionAcceptsMirrorOrMultipleSources(t *testing.T) {
	t.Parallel()
	for _, definition := range []Definition{
		{Name: "MIRROR", Storage: StorageFile, MaxBytes: -1, Replicas: 1, Mirror: "PRIMARY"},
		{Name: "AGGREGATE", Storage: StorageFile, MaxBytes: -1, Replicas: 1, Sources: []string{"REGION_A", "REGION_B"}},
	} {
		if err := definition.Validate(); err != nil {
			t.Fatalf("valid replication definition rejected: %#v: %v", definition, err)
		}
	}
}

func TestCreateObjectStoreBucketValidatesBeforeCallingPort(t *testing.T) {
	t.Parallel()
	operator := &bucketOperatorStub{}
	definition := validObjectStoreDefinition()
	definition.Name = "bad.bucket"
	if err := NewCreateBucketHandler(operator).Handle(context.Background(), definition); err == nil {
		t.Fatal("invalid definition accepted")
	}
	if operator.definition.Name != "" {
		t.Fatal("creator called for invalid definition")
	}
}

func TestUpdateObjectStoreBucketValidatesPatchAndPreservesOptionality(t *testing.T) {
	t.Parallel()
	operator := &bucketOperatorStub{}
	description := "release artifacts"
	metadata := map[string]string{"owner": "platform"}
	changes := Changes{Description: &description, Metadata: &metadata}
	if err := NewUpdateBucketHandler(operator).Handle(context.Background(), " ARTIFACTS ", changes); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if operator.name != "ARTIFACTS" || operator.changes.Description == nil || operator.changes.TTL != nil || (*operator.changes.Metadata)["owner"] != "platform" {
		t.Fatalf("update = %q %#v", operator.name, operator.changes)
	}
	invalidReplicas := 7
	if err := NewUpdateBucketHandler(operator).Handle(context.Background(), "ARTIFACTS", Changes{Replicas: &invalidReplicas}); err == nil {
		t.Fatal("invalid patch accepted")
	}
}

func TestDeleteObjectStoreBucketValidatesName(t *testing.T) {
	t.Parallel()
	operator := &bucketOperatorStub{}
	handler := NewDeleteBucketHandler(operator)
	if err := handler.Handle(context.Background(), "bad.bucket"); err == nil {
		t.Fatal("invalid name accepted")
	}
	if err := handler.Handle(context.Background(), " ARTIFACTS "); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if operator.deleted != "ARTIFACTS" {
		t.Fatalf("deleted = %q", operator.deleted)
	}
}
