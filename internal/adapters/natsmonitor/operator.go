package natsmonitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	app "github.com/clcsolutions/nats-tui/internal/app/operator"
	appTopology "github.com/clcsolutions/nats-tui/internal/app/topology"
	"github.com/nats-io/nats.go"
	"sort"
)

type sharingWire struct {
	Name     string   `json:"name"`
	Subject  string   `json:"subject"`
	Type     any      `json:"type"`
	Account  string   `json:"account"`
	Token    string   `json:"token"`
	Approved []string `json:"approved_accounts"`
}
type accountInfoWire struct {
	AccountName   string          `json:"account_name"`
	NameTag       string          `json:"name_tag"`
	IsSystem      bool            `json:"is_system"`
	Expired       bool            `json:"expired"`
	JetStream     bool            `json:"jetstream_enabled"`
	Clients       int             `json:"client_connections"`
	Subscriptions int             `json:"subscriptions"`
	JWT           string          `json:"jwt"`
	Issuer        string          `json:"issuer_key"`
	Claim         json.RawMessage `json:"decoded_jwt"`
	Imports       []sharingWire   `json:"imports"`
	Exports       []sharingWire   `json:"exports"`
	Revoked       map[string]any  `json:"revoked_user"`
}
type accountzWire struct {
	ServerID      string           `json:"server_id"`
	SystemAccount string           `json:"system_account"`
	Accounts      []string         `json:"accounts"`
	Account       *accountInfoWire `json:"account_detail"`
}
type accountUsageWire struct {
	Name      string `json:"name"`
	ID        string `json:"id"`
	Memory    uint64 `json:"memory"`
	Storage   uint64 `json:"storage"`
	Streams   int    `json:"streams"`
	Consumers int    `json:"consumers"`
	Messages  uint64 `json:"messages"`
}
type jszAccountsWire struct {
	Accounts []accountUsageWire `json:"account_details"`
}

func (m *Monitor) UpdateAccountClaim(ctx context.Context, token string) error {
	return m.publishClaimRequest(ctx, "$SYS.REQ.CLAIMS.UPDATE", "publish account claim", token)
}

func (m *Monitor) DeleteAccountClaim(ctx context.Context, token string) error {
	return m.publishClaimRequest(ctx, "$SYS.REQ.CLAIMS.DELETE", "delete account claim", token)
}

func (m *Monitor) publishClaimRequest(ctx context.Context, subject, operation, token string) error {
	message, err := m.connection.RequestWithContext(ctx, subject, []byte(token))
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if err := claimUpdateError(message.Data); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func claimUpdateError(payload []byte) error {
	var response struct {
		Error *struct {
			Code        int    `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return fmt.Errorf("decode claim update response: %w", err)
	}
	if response.Error != nil {
		return fmt.Errorf("server rejected claim (%d): %s", response.Error.Code, response.Error.Description)
	}
	return nil
}

func (m *Monitor) OperatorCatalog(ctx context.Context) (app.Catalog, error) {
	topology, err := m.Topology(ctx)
	if err != nil {
		return app.Catalog{}, err
	}
	servers := topology.Servers()
	if len(servers) == 0 {
		return app.NewCatalog(app.Capabilities{}, nil), nil
	}
	var list accountzWire
	if err := m.requestServerJSON(ctx, servers[0].ID().String(), "ACCOUNTZ", nil, &list); err != nil {
		if m.operatorFallback != nil {
			return m.operatorFallback.OperatorCatalog(ctx)
		}
		if errors.Is(err, nats.ErrNoResponders) {
			return app.Catalog{}, appTopology.ErrSystemAccountRequired
		}
		return app.Catalog{}, err
	}
	accounts := make([]app.Account, 0, len(list.Accounts)*len(servers))
	claimsRead := false
	for _, server := range servers {
		for _, id := range list.Accounts {
			payload, _ := json.Marshal(struct {
				Account string `json:"account"`
			}{id})
			var detail accountzWire
			if err := m.requestServerJSON(ctx, server.ID().String(), "ACCOUNTZ", payload, &detail); err != nil {
				continue
			}
			if detail.Account != nil {
				mapped := mapAccount(id, *detail.Account)
				if len(mapped.DecodedClaim) > 0 {
					claimsRead = true
				}
				accounts = append(accounts, mapped)
			}
		}
		var usage jszAccountsWire
		payload := []byte(`{"accounts":true,"leader_only":true}`)
		if err := m.requestServerJSON(ctx, server.ID().String(), "JSZ", payload, &usage); err == nil {
			for _, item := range usage.Accounts {
				accounts = append(accounts, mapAccountUsage(item))
			}
		}
	}
	caps := app.Capabilities{SystemAccount: list.SystemAccount != "", OperatorMode: list.SystemAccount != "" && claimsRead, ClaimsRead: claimsRead}
	return app.NewCatalog(caps, accounts), nil
}

func mapAccountUsage(wire accountUsageWire) app.Account {
	return app.Account{ID: wire.ID, Name: wire.Name, JetStream: true, Streams: wire.Streams, Consumers: wire.Consumers, Messages: wire.Messages, JetStreamMemory: wire.Memory, JetStreamStorage: wire.Storage}
}
func mapAccount(id string, wire accountInfoWire) app.Account {
	name := wire.NameTag
	if name == "" {
		name = wire.AccountName
	}
	if name == "" {
		name = id
	}
	imports := mapSharing(wire.Imports, false)
	exports := mapSharing(wire.Exports, true)
	signing, details, limits, activations := claimSummary(wire.Claim)
	return app.Account{ID: id, Name: name, Description: claimDescription(wire.Claim), Issuer: wire.Issuer, System: wire.IsSystem, Expired: wire.Expired, JetStream: wire.JetStream, Connections: wire.Clients, Subscriptions: wire.Subscriptions, Limits: limits, SigningKeys: signing, SigningKeyDetails: details, Imports: imports, Exports: exports, ActivationTokens: activations, JWT: wire.JWT, DecodedClaim: append(json.RawMessage(nil), wire.Claim...)}
}

func claimDescription(raw json.RawMessage) string {
	var claim struct {
		Description string `json:"description"`
	}
	_ = json.Unmarshal(raw, &claim)
	return claim.Description
}
func mapSharing(values []sharingWire, export bool) []app.Sharing {
	result := make([]app.Sharing, 0, len(values))
	for _, value := range values {
		kind := fmt.Sprint(value.Type)
		public := export && len(value.Approved) == 0
		result = append(result, app.Sharing{Name: value.Name, Subject: value.Subject, Type: kind, Account: value.Account, Token: value.Token, Public: public})
	}
	return result
}
func claimSummary(raw json.RawMessage) ([]string, []app.SigningKey, app.Limits, []string) {
	if len(raw) == 0 {
		return nil, nil, app.Limits{}, nil
	}
	var claim map[string]any
	if json.Unmarshal(raw, &claim) != nil {
		return nil, nil, app.Limits{}, nil
	}
	natsData, _ := claim["nats"].(map[string]any)
	signing, signingDetails := signingKeyValues(natsData["signing_keys"])
	limitsMap, _ := natsData["limits"].(map[string]any)
	limits := app.Limits{Connections: number(limitsMap, "conn"), LeafNodes: number(limitsMap, "leaf"), Subscriptions: number(limitsMap, "subs"), Payload: number(limitsMap, "payload"), Data: number(limitsMap, "data"), Exports: number(limitsMap, "exports"), Imports: number(limitsMap, "imports"), Streams: number(limitsMap, "streams"), Consumers: number(limitsMap, "consumer"), Memory: number(limitsMap, "mem_storage"), Storage: number(limitsMap, "disk_storage")}
	activations := tokensFromImports(natsData["imports"])
	return signing, signingDetails, limits, activations
}

func signingKeyValues(value any) ([]string, []app.SigningKey) {
	items, _ := value.([]any)
	keys, details := make([]string, 0, len(items)), make([]app.SigningKey, 0, len(items))
	for _, item := range items {
		switch typed := item.(type) {
		case string:
			keys = append(keys, typed)
			details = append(details, app.SigningKey{PublicKey: typed})
		case map[string]any:
			key, _ := typed["key"].(string)
			if key == "" {
				continue
			}
			detail := app.SigningKey{PublicKey: key, Scoped: true}
			detail.Role, _ = typed["role"].(string)
			detail.Description, _ = typed["description"].(string)
			template, _ := typed["template"].(map[string]any)
			pub, _ := template["pub"].(map[string]any)
			sub, _ := template["sub"].(map[string]any)
			detail.PublishAllow, detail.PublishDeny = stringValues(pub["allow"]), stringValues(pub["deny"])
			detail.SubscribeAllow, detail.SubscribeDeny = stringValues(sub["allow"]), stringValues(sub["deny"])
			keys, details = append(keys, key), append(details, detail)
		}
	}
	sort.Strings(keys)
	sort.Slice(details, func(i, j int) bool { return details[i].PublicKey < details[j].PublicKey })
	return keys, details
}
func stringValues(value any) []string {
	var result []string
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
	case map[string]any:
		for key := range typed {
			result = append(result, key)
		}
	}
	sort.Strings(result)
	return result
}
func number(values map[string]any, key string) int64 {
	if values == nil {
		return 0
	}
	value, _ := values[key].(float64)
	return int64(value)
}
func tokensFromImports(value any) []string {
	items, _ := value.([]any)
	var result []string
	for _, item := range items {
		entry, _ := item.(map[string]any)
		if token, ok := entry["token"].(string); ok && token != "" {
			result = append(result, token)
		}
	}
	return result
}
