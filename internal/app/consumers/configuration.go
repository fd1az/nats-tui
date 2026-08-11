package consumers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type DeliverPolicy string

const (
	DeliverAll             DeliverPolicy = "all"
	DeliverLast            DeliverPolicy = "last"
	DeliverNew             DeliverPolicy = "new"
	DeliverByStartSequence DeliverPolicy = "by-start-sequence"
	DeliverByStartTime     DeliverPolicy = "by-start-time"
	DeliverLastPerSubject  DeliverPolicy = "last-per-subject"
)

type AckPolicy string

const (
	AckNone     AckPolicy = "none"
	AckAll      AckPolicy = "all"
	AckExplicit AckPolicy = "explicit"
)

type ReplayPolicy string

const (
	ReplayInstant  ReplayPolicy = "instant"
	ReplayOriginal ReplayPolicy = "original"
)

type ConfigAction uint8

const (
	ConfigCreate ConfigAction = iota + 1
	ConfigUpdate
)

type Definition struct {
	Stream, Name, Description, DeliverSubject, DeliverGroup string
	Durable, HeadersOnly, MemoryStorage, FlowControl        bool
	DeliverPolicy                                           DeliverPolicy
	StartSequence                                           uint64
	StartTime                                               *time.Time
	AckPolicy                                               AckPolicy
	AckWait                                                 time.Duration
	MaxDeliver, MaxAckPending, MaxWaiting                   int
	Backoff                                                 []time.Duration
	FilterSubjects                                          []string
	ReplayPolicy                                            ReplayPolicy
	RateLimit                                               uint64
	SampleFrequency                                         string
	MaxRequestBatch, MaxRequestMaxBytes                     int
	MaxRequestExpires, InactiveThreshold                    time.Duration
	Replicas                                                int
	Metadata                                                map[string]string
	IdleHeartbeat                                           time.Duration
}

func DefaultDefinition(stream, name string) Definition {
	return Definition{Stream: stream, Name: name, Durable: true, DeliverPolicy: DeliverAll, AckPolicy: AckExplicit, MaxDeliver: -1, MaxAckPending: 1000, MaxWaiting: 512, ReplayPolicy: ReplayInstant}
}

func (d Definition) Validate() error {
	d.Stream, d.Name = strings.TrimSpace(d.Stream), strings.TrimSpace(d.Name)
	if d.Stream == "" || d.Name == "" {
		return fmt.Errorf("consumer stream and name are required")
	}
	if strings.ContainsAny(d.Name, " .*\\/\t\r\n") {
		return fmt.Errorf("consumer name cannot contain whitespace, dots, wildcards, or path separators")
	}
	if !oneOf(string(d.DeliverPolicy), string(DeliverAll), string(DeliverLast), string(DeliverNew), string(DeliverByStartSequence), string(DeliverByStartTime), string(DeliverLastPerSubject)) {
		return fmt.Errorf("unsupported deliver policy %q", d.DeliverPolicy)
	}
	if d.DeliverPolicy == DeliverByStartSequence && d.StartSequence == 0 {
		return fmt.Errorf("start sequence must be greater than zero for %s delivery", d.DeliverPolicy)
	}
	if d.DeliverPolicy == DeliverByStartTime && d.StartTime == nil {
		return fmt.Errorf("start time is required for %s delivery", d.DeliverPolicy)
	}
	if !oneOf(string(d.AckPolicy), string(AckNone), string(AckAll), string(AckExplicit)) {
		return fmt.Errorf("unsupported ack policy %q", d.AckPolicy)
	}
	if d.AckWait < 0 || d.IdleHeartbeat < 0 || d.MaxRequestExpires < 0 || d.InactiveThreshold < 0 {
		return fmt.Errorf("consumer durations cannot be negative")
	}
	if d.MaxDeliver < -1 || d.MaxDeliver == 0 || d.MaxAckPending < -1 || d.MaxWaiting < 0 || d.MaxRequestBatch < 0 || d.MaxRequestMaxBytes < 0 || d.Replicas < 0 {
		return fmt.Errorf("consumer limits are outside their supported ranges")
	}
	if len(d.Backoff) > 0 && d.MaxDeliver > 0 && len(d.Backoff) > d.MaxDeliver {
		return fmt.Errorf("backoff entries cannot exceed max deliver")
	}
	if d.AckPolicy == AckNone && (d.AckWait > 0 || len(d.Backoff) > 0 || d.MaxDeliver > 1) {
		return fmt.Errorf("ack wait, backoff, and redelivery require an acknowledging policy")
	}
	if d.DeliverSubject == "" && (d.DeliverGroup != "" || d.FlowControl || d.IdleHeartbeat > 0) {
		return fmt.Errorf("deliver group, flow control, and idle heartbeat require a push deliver subject")
	}
	if d.FlowControl && d.IdleHeartbeat == 0 {
		return fmt.Errorf("flow control requires an idle heartbeat")
	}
	if !oneOf(string(d.ReplayPolicy), string(ReplayInstant), string(ReplayOriginal)) {
		return fmt.Errorf("unsupported replay policy %q", d.ReplayPolicy)
	}
	if err := validateSampleFrequency(d.SampleFrequency); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(d.FilterSubjects))
	for _, subject := range d.FilterSubjects {
		subject = strings.TrimSpace(subject)
		if subject == "" {
			return fmt.Errorf("filter subjects cannot be empty")
		}
		if _, exists := seen[subject]; exists {
			return fmt.Errorf("filter subject %q is duplicated", subject)
		}
		seen[subject] = struct{}{}
	}
	for key := range d.Metadata {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("metadata keys cannot be empty")
		}
	}
	return nil
}

func validateSampleFrequency(value string) error {
	value = strings.TrimSpace(strings.TrimSuffix(value, "%"))
	if value == "" {
		return nil
	}
	percentage, err := strconv.Atoi(value)
	if err != nil || percentage < 0 || percentage > 100 {
		return fmt.Errorf("sample frequency must be between 0 and 100 percent")
	}
	return nil
}

func oneOf(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

func (d Definition) Clone() Definition {
	d.Backoff = append([]time.Duration(nil), d.Backoff...)
	d.FilterSubjects = append([]string(nil), d.FilterSubjects...)
	d.Metadata = cloneMap(d.Metadata)
	if d.StartTime != nil {
		value := *d.StartTime
		d.StartTime = &value
	}
	return d
}

func cloneMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

type ConfigCommand struct {
	Action     ConfigAction
	Definition Definition
}

type ConfigOperator interface {
	OperateConsumerConfig(context.Context, ConfigCommand) error
}

type ConfigureHandler struct{ operator ConfigOperator }

func NewConfigureHandler(operator ConfigOperator) ConfigureHandler {
	if operator == nil {
		panic("nil consumer configuration operator")
	}
	return ConfigureHandler{operator: operator}
}

func (h ConfigureHandler) Handle(ctx context.Context, command ConfigCommand) error {
	if command.Action != ConfigCreate && command.Action != ConfigUpdate {
		return fmt.Errorf("unsupported consumer configuration action %d", command.Action)
	}
	if err := command.Definition.Validate(); err != nil {
		return err
	}
	return h.operator.OperateConsumerConfig(ctx, command)
}
