package natsmonitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	app "github.com/clcsolutions/nats-tui/internal/app/topology"
	domain "github.com/clcsolutions/nats-tui/internal/domain/topology"
	"github.com/nats-io/nats.go"
)

const (
	serverStatsSubject  = "$SYS.REQ.SERVER.PING.STATSZ"
	metaStepDownSubject = "$JS.API.META.LEADER.STEPDOWN"
)

type serverInfoWire struct {
	ID, Name, Host, Cluster, Domain, Version string
	Tags                                     []string
	Time                                     time.Time
}

func (s *serverInfoWire) UnmarshalJSON(data []byte) error {
	type wire struct {
		ID      string    `json:"id"`
		Name    string    `json:"name"`
		Host    string    `json:"host"`
		Cluster string    `json:"cluster"`
		Domain  string    `json:"domain"`
		Version string    `json:"ver"`
		Tags    []string  `json:"tags"`
		Time    time.Time `json:"time"`
	}
	var value wire
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	s.ID, s.Name, s.Host, s.Cluster, s.Domain, s.Version, s.Tags, s.Time = value.ID, value.Name, value.Host, value.Cluster, value.Domain, value.Version, value.Tags, value.Time
	return nil
}

type jetStreamWire struct {
	Memory         uint64 `json:"memory"`
	Store          uint64 `json:"storage"`
	ReservedMemory uint64 `json:"reserved_memory"`
	ReservedStore  uint64 `json:"reserved_storage"`
}
type routeStatWire struct {
	Name    string `json:"name"`
	Pending int64  `json:"pending"`
}
type gatewayStatWire struct {
	Name string `json:"name"`
}
type dataStatsWire struct {
	Messages uint64 `json:"msgs"`
	Bytes    uint64 `json:"bytes"`
}
type serverStatsWire struct {
	Start            time.Time         `json:"start"`
	Mem              int64             `json:"mem"`
	CPU              float64           `json:"cpu"`
	Connections      int               `json:"connections"`
	Subscriptions    int               `json:"subscriptions"`
	SlowConsumers    int64             `json:"slow_consumers"`
	StaleConnections int64             `json:"stale_connections"`
	StalledClients   int64             `json:"stalled_clients"`
	Cores            int               `json:"cores"`
	MaxProcs         int               `json:"gomaxprocs"`
	Goroutines       int               `json:"goroutines"`
	Sent             dataStatsWire     `json:"sent"`
	Received         dataStatsWire     `json:"received"`
	ActiveServers    int               `json:"active_servers"`
	JetStream        *jetStreamWire    `json:"jetstream"`
	Routes           []routeStatWire   `json:"routes"`
	Gateways         []gatewayStatWire `json:"gateways"`
}
type statsResponse struct {
	Server serverInfoWire  `json:"server"`
	Stats  serverStatsWire `json:"statsz"`
	Error  *apiErrorWire   `json:"error,omitempty"`
}
type apiErrorWire struct {
	Code        int    `json:"code"`
	Description string `json:"description"`
	ErrCode     int    `json:"err_code"`
}
type apiResponse struct {
	Error *apiErrorWire `json:"error,omitempty"`
}

type varzWire struct {
	ID               string    `json:"server_id"`
	Name             string    `json:"server_name"`
	Version          string    `json:"version"`
	Host             string    `json:"host"`
	Port             int       `json:"port"`
	ConnectURLs      []string  `json:"connect_urls"`
	WSConnectURLs    []string  `json:"ws_connect_urls"`
	Start            time.Time `json:"start"`
	Now              time.Time `json:"now"`
	Mem              int64     `json:"mem"`
	CPU              float64   `json:"cpu"`
	Connections      int       `json:"connections"`
	Subscriptions    int       `json:"subscriptions"`
	SlowConsumers    int64     `json:"slow_consumers"`
	StaleConnections int64     `json:"stale_connections"`
	StalledClients   int64     `json:"stalled_clients"`
	Cores            int       `json:"cores"`
	MaxProcs         int       `json:"gomaxprocs"`
	Goroutines       int       `json:"goroutines"`
	NumGoroutines    int       `json:"num_goroutines"`
	InMessages       uint64    `json:"in_msgs"`
	OutMessages      uint64    `json:"out_msgs"`
	InBytes          uint64    `json:"in_bytes"`
	OutBytes         uint64    `json:"out_bytes"`
	Tags             []string  `json:"tags"`
	Cluster          struct {
		Name string `json:"name"`
	} `json:"cluster"`
	JetStream struct {
		Config struct {
			Domain string `json:"domain"`
		} `json:"config"`
	} `json:"jetstream"`
}
type routeWire struct {
	RemoteID   string `json:"remote_id"`
	RemoteName string `json:"remote_name"`
	RTT        string `json:"rtt"`
	Pending    int64  `json:"pending_size"`
}
type routezWire struct {
	Routes []routeWire `json:"routes"`
}
type gatewayConnectionWire struct {
	RTT     string `json:"rtt"`
	Pending int64  `json:"pending_size"`
	Name    string `json:"name"`
}
type remoteGatewayWire struct {
	Connection *gatewayConnectionWire `json:"connection"`
}
type gatewayzWire struct {
	Outbound map[string]*remoteGatewayWire   `json:"outbound_gateways"`
	Inbound  map[string][]*remoteGatewayWire `json:"inbound_gateways"`
}
type leafWire struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
	RTT  string `json:"rtt"`
}
type leafzWire struct {
	Leafs []leafWire `json:"leafs"`
}
type peerWire struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
	Offline bool   `json:"offline"`
	Lag     uint64 `json:"lag"`
}
type metaWire struct {
	Name     string     `json:"name"`
	Leader   string     `json:"leader"`
	Replicas []peerWire `json:"replicas"`
}
type jszWire struct {
	Memory         uint64    `json:"memory"`
	Store          uint64    `json:"storage"`
	ReservedMemory uint64    `json:"reserved_memory"`
	ReservedStore  uint64    `json:"reserved_storage"`
	Meta           *metaWire `json:"meta_cluster"`
}
type serverDetails struct {
	Varz     varzWire
	Routes   routezWire
	Gateways gatewayzWire
	Leafs    leafzWire
	JS       jszWire
}

func (m *Monitor) Topology(ctx context.Context) (domain.Snapshot, error) {
	responses, err := m.collectServerStats(ctx)
	if err != nil {
		if errors.Is(err, app.ErrSystemAccountRequired) && m.topologyFallback != nil {
			return m.topologyFallback.Topology(ctx)
		}
		return domain.Snapshot{}, err
	}
	servers := make([]domain.Server, 0, len(responses))
	meta := domain.MetaGroup{}
	for _, response := range responses {
		details := m.loadServerDetails(ctx, response.Server.ID)
		server, mapErr := mapTopologyServer(response, details)
		if mapErr != nil {
			return domain.Snapshot{}, mapErr
		}
		servers = append(servers, server)
		if details.JS.Meta != nil {
			meta = mapMeta(*details.JS.Meta)
		}
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].Name() < servers[j].Name() })
	return domain.NewSnapshot(servers, meta)
}

func (m *Monitor) collectServerStats(ctx context.Context) ([]statsResponse, error) {
	inbox := nats.NewInbox()
	sub, err := m.connection.SubscribeSync(inbox)
	if err != nil {
		return nil, fmt.Errorf("subscribe for server topology: %w", err)
	}
	defer func() { _ = sub.Unsubscribe() }()
	if err := m.connection.PublishRequest(serverStatsSubject, inbox, nil); err != nil {
		return nil, fmt.Errorf("request server topology: %w", err)
	}
	if err := m.connection.FlushWithContext(ctx); err != nil {
		return nil, fmt.Errorf("flush topology request: %w", err)
	}
	var result []statsResponse
	expected := 1
	for len(result) < expected {
		message, receiveErr := sub.NextMsgWithContext(ctx)
		if receiveErr != nil {
			if len(result) > 0 && (errors.Is(receiveErr, context.DeadlineExceeded) || errors.Is(receiveErr, nats.ErrTimeout)) {
				break
			}
			if errors.Is(receiveErr, nats.ErrNoResponders) || errors.Is(receiveErr, context.DeadlineExceeded) {
				return nil, app.ErrSystemAccountRequired
			}
			return nil, fmt.Errorf("receive server topology: %w", receiveErr)
		}
		var response statsResponse
		if err := json.Unmarshal(message.Data, &response); err != nil {
			return nil, fmt.Errorf("decode server topology: %w", err)
		}
		if response.Error != nil {
			return nil, fmt.Errorf("server topology: %s", response.Error.Description)
		}
		if response.Stats.ActiveServers > expected {
			expected = response.Stats.ActiveServers
		}
		result = append(result, response)
	}
	return result, nil
}

func (m *Monitor) loadServerDetails(ctx context.Context, id string) serverDetails {
	var details serverDetails
	_ = m.requestServerJSON(ctx, id, "VARZ", nil, &details.Varz)
	_ = m.requestServerJSON(ctx, id, "ROUTEZ", nil, &details.Routes)
	_ = m.requestServerJSON(ctx, id, "GATEWAYZ", nil, &details.Gateways)
	_ = m.requestServerJSON(ctx, id, "LEAFZ", nil, &details.Leafs)
	_ = m.requestServerJSON(ctx, id, "JSZ", []byte(`{"raft":true}`), &details.JS)
	return details
}

func (m *Monitor) requestServerJSON(ctx context.Context, id, endpoint string, payload []byte, target any) error {
	message, err := m.connection.RequestWithContext(ctx, fmt.Sprintf("$SYS.REQ.SERVER.%s.%s", id, endpoint), payload)
	if err != nil {
		return err
	}
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error *apiErrorWire   `json:"error"`
	}
	if err := json.Unmarshal(message.Data, &envelope); err != nil {
		return err
	}
	if envelope.Error != nil {
		return errors.New(envelope.Error.Description)
	}
	return json.Unmarshal(envelope.Data, target)
}

func mapTopologyServer(stats statsResponse, details serverDetails) (domain.Server, error) {
	info := stats.Server
	current := details.Varz
	name := first(current.Name, info.Name, info.ID)
	host := first(current.Host, info.Host)
	version := first(current.Version, info.Version)
	started, observed := stats.Stats.Start, infoTime(stats)
	if !current.Start.IsZero() {
		started = current.Start
	}
	if !current.Now.IsZero() {
		observed = current.Now
	}
	connections, subscriptions, cpu, memory, slow := stats.Stats.Connections, stats.Stats.Subscriptions, stats.Stats.CPU, stats.Stats.Mem, stats.Stats.SlowConsumers
	if current.ID != "" {
		connections, subscriptions, cpu, memory, slow = current.Connections, current.Subscriptions, current.CPU, current.Mem, current.SlowConsumers
	}
	resources := domain.Resources{}
	if stats.Stats.JetStream != nil {
		resources = domain.NewResources(stats.Stats.JetStream.Memory, stats.Stats.JetStream.Store, stats.Stats.JetStream.ReservedMemory, stats.Stats.JetStream.ReservedStore)
	}
	if details.JS.Memory+details.JS.Store+details.JS.ReservedMemory+details.JS.ReservedStore > 0 {
		resources = domain.NewResources(details.JS.Memory, details.JS.Store, details.JS.ReservedMemory, details.JS.ReservedStore)
	}
	links := mapLinks(info.ID, details)
	if len(links) == 0 {
		links = mapStatsLinks(info.ID, stats.Stats)
	}
	return domain.NewServer(domain.ServerInput{ID: info.ID, Name: name, Host: host, Cluster: info.Cluster, Domain: info.Domain, Version: version, Tags: append(info.Tags, current.Tags...), Endpoints: endpoints(current, host), StartedAt: started, ObservedAt: observed, Connections: connections, Subscriptions: subscriptions, CPU: cpu, Memory: memory, SlowConsumers: slow, JetStream: resources, Links: links})
}
func infoTime(stats statsResponse) time.Time {
	if !stats.Server.Time.IsZero() {
		return stats.Server.Time
	}
	return stats.Stats.Start
}
func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
func endpoints(value varzWire, host string) []string {
	result := append([]string(nil), value.ConnectURLs...)
	result = append(result, value.WSConnectURLs...)
	if value.Port > 0 {
		result = append(result, net.JoinHostPort(host, strconv.Itoa(value.Port)))
	}
	return result
}
func parseRTT(value string) time.Duration { duration, _ := time.ParseDuration(value); return duration }
func mapLinks(local string, details serverDetails) []domain.Link {
	var result []domain.Link
	for _, route := range details.Routes.Routes {
		link, err := domain.NewLink(domain.LinkRoute, local, route.RemoteID, route.RemoteName, true, route.Pending, parseRTT(route.RTT))
		if err == nil {
			result = append(result, link)
		}
	}
	for name, gateway := range details.Gateways.Outbound {
		if gateway == nil {
			continue
		}
		pending, rtt := int64(0), time.Duration(0)
		if gateway.Connection != nil {
			pending, rtt = gateway.Connection.Pending, parseRTT(gateway.Connection.RTT)
		}
		link, err := domain.NewLink(domain.LinkGateway, local, name, name, true, pending, rtt)
		if err == nil {
			result = append(result, link)
		}
	}
	for name, gateways := range details.Gateways.Inbound {
		for _, gateway := range gateways {
			if gateway == nil {
				continue
			}
			pending, rtt := int64(0), time.Duration(0)
			if gateway.Connection != nil {
				pending, rtt = gateway.Connection.Pending, parseRTT(gateway.Connection.RTT)
			}
			link, err := domain.NewLink(domain.LinkGateway, local, name, name, true, pending, rtt)
			if err == nil {
				result = append(result, link)
			}
		}
	}
	for _, leaf := range details.Leafs.Leafs {
		remote := strconv.FormatUint(leaf.ID, 10)
		link, err := domain.NewLink(domain.LinkLeaf, local, remote, leaf.Name, true, 0, parseRTT(leaf.RTT))
		if err == nil {
			result = append(result, link)
		}
	}
	return result
}

func mapStatsLinks(local string, stats serverStatsWire) []domain.Link {
	result := make([]domain.Link, 0, len(stats.Routes)+len(stats.Gateways))
	for _, route := range stats.Routes {
		link, err := domain.NewLink(domain.LinkRoute, local, "", route.Name, true, route.Pending, 0)
		if err == nil {
			result = append(result, link)
		}
	}
	for _, gateway := range stats.Gateways {
		link, err := domain.NewLink(domain.LinkGateway, local, "", gateway.Name, true, 0, 0)
		if err == nil {
			result = append(result, link)
		}
	}
	return result
}
func mapMeta(meta metaWire) domain.MetaGroup {
	peers := make([]domain.Peer, 0, len(meta.Replicas))
	for _, peer := range meta.Replicas {
		peers = append(peers, domain.Peer{Name: peer.Name, Current: peer.Current, Offline: peer.Offline, Lag: peer.Lag})
	}
	return domain.MetaGroup{Name: meta.Name, Leader: meta.Leader, Peers: peers}
}

func (m *Monitor) OperateTopology(ctx context.Context, command app.Command) error {
	subject := metaStepDownSubject
	if command.Action == app.ActionReload {
		subject = fmt.Sprintf("$SYS.REQ.SERVER.%s.RELOAD", strings.TrimSpace(command.ServerID))
	}
	message, err := m.connection.RequestWithContext(ctx, subject, nil)
	if err != nil {
		if errors.Is(err, nats.ErrNoResponders) {
			return app.ErrSystemAccountRequired
		}
		return fmt.Errorf("%s topology: %w", command.Action, err)
	}
	var response apiResponse
	if err := json.Unmarshal(message.Data, &response); err != nil {
		return fmt.Errorf("decode %s response: %w", command.Action, err)
	}
	if response.Error != nil {
		return fmt.Errorf("%s: %s", command.Action, response.Error.Description)
	}
	return nil
}
