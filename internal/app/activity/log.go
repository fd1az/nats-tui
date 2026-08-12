package activity

import (
	"errors"
	"strings"
	"time"
)

type Level string

const (
	Info    Level = "info"
	Success Level = "success"
	Warning Level = "warning"
	Error   Level = "error"
)

type Event struct {
	ID     uint64
	Level  Level
	Title  string
	Detail string
	At     time.Time
}

type Log struct {
	limit  int
	nextID uint64
	events []Event
}

func NewLog(limit int) *Log {
	if limit < 1 {
		panic("activity log limit must be positive")
	}
	return &Log{limit: limit}
}

func (l *Log) Add(level Level, title, detail string, at time.Time) (Event, error) {
	title, detail = strings.TrimSpace(title), strings.TrimSpace(detail)
	if title == "" {
		return Event{}, errors.New("activity title is required")
	}
	switch level {
	case Info, Success, Warning, Error:
	default:
		return Event{}, errors.New("invalid activity level")
	}
	l.nextID++
	event := Event{ID: l.nextID, Level: level, Title: title, Detail: detail, At: at}
	l.events = append(l.events, event)
	if overflow := len(l.events) - l.limit; overflow > 0 {
		copy(l.events, l.events[overflow:])
		l.events = l.events[:l.limit]
	}
	return event, nil
}
func (l *Log) Events() []Event { return append([]Event(nil), l.events...) }
func (l *Log) Clear()          { l.events = nil }
