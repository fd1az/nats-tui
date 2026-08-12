package natsmonitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	app "github.com/fd1az/nats-tui/internal/app/connections"
	appTopology "github.com/fd1az/nats-tui/internal/app/topology"
	"github.com/nats-io/nats.go"
)

type subscriptionWire struct {
	Subject  string `json:"subject"`
	Queue    string `json:"qgroup"`
	SID      string `json:"sid"`
	Messages int64  `json:"msgs"`
	Max      int64  `json:"max"`
}
type connectionWire struct {
	CID          uint64             `json:"cid"`
	IP           string             `json:"ip"`
	Start        time.Time          `json:"start"`
	LastActivity time.Time          `json:"last_activity"`
	RTT          string             `json:"rtt"`
	Pending      int64              `json:"pending_bytes"`
	InMessages   int64              `json:"in_msgs"`
	OutMessages  int64              `json:"out_msgs"`
	InBytes      int64              `json:"in_bytes"`
	OutBytes     int64              `json:"out_bytes"`
	Stalls       int64              `json:"stalls"`
	Name         string             `json:"name"`
	Lang         string             `json:"lang"`
	Version      string             `json:"version"`
	User         string             `json:"authorized_user"`
	Account      string             `json:"account"`
	Subs         []subscriptionWire `json:"subscriptions_list_detail"`
}
type connzWire struct {
	ServerID    string           `json:"server_id"`
	Now         time.Time        `json:"now"`
	Connections []connectionWire `json:"connections"`
}

var connectionQuery = []byte(`{"auth":true,"subscriptions":true,"subscriptions_detail":true,"limit":10000}`)

func (m *Monitor) ListConnections(ctx context.Context) (app.Snapshot, error) {
	topology, err := m.Topology(ctx)
	if err != nil {
		return app.Snapshot{}, err
	}
	var result []app.Connection
	observed := m.now()
	for _, server := range topology.Servers() {
		var wire connzWire
		if err := m.requestServerJSON(ctx, server.ID().String(), "CONNZ", connectionQuery, &wire); err != nil {
			if m.connectionFallback != nil {
				return m.connectionFallback.ListConnections(ctx)
			}
			if errors.Is(err, nats.ErrNoResponders) {
				return app.Snapshot{}, appTopology.ErrSystemAccountRequired
			}
			return app.Snapshot{}, fmt.Errorf("list connections on %s: %w", server.Name(), err)
		}
		if !wire.Now.IsZero() {
			observed = wire.Now
		}
		result = append(result, mapConnections(server.ID().String(), server.Name(), wire.Connections)...)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ServerName == result[j].ServerName {
			return result[i].CID < result[j].CID
		}
		return result[i].ServerName < result[j].ServerName
	})
	return app.Snapshot{Connections: result, ObservedAt: observed}, nil
}

func mapConnections(serverID, serverName string, wires []connectionWire) []app.Connection {
	result := make([]app.Connection, 0, len(wires))
	for _, wire := range wires {
		subscriptions := make([]app.Subscription, 0, len(wire.Subs))
		for _, sub := range wire.Subs {
			subscriptions = append(subscriptions, app.Subscription{Subject: sub.Subject, Queue: sub.Queue, SID: sub.SID, Delivered: sub.Messages, Max: sub.Max})
		}
		result = append(result, app.Connection{ServerID: serverID, ServerName: serverName, CID: wire.CID, Name: wire.Name, User: wire.User, Account: wire.Account, IP: wire.IP, Lang: wire.Lang, Version: wire.Version, RTT: parseRTT(wire.RTT), PendingBytes: wire.Pending, InBytes: wire.InBytes, OutBytes: wire.OutBytes, Stalls: wire.Stalls, InMessages: wire.InMessages, OutMessages: wire.OutMessages, StartedAt: wire.Start, LastActivity: wire.LastActivity, Subscriptions: subscriptions})
	}
	return result
}

func (m *Monitor) CloseConnection(ctx context.Context, serverID string, cid uint64) error {
	payload, err := json.Marshal(struct {
		CID uint64 `json:"cid"`
	}{cid})
	if err != nil {
		return err
	}
	message, err := m.connection.RequestWithContext(ctx, fmt.Sprintf("$SYS.REQ.SERVER.%s.KICK", serverID), payload)
	if err != nil {
		if errors.Is(err, nats.ErrNoResponders) {
			return appTopology.ErrSystemAccountRequired
		}
		return fmt.Errorf("close connection: %w", err)
	}
	var response apiResponse
	if err := json.Unmarshal(message.Data, &response); err != nil {
		return fmt.Errorf("decode close connection response: %w", err)
	}
	if response.Error != nil {
		return fmt.Errorf("close connection: %s", response.Error.Description)
	}
	return nil
}
