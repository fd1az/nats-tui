package kvstore

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type catalogStub struct {
	buckets []BucketSummary
	err     error
}

func (s catalogStub) ListKeyValueBuckets(context.Context) ([]BucketSummary, error) {
	return s.buckets, s.err
}

func TestListBucketsReturnsCatalogResult(t *testing.T) {
	t.Parallel()

	want := []BucketSummary{{Name: "SESSIONS", Values: 12, Bytes: 1024, History: 5, TTL: time.Hour, Storage: StorageFile, Replicas: 3}}
	got, err := NewListBucketsHandler(catalogStub{buckets: want}).Handle(context.Background())
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Handle() = %#v, want %#v", got, want)
	}
}

func TestListBucketsPreservesCatalogError(t *testing.T) {
	t.Parallel()

	want := errors.New("catalog unavailable")
	_, got := NewListBucketsHandler(catalogStub{err: want}).Handle(context.Background())
	if !errors.Is(got, want) {
		t.Fatalf("Handle() error = %v, want %v", got, want)
	}
}
