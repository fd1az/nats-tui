package objectstore

import (
	"context"
	"reflect"
	"testing"
)

type objectCatalogStub struct{ objects []ObjectSummary }

func (s objectCatalogStub) ListObjects(context.Context, string) ([]ObjectSummary, error) {
	return append([]ObjectSummary(nil), s.objects...), nil
}

func TestListObjectsHandlerSortsCatalog(t *testing.T) {
	t.Parallel()
	handler := NewListObjectsHandler(objectCatalogStub{objects: []ObjectSummary{{Name: "zeta"}, {Name: "alpha"}}})
	got, err := handler.Handle(context.Background(), "ARTIFACTS")
	want := []ObjectSummary{{Name: "alpha"}, {Name: "zeta"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Handle() = %#v, %v; want %#v", got, err, want)
	}
}

func TestListObjectsHandlerRejectsEmptyBucket(t *testing.T) {
	t.Parallel()
	if _, err := NewListObjectsHandler(objectCatalogStub{}).Handle(context.Background(), " "); err == nil {
		t.Fatal("Handle() accepted an empty bucket")
	}
}
