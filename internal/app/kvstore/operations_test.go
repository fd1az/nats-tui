package kvstore

import (
	"context"
	"errors"
	"testing"
	"time"
)

type creatorStub struct {
	definition Definition
	err        error
}

type updaterStub struct {
	name    string
	changes Changes
	err     error
}

type deleterStub struct {
	name string
	err  error
}

func (s *deleterStub) DeleteKeyValueBucket(_ context.Context, name string) error {
	s.name = name
	return s.err
}

func (s *updaterStub) UpdateKeyValueBucket(_ context.Context, name string, changes Changes) error {
	s.name, s.changes = name, changes
	return s.err
}

func (s *creatorStub) CreateKeyValueBucket(_ context.Context, definition Definition) error {
	s.definition = definition
	return s.err
}

func TestDefinitionValidatesBucketConstraints(t *testing.T) {
	t.Parallel()

	valid := Definition{Name: "SESSIONS", Storage: StorageFile, History: 5, TTL: time.Hour, MaxBytes: -1, MaxValueSize: -1, Replicas: 3}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid definition rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Definition)
	}{
		{name: "missing name", mutate: func(d *Definition) { d.Name = "" }},
		{name: "invalid name", mutate: func(d *Definition) { d.Name = "bad.bucket" }},
		{name: "history zero", mutate: func(d *Definition) { d.History = 0 }},
		{name: "history above 64", mutate: func(d *Definition) { d.History = 65 }},
		{name: "negative ttl", mutate: func(d *Definition) { d.TTL = -time.Second }},
		{name: "invalid replicas", mutate: func(d *Definition) { d.Replicas = 6 }},
		{name: "mirror and sources", mutate: func(d *Definition) { d.Mirror = "PRIMARY"; d.Sources = []string{"SECONDARY"} }},
		{name: "invalid mirror", mutate: func(d *Definition) { d.Mirror = "bad.bucket" }},
		{name: "invalid source", mutate: func(d *Definition) { d.Sources = []string{"bad.bucket"} }},
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

func TestCreateBucketHandlerValidatesBeforeCallingPort(t *testing.T) {
	t.Parallel()

	creator := &creatorStub{}
	handler := NewCreateBucketHandler(creator)
	invalid := Definition{Name: "bad.bucket", Storage: StorageFile, History: 1, Replicas: 1, MaxBytes: -1, MaxValueSize: -1}
	if err := handler.Handle(context.Background(), invalid); err == nil {
		t.Fatal("Handle() accepted invalid definition")
	}
	if creator.definition.Name != "" {
		t.Fatal("creator was called for invalid definition")
	}
}

func TestCreateBucketHandlerCallsPortAndPreservesError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("create failed")
	creator := &creatorStub{err: wantErr}
	definition := Definition{Name: "SESSIONS", Storage: StorageMemory, History: 2, Replicas: 1, MaxBytes: -1, MaxValueSize: -1}
	err := NewCreateBucketHandler(creator).Handle(context.Background(), definition)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Handle() error = %v, want %v", err, wantErr)
	}
	if creator.definition.Name != "SESSIONS" {
		t.Fatalf("creator definition = %#v", creator.definition)
	}
}

func TestUpdateBucketHandlerValidatesPatchBeforeCallingPort(t *testing.T) {
	t.Parallel()

	updater := &updaterStub{}
	invalidHistory := 65
	err := NewUpdateBucketHandler(updater).Handle(context.Background(), "SESSIONS", Changes{History: &invalidHistory})
	if err == nil {
		t.Fatal("invalid changes accepted")
	}
	if updater.name != "" {
		t.Fatal("updater called for invalid changes")
	}
}

func TestUpdateBucketHandlerPassesOnlyRequestedChanges(t *testing.T) {
	t.Parallel()

	updater := &updaterStub{}
	description := "rotating sessions"
	replicas := 3
	changes := Changes{Description: &description, Replicas: &replicas}
	if err := NewUpdateBucketHandler(updater).Handle(context.Background(), "SESSIONS", changes); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if updater.name != "SESSIONS" || updater.changes.Description != changes.Description || updater.changes.Replicas != changes.Replicas {
		t.Fatalf("update = %q %#v", updater.name, updater.changes)
	}
}

func TestDeleteBucketHandlerValidatesNameAndCallsPort(t *testing.T) {
	t.Parallel()

	deleter := &deleterStub{}
	handler := NewDeleteBucketHandler(deleter)
	if err := handler.Handle(context.Background(), "bad.bucket"); err == nil {
		t.Fatal("invalid bucket name accepted")
	}
	if deleter.name != "" {
		t.Fatal("deleter called for invalid name")
	}
	if err := handler.Handle(context.Background(), "SESSIONS"); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if deleter.name != "SESSIONS" {
		t.Fatalf("deleted name = %q", deleter.name)
	}
}
