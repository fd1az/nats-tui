package connections

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrConnectionTargetRequired = errors.New("server ID and positive CID are required")

const DefaultSlowPendingBytes int64 = 1 << 20

type Subscription struct {
	Subject, Queue, SID string
	Delivered, Max      int64
}
type Connection struct {
	ServerID, ServerName                    string
	CID                                     uint64
	Name, User, Account, IP, Lang, Version  string
	RTT                                     time.Duration
	PendingBytes, InBytes, OutBytes, Stalls int64
	InMessages, OutMessages                 int64
	StartedAt, LastActivity                 time.Time
	Subscriptions                           []Subscription
}

func (c Connection) Key() string    { return c.ServerID + ":" + stringID(c.CID) }
func (c Connection) Traffic() int64 { return c.InBytes + c.OutBytes }
func (c Connection) SlowConsumer() bool {
	return c.Stalls > 0 || c.PendingBytes >= DefaultSlowPendingBytes
}

type Snapshot struct {
	Connections []Connection
	ObservedAt  time.Time
}

func (s Snapshot) Clone() Snapshot {
	result := Snapshot{ObservedAt: s.ObservedAt, Connections: make([]Connection, len(s.Connections))}
	copy(result.Connections, s.Connections)
	for i := range result.Connections {
		result.Connections[i].Subscriptions = append([]Subscription(nil), result.Connections[i].Subscriptions...)
	}
	return result
}
func (s Snapshot) SubscriptionCount() int {
	count := 0
	for _, connection := range s.Connections {
		count += len(connection.Subscriptions)
	}
	return count
}
func (s Snapshot) SlowConsumerCount() int {
	count := 0
	for _, connection := range s.Connections {
		if connection.SlowConsumer() {
			count++
		}
	}
	return count
}

type Sort string

const (
	SortName     Sort = "name"
	SortUser     Sort = "user"
	SortIP       Sort = "ip"
	SortLanguage Sort = "language"
	SortVersion  Sort = "version"
	SortRTT      Sort = "rtt"
	SortTraffic  Sort = "traffic"
	SortPending  Sort = "pending"
)

type Group string

const (
	GroupNone     Group = "none"
	GroupName     Group = "name"
	GroupUser     Group = "user"
	GroupServer   Group = "server"
	GroupLanguage Group = "language"
)

type Query struct {
	Filter   string
	Sort     Sort
	OnlySlow bool
}

func (q Query) Apply(items []Connection) []Connection {
	query := strings.ToLower(strings.TrimSpace(q.Filter))
	result := make([]Connection, 0, len(items))
	for _, item := range items {
		if q.OnlySlow && !item.SlowConsumer() {
			continue
		}
		haystack := strings.Join([]string{item.Name, item.User, item.IP, item.Lang, item.Version, item.ServerName, item.RTT.String(), strconv.FormatInt(item.Traffic(), 10), strconv.FormatInt(item.PendingBytes, 10)}, " ")
		if query != "" && !strings.Contains(strings.ToLower(haystack), query) {
			continue
		}
		item.Subscriptions = append([]Subscription(nil), item.Subscriptions...)
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool { return less(result[i], result[j], q.Sort) })
	return result
}
func less(a, b Connection, by Sort) bool {
	switch by {
	case SortUser:
		return a.User < b.User
	case SortIP:
		return a.IP < b.IP
	case SortLanguage:
		return a.Lang < b.Lang
	case SortVersion:
		return a.Version < b.Version
	case SortRTT:
		return a.RTT < b.RTT
	case SortTraffic:
		return a.Traffic() > b.Traffic()
	case SortPending:
		return a.PendingBytes > b.PendingBytes
	default:
		return a.Name < b.Name
	}
}

type ConnectionGroup struct {
	Name        string
	Connections []Connection
}

func GroupBy(items []Connection, by Group) []ConnectionGroup {
	if by == GroupNone {
		return []ConnectionGroup{{Connections: append([]Connection(nil), items...)}}
	}
	values := map[string][]Connection{}
	for _, item := range items {
		name := ""
		switch by {
		case GroupName:
			name = item.Name
		case GroupUser:
			name = item.User
		case GroupServer:
			name = item.ServerName
		case GroupLanguage:
			name = item.Lang
		}
		if name == "" {
			name = "(unnamed)"
		}
		values[name] = append(values[name], item)
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	groups := make([]ConnectionGroup, 0, len(names))
	for _, name := range names {
		groups = append(groups, ConnectionGroup{Name: name, Connections: values[name]})
	}
	return groups
}

type Source interface {
	ListConnections(context.Context) (Snapshot, error)
}
type Operator interface {
	CloseConnection(context.Context, string, uint64) error
}
type ListHandler struct{ source Source }

func NewListHandler(source Source) ListHandler {
	if source == nil {
		panic("nil connection source")
	}
	return ListHandler{source}
}
func (h ListHandler) Handle(ctx context.Context) (Snapshot, error) {
	return h.source.ListConnections(ctx)
}

type CloseHandler struct{ operator Operator }

func NewCloseHandler(operator Operator) CloseHandler {
	if operator == nil {
		panic("nil connection operator")
	}
	return CloseHandler{operator}
}
func (h CloseHandler) Handle(ctx context.Context, serverID string, cid uint64) error {
	if strings.TrimSpace(serverID) == "" || cid == 0 {
		return ErrConnectionTargetRequired
	}
	return h.operator.CloseConnection(ctx, strings.TrimSpace(serverID), cid)
}
func stringID(value uint64) string {
	if value == 0 {
		return "0"
	}
	var buffer [20]byte
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = byte('0' + value%10)
		value /= 10
	}
	return string(buffer[index:])
}
