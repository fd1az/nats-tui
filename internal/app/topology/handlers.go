package topology

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domain "github.com/fd1az/nats-tui/internal/domain/topology"
)

var (
	ErrServerRequired        = errors.New("server ID is required for reload")
	ErrSystemAccountRequired = errors.New("NATS system-account permission is required")
)

type Source interface {
	Topology(context.Context) (domain.Snapshot, error)
}
type Operator interface {
	OperateTopology(context.Context, Command) error
}

type Action string

const (
	ActionReload       Action = "reload"
	ActionStepDownMeta Action = "step-down-meta"
)

type Command struct {
	Action   Action
	ServerID string
}

type GetHandler struct{ source Source }

func NewGetHandler(source Source) GetHandler {
	if source == nil {
		panic("nil topology source")
	}
	return GetHandler{source: source}
}
func (h GetHandler) Handle(ctx context.Context) (domain.Snapshot, error) {
	return h.source.Topology(ctx)
}

type OperateHandler struct{ operator Operator }

func NewOperateHandler(operator Operator) OperateHandler {
	if operator == nil {
		panic("nil topology operator")
	}
	return OperateHandler{operator: operator}
}
func (h OperateHandler) Handle(ctx context.Context, command Command) error {
	if command.Action == ActionReload && strings.TrimSpace(command.ServerID) == "" {
		return ErrServerRequired
	}
	if command.Action != ActionReload && command.Action != ActionStepDownMeta {
		return fmt.Errorf("unsupported topology action %q", command.Action)
	}
	return h.operator.OperateTopology(ctx, command)
}
