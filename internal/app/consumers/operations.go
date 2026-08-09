package consumers

import (
	"context"
	"fmt"
	"strings"
)

type Action uint8

const (
	ActionPause Action = iota + 1
	ActionResume
	ActionReset
	ActionDelete
)

func (a Action) String() string {
	switch a {
	case ActionPause:
		return "pause"
	case ActionResume:
		return "resume"
	case ActionReset:
		return "reset"
	case ActionDelete:
		return "delete"
	default:
		return "unknown"
	}
}

type Ref struct {
	stream string
	name   string
}

func NewRef(stream, name string) (Ref, error) {
	stream = strings.TrimSpace(stream)
	name = strings.TrimSpace(name)
	if stream == "" || name == "" {
		return Ref{}, fmt.Errorf("consumer stream and name are required")
	}
	return Ref{stream: stream, name: name}, nil
}

func (r Ref) Stream() string { return r.stream }
func (r Ref) Name() string   { return r.name }

type Command struct {
	Consumer Ref
	Action   Action
}

type Operator interface {
	OperateConsumer(context.Context, Command) error
}

type OperateHandler struct{ operator Operator }

func NewOperateHandler(operator Operator) OperateHandler {
	if operator == nil {
		panic("nil consumer operator")
	}
	return OperateHandler{operator: operator}
}

func (h OperateHandler) Handle(ctx context.Context, command Command) error {
	return h.operator.OperateConsumer(ctx, command)
}
