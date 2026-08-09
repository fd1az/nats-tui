package alerts

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrAlertIDRequired          = errors.New("alert ID is required")
	ErrAlertTitleRequired       = errors.New("alert title is required")
	ErrAlertSourceRequired      = errors.New("alert source is required")
	ErrAlertTimeRequired        = errors.New("alert occurrence and receipt times are required")
	ErrReceivedBeforeOccurrence = errors.New("alert cannot be received before it occurred")
	ErrAlreadyAcknowledged      = errors.New("alert is already acknowledged")
	ErrInvalidAcknowledgement   = errors.New("acknowledgement cannot precede alert receipt")
)

type ID struct{ value string }

func NewID(value string) (ID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ID{}, ErrAlertIDRequired
	}
	return ID{value: value}, nil
}
func (id ID) String() string { return id.value }

type Severity struct{ value string }

var (
	SeverityInfo     = Severity{"info"}
	SeverityWarning  = Severity{"warning"}
	SeverityCritical = Severity{"critical"}
)

func ParseSeverity(value string) (Severity, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SeverityInfo.value:
		return SeverityInfo, nil
	case SeverityWarning.value:
		return SeverityWarning, nil
	case SeverityCritical.value:
		return SeverityCritical, nil
	default:
		return Severity{}, fmt.Errorf("unknown alert severity %q", value)
	}
}
func (s Severity) String() string { return s.value }
func (s Severity) Rank() int {
	switch s {
	case SeverityInfo:
		return 1
	case SeverityWarning:
		return 2
	case SeverityCritical:
		return 3
	}
	return 0
}
func (s Severity) IsZero() bool { return s.value == "" }

type Category struct{ value string }

var (
	CategoryAudit      = Category{"audit"}
	CategoryStream     = Category{"stream"}
	CategoryConsumer   = Category{"consumer"}
	CategoryServer     = Category{"server"}
	CategoryConnection = Category{"connection"}
	CategoryJetStream  = Category{"jetstream"}
	CategoryUnknown    = Category{"unknown"}
)

func ParseCategory(value string) (Category, error) {
	for _, category := range []Category{CategoryAudit, CategoryStream, CategoryConsumer, CategoryServer, CategoryConnection, CategoryJetStream, CategoryUnknown} {
		if strings.EqualFold(strings.TrimSpace(value), category.value) {
			return category, nil
		}
	}
	return Category{}, fmt.Errorf("unknown alert category %q", value)
}
func (c Category) String() string { return c.value }
func (c Category) IsZero() bool   { return c.value == "" }

type ResourceKind struct{ value string }

var (
	ResourceStream     = ResourceKind{"stream"}
	ResourceConsumer   = ResourceKind{"consumer"}
	ResourceServer     = ResourceKind{"server"}
	ResourceConnection = ResourceKind{"connection"}
)

func ParseResourceKind(value string) (ResourceKind, error) {
	for _, kind := range []ResourceKind{ResourceStream, ResourceConsumer, ResourceServer, ResourceConnection} {
		if strings.EqualFold(strings.TrimSpace(value), kind.value) {
			return kind, nil
		}
	}
	return ResourceKind{}, fmt.Errorf("unknown alert resource kind %q", value)
}
func (k ResourceKind) String() string { return k.value }

type Resource struct {
	kind               ResourceKind
	primary, secondary string
}

func NewResource(kind ResourceKind, primary, secondary string) (Resource, error) {
	primary, secondary = strings.TrimSpace(primary), strings.TrimSpace(secondary)
	if kind.value == "" || primary == "" {
		return Resource{}, errors.New("resource kind and primary identity are required")
	}
	if kind == ResourceConsumer && secondary == "" {
		return Resource{}, errors.New("consumer resource requires stream and consumer identities")
	}
	return Resource{kind: kind, primary: primary, secondary: secondary}, nil
}
func (r Resource) Kind() ResourceKind { return r.kind }
func (r Resource) Primary() string    { return r.primary }
func (r Resource) Secondary() string  { return r.secondary }
func (r Resource) IsZero() bool       { return r.kind.value == "" }

type AlertInput struct {
	ID, Kind, Title, Description, Source string
	Category                             Category
	Severity                             Severity
	Resource                             Resource
	OccurredAt, ReceivedAt               time.Time
	Payload                              []byte
}

type Alert struct {
	id                                     ID
	kind, title, description, source       string
	category                               Category
	severity                               Severity
	resource                               Resource
	occurredAt, receivedAt, acknowledgedAt time.Time
	payload                                []byte
}

func NewAlert(input AlertInput) (*Alert, error) { return RehydrateAlert(input, time.Time{}) }

func RehydrateAlert(input AlertInput, acknowledgedAt time.Time) (*Alert, error) {
	id, err := NewID(input.ID)
	if err != nil {
		return nil, err
	}
	input.Kind, input.Title, input.Description, input.Source = strings.TrimSpace(input.Kind), strings.TrimSpace(input.Title), strings.TrimSpace(input.Description), strings.TrimSpace(input.Source)
	if input.Title == "" {
		return nil, ErrAlertTitleRequired
	}
	if input.Source == "" {
		return nil, ErrAlertSourceRequired
	}
	if input.Category.IsZero() || input.Severity.IsZero() {
		return nil, errors.New("alert category and severity are required")
	}
	if input.OccurredAt.IsZero() || input.ReceivedAt.IsZero() {
		return nil, ErrAlertTimeRequired
	}
	if input.ReceivedAt.Before(input.OccurredAt) {
		return nil, ErrReceivedBeforeOccurrence
	}
	if !acknowledgedAt.IsZero() && acknowledgedAt.Before(input.ReceivedAt) {
		return nil, ErrInvalidAcknowledgement
	}
	return &Alert{id: id, kind: input.Kind, title: input.Title, description: input.Description, source: input.Source, category: input.Category, severity: input.Severity, resource: input.Resource, occurredAt: input.OccurredAt, receivedAt: input.ReceivedAt, acknowledgedAt: acknowledgedAt, payload: append([]byte(nil), input.Payload...)}, nil
}

func (a *Alert) Acknowledge(at time.Time) error {
	if !a.acknowledgedAt.IsZero() {
		return ErrAlreadyAcknowledged
	}
	if at.IsZero() || at.Before(a.receivedAt) {
		return ErrInvalidAcknowledgement
	}
	a.acknowledgedAt = at
	return nil
}

func (a *Alert) Classify(severity Severity) {
	if severity.IsZero() {
		panic("zero alert severity")
	}
	a.severity = severity
}

func (a Alert) ID() ID                    { return a.id }
func (a Alert) Kind() string              { return a.kind }
func (a Alert) Title() string             { return a.title }
func (a Alert) Description() string       { return a.description }
func (a Alert) Source() string            { return a.source }
func (a Alert) Category() Category        { return a.category }
func (a Alert) Severity() Severity        { return a.severity }
func (a Alert) Resource() Resource        { return a.resource }
func (a Alert) OccurredAt() time.Time     { return a.occurredAt }
func (a Alert) ReceivedAt() time.Time     { return a.receivedAt }
func (a Alert) AcknowledgedAt() time.Time { return a.acknowledgedAt }
func (a Alert) Acknowledged() bool        { return !a.acknowledgedAt.IsZero() }
func (a Alert) Payload() []byte           { return append([]byte(nil), a.payload...) }
