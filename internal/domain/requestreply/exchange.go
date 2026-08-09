package requestreply

import (
	"fmt"
	"strings"
	"time"
)

type Exchange struct {
	id        string
	startedAt time.Time
	request   Request
	replies   []Reply
}

func NewExchange(id string, startedAt time.Time, request Request, replies []Reply) (Exchange, error) {
	id = strings.TrimSpace(id)
	if id == "" || startedAt.IsZero() || request.IsZero() {
		return Exchange{}, fmt.Errorf("exchange identity, start time, and request are required")
	}
	return Exchange{id: id, startedAt: startedAt, request: request, replies: append([]Reply(nil), replies...)}, nil
}

func (e Exchange) ID() string           { return e.id }
func (e Exchange) StartedAt() time.Time { return e.startedAt }
func (e Exchange) Request() Request     { return e.request }
func (e Exchange) Replies() []Reply     { return append([]Reply(nil), e.replies...) }
func (e Exchange) IsZero() bool         { return e.id == "" }
