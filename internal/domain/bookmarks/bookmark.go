package bookmarks

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Kind string

const (
	KindStream       Kind = "stream"
	KindConsumer     Kind = "consumer"
	KindSubject      Kind = "subject"
	KindKVBucket     Kind = "kv-bucket"
	KindKVKey        Kind = "kv-key"
	KindObjectBucket Kind = "object-bucket"
	KindObject       Kind = "object"
)

func ParseKind(value string) (Kind, error) {
	kind := Kind(strings.ToLower(strings.TrimSpace(value)))
	switch kind {
	case KindStream, KindConsumer, KindSubject, KindKVBucket, KindKVKey, KindObjectBucket, KindObject:
		return kind, nil
	default:
		return "", fmt.Errorf("unknown bookmark kind %q", value)
	}
}

type Target struct {
	kind               Kind
	primary, secondary string
}

func NewTarget(kind Kind, primary, secondary string) (Target, error) {
	primary, secondary = strings.TrimSpace(primary), strings.TrimSpace(secondary)
	if _, err := ParseKind(string(kind)); err != nil {
		return Target{}, err
	}
	if primary == "" {
		return Target{}, errors.New("bookmark primary identity is required")
	}
	needsSecondary := kind == KindConsumer || kind == KindKVKey || kind == KindObject
	if needsSecondary && secondary == "" {
		return Target{}, fmt.Errorf("%s bookmark requires primary and secondary identities", kind)
	}
	if !needsSecondary && secondary != "" {
		return Target{}, fmt.Errorf("%s bookmark does not accept a secondary identity", kind)
	}
	return Target{kind: kind, primary: primary, secondary: secondary}, nil
}
func (t Target) Kind() Kind        { return t.kind }
func (t Target) Primary() string   { return t.primary }
func (t Target) Secondary() string { return t.secondary }
func (t Target) Key() string       { return string(t.kind) + "\x00" + t.primary + "\x00" + t.secondary }
func (t Target) IsZero() bool      { return t.kind == "" }

type ID struct{ value string }

func NewID(value string) (ID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ID{}, errors.New("bookmark ID is required")
	}
	return ID{value}, nil
}
func (id ID) String() string { return id.value }

type Bookmark struct {
	id        ID
	target    Target
	label     string
	createdAt time.Time
}

func NewBookmark(idValue string, target Target, label string, createdAt time.Time) (*Bookmark, error) {
	id, err := NewID(idValue)
	if err != nil {
		return nil, err
	}
	if target.IsZero() || createdAt.IsZero() {
		return nil, errors.New("bookmark target and creation time are required")
	}
	return &Bookmark{id: id, target: target, label: strings.TrimSpace(label), createdAt: createdAt}, nil
}
func (b *Bookmark) Retarget(target Target) error {
	if target.IsZero() {
		return errors.New("bookmark target is required")
	}
	if target.kind != b.target.kind {
		return errors.New("bookmark cannot change resource kind")
	}
	b.target = target
	return nil
}
func (b Bookmark) ID() ID               { return b.id }
func (b Bookmark) Target() Target       { return b.target }
func (b Bookmark) Label() string        { return b.label }
func (b Bookmark) CreatedAt() time.Time { return b.createdAt }
