package topology

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrServerIDRequired   = errors.New("server ID is required")
	ErrServerNameRequired = errors.New("server name is required")
	ErrDuplicateServer    = errors.New("duplicate server identity")
)

type ServerID string

func NewServerID(value string) (ServerID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrServerIDRequired
	}
	return ServerID(value), nil
}
func (id ServerID) String() string { return string(id) }

type Health string

const (
	HealthHealthy  Health = "healthy"
	HealthDegraded Health = "degraded"
	HealthDown     Health = "down"
)

type LinkKind string

const (
	LinkRoute   LinkKind = "route"
	LinkGateway LinkKind = "gateway"
	LinkLeaf    LinkKind = "leaf"
)

type Resources struct{ memory, storage, reservedMemory, reservedStorage uint64 }

func NewResources(memory, storage, reservedMemory, reservedStorage uint64) Resources {
	return Resources{memory, storage, reservedMemory, reservedStorage}
}
func (r Resources) Memory() uint64          { return r.memory }
func (r Resources) Storage() uint64         { return r.storage }
func (r Resources) ReservedMemory() uint64  { return r.reservedMemory }
func (r Resources) ReservedStorage() uint64 { return r.reservedStorage }

type Link struct {
	kind          LinkKind
	local, remote ServerID
	remoteName    string
	connected     bool
	pending       int64
	rtt           time.Duration
}

func NewLink(kind LinkKind, local, remote, remoteName string, connected bool, pending int64, rtt time.Duration) (Link, error) {
	if kind != LinkRoute && kind != LinkGateway && kind != LinkLeaf {
		return Link{}, fmt.Errorf("unknown link kind %q", kind)
	}
	localID, err := NewServerID(local)
	if err != nil {
		return Link{}, fmt.Errorf("local server: %w", err)
	}
	remoteID := ServerID(strings.TrimSpace(remote))
	if remoteID == "" {
		remoteID = ServerID(strings.TrimSpace(remoteName))
	}
	if remoteID == "" {
		return Link{}, fmt.Errorf("remote server: %w", ErrServerIDRequired)
	}
	if pending < 0 {
		return Link{}, errors.New("pending bytes cannot be negative")
	}
	if rtt < 0 {
		return Link{}, errors.New("RTT cannot be negative")
	}
	return Link{kind: kind, local: localID, remote: remoteID, remoteName: strings.TrimSpace(remoteName), connected: connected, pending: pending, rtt: rtt}, nil
}
func (l Link) Kind() LinkKind      { return l.kind }
func (l Link) Local() ServerID     { return l.local }
func (l Link) Remote() ServerID    { return l.remote }
func (l Link) RemoteName() string  { return l.remoteName }
func (l Link) PendingBytes() int64 { return l.pending }
func (l Link) RTT() time.Duration  { return l.rtt }
func (l Link) Health() Health {
	if !l.connected {
		return HealthDown
	}
	if l.pending > 0 {
		return HealthDegraded
	}
	return HealthHealthy
}

type ServerInput struct {
	ID, Name, Host, Cluster, Domain, Version string
	Tags, Endpoints                          []string
	StartedAt, ObservedAt                    time.Time
	Connections, Subscriptions               int
	CPU                                      float64
	Memory                                   int64
	SlowConsumers                            int64
	JetStream                                Resources
	Links                                    []Link
}

type Server struct {
	input ServerInput
	id    ServerID
}

func NewServer(input ServerInput) (Server, error) {
	id, err := NewServerID(input.ID)
	if err != nil {
		return Server{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return Server{}, ErrServerNameRequired
	}
	if input.Connections < 0 || input.Subscriptions < 0 || input.Memory < 0 || input.SlowConsumers < 0 {
		return Server{}, errors.New("server counters cannot be negative")
	}
	input.Tags = append([]string(nil), input.Tags...)
	input.Endpoints = append([]string(nil), input.Endpoints...)
	input.Links = append([]Link(nil), input.Links...)
	return Server{input: input, id: id}, nil
}
func (s Server) IsZero() bool          { return s.id == "" }
func (s Server) ID() ServerID          { return s.id }
func (s Server) Name() string          { return s.input.Name }
func (s Server) Host() string          { return s.input.Host }
func (s Server) Cluster() string       { return s.input.Cluster }
func (s Server) Domain() string        { return s.input.Domain }
func (s Server) Version() string       { return s.input.Version }
func (s Server) Tags() []string        { return append([]string(nil), s.input.Tags...) }
func (s Server) Endpoints() []string   { return append([]string(nil), s.input.Endpoints...) }
func (s Server) StartedAt() time.Time  { return s.input.StartedAt }
func (s Server) ObservedAt() time.Time { return s.input.ObservedAt }
func (s Server) Uptime() time.Duration {
	if s.input.StartedAt.IsZero() || s.input.ObservedAt.Before(s.input.StartedAt) {
		return 0
	}
	return s.input.ObservedAt.Sub(s.input.StartedAt)
}
func (s Server) Connections() int     { return s.input.Connections }
func (s Server) Subscriptions() int   { return s.input.Subscriptions }
func (s Server) CPU() float64         { return s.input.CPU }
func (s Server) Memory() int64        { return s.input.Memory }
func (s Server) SlowConsumers() int64 { return s.input.SlowConsumers }
func (s Server) JetStream() Resources { return s.input.JetStream }
func (s Server) Links() []Link        { return append([]Link(nil), s.input.Links...) }

type Peer struct {
	Name    string
	Current bool
	Offline bool
	Lag     uint64
}
type MetaGroup struct {
	Name, Leader string
	Peers        []Peer
}

func (m MetaGroup) Clone() MetaGroup { m.Peers = append([]Peer(nil), m.Peers...); return m }

type Snapshot struct {
	servers []Server
	meta    MetaGroup
}

func NewSnapshot(servers []Server, meta MetaGroup) (Snapshot, error) {
	owned := append([]Server(nil), servers...)
	seen := make(map[ServerID]struct{}, len(owned))
	for _, server := range owned {
		if server.IsZero() {
			return Snapshot{}, ErrServerIDRequired
		}
		if _, ok := seen[server.ID()]; ok {
			return Snapshot{}, fmt.Errorf("%w: %s", ErrDuplicateServer, server.ID())
		}
		seen[server.ID()] = struct{}{}
	}
	return Snapshot{servers: owned, meta: meta.Clone()}, nil
}
func (s Snapshot) Servers() []Server { return append([]Server(nil), s.servers...) }
func (s Snapshot) Meta() MetaGroup   { return s.meta.Clone() }
