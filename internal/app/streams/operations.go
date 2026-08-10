package streams

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Storage string

const (
	StorageFile   Storage = "file"
	StorageMemory Storage = "memory"
)

type Retention string

const (
	RetentionLimits    Retention = "limits"
	RetentionInterest  Retention = "interest"
	RetentionWorkQueue Retention = "workqueue"
)

type Discard string

const (
	DiscardOld Discard = "old"
	DiscardNew Discard = "new"
)

type SubjectTransform struct {
	Source      string
	Destination string
}

type Republish struct {
	Source      string
	Destination string
	HeadersOnly bool
}

type External struct {
	APIPrefix     string
	DeliverPrefix string
}

type Source struct {
	Name              string
	StartSequence     uint64
	StartTime         *time.Time
	FilterSubject     string
	SubjectTransforms []SubjectTransform
	External          *External
}

func (s Source) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("source stream name is required")
	}
	if s.StartSequence > 0 && s.StartTime != nil {
		return fmt.Errorf("source %q cannot set both start sequence and start time", s.Name)
	}
	for _, transform := range s.SubjectTransforms {
		if err := transform.Validate(); err != nil {
			return fmt.Errorf("source %q: %w", s.Name, err)
		}
	}
	if s.External != nil && strings.TrimSpace(s.External.APIPrefix) == "" {
		return fmt.Errorf("source %q external API prefix is required", s.Name)
	}
	return nil
}

func (t SubjectTransform) Validate() error {
	if strings.TrimSpace(t.Destination) == "" {
		return fmt.Errorf("subject transform destination is required")
	}
	return nil
}

type Action uint8

const (
	ActionCreate Action = iota + 1
	ActionUpdate
	ActionDelete
)

func (a Action) String() string {
	switch a {
	case ActionCreate:
		return "create"
	case ActionUpdate:
		return "update"
	case ActionDelete:
		return "delete"
	default:
		return "unknown"
	}
}

type Definition struct {
	Name                  string
	Description           string
	Subjects              []string
	Storage               Storage
	Retention             Retention
	MaxMessages           int64
	MaxBytes              int64
	Replicas              int
	MaxAge                time.Duration
	MaxMessageSize        int32
	MaxMessagesPerSubject int64
	DuplicateWindow       time.Duration
	MaxConsumers          int
	Discard               Discard
	DiscardNewPerSubject  bool
	Compression           bool
	AllowDirect           bool
	MirrorDirect          bool
	AllowRollup           bool
	DenyDelete            bool
	DenyPurge             bool
	Sealed                bool
	NoAck                 bool
	AllowMessageTTL       bool
	PlacementCluster      string
	PlacementTags         []string
	Metadata              map[string]string
	SubjectTransform      *SubjectTransform
	Republish             *Republish
	Mirror                *Source
	Sources               []Source
}

func (d Definition) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("stream name is required")
	}
	if strings.ContainsAny(d.Name, " \t\r\n.*>/\\") {
		return fmt.Errorf("stream name contains unsupported characters")
	}
	if len(d.Subjects) == 0 && d.Mirror == nil && len(d.Sources) == 0 {
		return fmt.Errorf("at least one subject, mirror, or source is required")
	}
	if d.Mirror != nil && len(d.Subjects) > 0 {
		return fmt.Errorf("mirror streams cannot define subjects")
	}
	if d.Mirror != nil && len(d.Sources) > 0 {
		return fmt.Errorf("mirror and sources are mutually exclusive")
	}
	for _, subject := range d.Subjects {
		if strings.TrimSpace(subject) == "" {
			return fmt.Errorf("subjects cannot be empty")
		}
		if strings.ContainsAny(subject, " \t\r\n") {
			return fmt.Errorf("subjects cannot contain whitespace")
		}
	}
	if d.Storage != StorageFile && d.Storage != StorageMemory {
		return fmt.Errorf("unsupported storage %q", d.Storage)
	}
	if d.Retention != RetentionLimits && d.Retention != RetentionInterest && d.Retention != RetentionWorkQueue {
		return fmt.Errorf("unsupported retention %q", d.Retention)
	}
	if d.MaxMessages < -1 || d.MaxBytes < -1 {
		return fmt.Errorf("limits must be -1 or greater")
	}
	if d.MaxMessagesPerSubject < -1 || d.MaxMessageSize < -1 || d.MaxConsumers < -1 {
		return fmt.Errorf("advanced limits must be -1 or greater")
	}
	if d.MaxAge < 0 || d.DuplicateWindow < 0 {
		return fmt.Errorf("durations cannot be negative")
	}
	if d.Discard != "" && d.Discard != DiscardOld && d.Discard != DiscardNew {
		return fmt.Errorf("unsupported discard policy %q", d.Discard)
	}
	if d.DiscardNewPerSubject && (d.Discard != DiscardNew || d.MaxMessagesPerSubject < 1) {
		return fmt.Errorf("discard new per subject requires discard=new and a positive per-subject limit")
	}
	if d.Compression && d.Storage != StorageFile {
		return fmt.Errorf("compression requires file storage")
	}
	if d.MirrorDirect && d.Mirror == nil {
		return fmt.Errorf("mirror direct requires a mirror stream")
	}
	if d.Replicas < 1 || d.Replicas > 5 {
		return fmt.Errorf("replicas must be between 1 and 5")
	}
	if d.SubjectTransform != nil {
		if err := d.SubjectTransform.Validate(); err != nil {
			return err
		}
	}
	if d.Republish != nil && strings.TrimSpace(d.Republish.Destination) == "" {
		return fmt.Errorf("republish destination is required")
	}
	if d.Mirror != nil {
		if err := d.Mirror.Validate(); err != nil {
			return fmt.Errorf("mirror: %w", err)
		}
	}
	seenSources := make(map[string]struct{}, len(d.Sources))
	for _, source := range d.Sources {
		if err := source.Validate(); err != nil {
			return err
		}
		if _, exists := seenSources[source.Name]; exists {
			return fmt.Errorf("duplicate source stream %q", source.Name)
		}
		seenSources[source.Name] = struct{}{}
	}
	return nil
}

type PurgeCommand struct {
	Stream   string
	Subject  string
	Sequence uint64
	Keep     uint64
}

func (c PurgeCommand) Validate() error {
	if strings.TrimSpace(c.Stream) == "" {
		return fmt.Errorf("stream name is required")
	}
	if c.Sequence > 0 && c.Keep > 0 {
		return fmt.Errorf("sequence and keep are mutually exclusive")
	}
	return nil
}

type MessageDeleteCommand struct {
	Stream   string
	Sequence uint64
	Secure   bool
}

func (c MessageDeleteCommand) Validate() error {
	if strings.TrimSpace(c.Stream) == "" || c.Sequence == 0 {
		return fmt.Errorf("stream name and positive sequence are required")
	}
	return nil
}

type Command struct {
	Action     Action
	Name       string
	Definition Definition
}

type Operator interface {
	OperateStream(context.Context, Command) error
}

type Maintenance interface {
	PurgeStream(context.Context, PurgeCommand) error
	DeleteStreamMessage(context.Context, MessageDeleteCommand) error
}

type OperateHandler struct{ operator Operator }

func NewOperateHandler(operator Operator) OperateHandler {
	if operator == nil {
		panic("nil stream operator")
	}
	return OperateHandler{operator: operator}
}

type MaintenanceHandler struct{ maintenance Maintenance }

func NewMaintenanceHandler(maintenance Maintenance) MaintenanceHandler {
	if maintenance == nil {
		panic("nil stream maintenance")
	}
	return MaintenanceHandler{maintenance: maintenance}
}

func (h MaintenanceHandler) Purge(ctx context.Context, command PurgeCommand) error {
	if err := command.Validate(); err != nil {
		return err
	}
	return h.maintenance.PurgeStream(ctx, command)
}

func (h MaintenanceHandler) DeleteMessage(ctx context.Context, command MessageDeleteCommand) error {
	if err := command.Validate(); err != nil {
		return err
	}
	return h.maintenance.DeleteStreamMessage(ctx, command)
}

func (h OperateHandler) Handle(ctx context.Context, command Command) error {
	if command.Action == ActionCreate || command.Action == ActionUpdate {
		if err := command.Definition.Validate(); err != nil {
			return err
		}
		if command.Action == ActionCreate && command.Definition.Sealed {
			return fmt.Errorf("streams can only be sealed after creation")
		}
	}
	if strings.TrimSpace(command.Name) == "" {
		return fmt.Errorf("stream name is required")
	}
	return h.operator.OperateStream(ctx, command)
}
