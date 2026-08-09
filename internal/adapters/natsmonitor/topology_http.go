package natsmonitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	appconnections "github.com/clcsolutions/nats-tui/internal/app/connections"
	appoperator "github.com/clcsolutions/nats-tui/internal/app/operator"
	domain "github.com/clcsolutions/nats-tui/internal/domain/topology"
)

const maxMonitoringResponse = 16 << 20

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func (h *httpTopology) OperatorCatalog(ctx context.Context) (appoperator.Catalog, error) {
	var list accountzWire
	if err := h.get(ctx, "/accountz", &list); err != nil {
		return appoperator.Catalog{}, err
	}
	accounts := make([]appoperator.Account, 0, len(list.Accounts))
	claimsRead := false
	for _, id := range list.Accounts {
		var detail accountzWire
		if err := h.get(ctx, "/accountz?acc="+url.QueryEscape(id), &detail); err != nil {
			return appoperator.Catalog{}, err
		}
		if detail.Account != nil {
			mapped := mapAccount(id, *detail.Account)
			claimsRead = claimsRead || len(mapped.DecodedClaim) > 0
			accounts = append(accounts, mapped)
		}
	}
	var usage jszAccountsWire
	if err := h.get(ctx, "/jsz?accounts=true&leader_only=true", &usage); err == nil {
		for _, item := range usage.Accounts {
			accounts = append(accounts, mapAccountUsage(item))
		}
	}
	caps := appoperator.Capabilities{SystemAccount: list.SystemAccount != "", OperatorMode: list.SystemAccount != "" && claimsRead, ClaimsRead: claimsRead}
	return appoperator.NewCatalog(caps, accounts), nil
}

func (h *httpTopology) ListConnections(ctx context.Context) (appconnections.Snapshot, error) {
	var wire connzWire
	if err := h.get(ctx, "/connz?auth=true&subs=detail&limit=10000", &wire); err != nil {
		return appconnections.Snapshot{}, err
	}
	return appconnections.Snapshot{Connections: mapConnections(wire.ServerID, wire.ServerID, wire.Connections), ObservedAt: wire.Now}, nil
}

type httpTopology struct {
	baseURL string
	client  httpDoer
}

func newHTTPTopology(rawURL string, client httpDoer) (*httpTopology, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(rawURL), "/"))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("invalid NATS monitoring URL %q", rawURL)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &httpTopology{baseURL: parsed.String(), client: client}, nil
}

func (h *httpTopology) Topology(ctx context.Context) (domain.Snapshot, error) {
	var details serverDetails
	if err := h.get(ctx, "/varz", &details.Varz); err != nil {
		return domain.Snapshot{}, err
	}
	for path, target := range map[string]any{"/routez": &details.Routes, "/gatewayz": &details.Gateways, "/leafz": &details.Leafs, "/jsz?raft=true": &details.JS} {
		if err := h.get(ctx, path, target); err != nil {
			return domain.Snapshot{}, err
		}
	}
	stats := statsResponse{Server: serverInfoWire{ID: details.Varz.ID, Name: details.Varz.Name, Host: details.Varz.Host, Cluster: details.Varz.Cluster.Name, Domain: details.Varz.JetStream.Config.Domain, Version: details.Varz.Version, Tags: details.Varz.Tags, Time: details.Varz.Now}, Stats: serverStatsWire{Start: details.Varz.Start, Mem: details.Varz.Mem, CPU: details.Varz.CPU, Connections: details.Varz.Connections, Subscriptions: details.Varz.Subscriptions, SlowConsumers: details.Varz.SlowConsumers}}
	server, err := mapTopologyServer(stats, details)
	if err != nil {
		return domain.Snapshot{}, err
	}
	meta := domain.MetaGroup{}
	if details.JS.Meta != nil {
		meta = mapMeta(*details.JS.Meta)
	}
	return domain.NewSnapshot([]domain.Server{server}, meta)
}

func (h *httpTopology) get(ctx context.Context, path string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, h.baseURL+path, nil)
	if err != nil {
		return err
	}
	response, err := h.client.Do(request)
	if err != nil {
		return fmt.Errorf("query NATS monitoring %s: %w", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("NATS monitoring %s returned %s", path, response.Status)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxMonitoringResponse))
	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("NATS monitoring %s returned an empty response", path)
		}
		return fmt.Errorf("decode NATS monitoring %s: %w", path, err)
	}
	return nil
}
