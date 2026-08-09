package streams

import (
	"context"
	"fmt"
	"strings"
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
	Name        string
	Description string
	Subjects    []string
	Storage     Storage
	Retention   Retention
	MaxMessages int64
	MaxBytes    int64
	Replicas    int
}

func (d Definition) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("stream name is required")
	}
	if strings.ContainsAny(d.Name, " \t\r\n.*>/\\") {
		return fmt.Errorf("stream name contains unsupported characters")
	}
	if len(d.Subjects) == 0 {
		return fmt.Errorf("at least one subject is required")
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
	if d.Replicas < 1 || d.Replicas > 5 {
		return fmt.Errorf("replicas must be between 1 and 5")
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

type OperateHandler struct{ operator Operator }

func NewOperateHandler(operator Operator) OperateHandler {
	if operator == nil {
		panic("nil stream operator")
	}
	return OperateHandler{operator: operator}
}

func (h OperateHandler) Handle(ctx context.Context, command Command) error {
	if command.Action == ActionCreate || command.Action == ActionUpdate {
		if err := command.Definition.Validate(); err != nil {
			return err
		}
	}
	if strings.TrimSpace(command.Name) == "" {
		return fmt.Errorf("stream name is required")
	}
	return h.operator.OperateStream(ctx, command)
}
