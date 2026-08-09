package objectstore

import (
	"context"
	"reflect"
	"testing"
)

type catalogStub struct{ buckets []BucketSummary }

func (s catalogStub) ListObjectStoreBuckets(context.Context) ([]BucketSummary, error) {
	return s.buckets, nil
}

func TestListBucketsHandlerReturnsCatalog(t *testing.T) {
	t.Parallel()
	want := []BucketSummary{{Name: "ARTIFACTS", Size: 4096, Storage: "FILE"}}
	got, err := NewListBucketsHandler(catalogStub{buckets: want}).Handle(context.Background())
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Handle() = %#v, %v; want %#v", got, err, want)
	}
}
