package bookmarks

import (
	"testing"
	"time"
)

func TestTargetValidatesIdentityByKind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind               Kind
		primary, secondary string
		valid              bool
	}{{KindStream, "ORDERS", "", true}, {KindConsumer, "ORDERS", "WORKER", true}, {KindConsumer, "ORDERS", "", false}, {KindKVKey, "SESSIONS", "user.42", true}, {KindObject, "ARTIFACTS", "release.tar", true}, {KindSubject, "events.>", "", true}, {KindStream, "", "", false}}
	for _, test := range cases {
		_, err := NewTarget(test.kind, test.primary, test.secondary)
		if (err == nil) != test.valid {
			t.Errorf("kind=%s primary=%q secondary=%q err=%v", test.kind, test.primary, test.secondary, err)
		}
	}
}
func TestBookmarkRetargetPreservesIdentity(t *testing.T) {
	t.Parallel()
	original, _ := NewTarget(KindStream, "ORDERS", "")
	bookmark, err := NewBookmark("b-1", original, "Orders", time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	replacement, _ := NewTarget(KindStream, "ORDERS_V2", "")
	if err := bookmark.Retarget(replacement); err != nil {
		t.Fatal(err)
	}
	if bookmark.ID().String() != "b-1" || bookmark.Target().Primary() != "ORDERS_V2" {
		t.Fatalf("bookmark=%#v", bookmark)
	}
	other, _ := NewTarget(KindKVBucket, "CONFIG", "")
	if err := bookmark.Retarget(other); err == nil {
		t.Fatal("cross-kind retarget succeeded")
	}
}
