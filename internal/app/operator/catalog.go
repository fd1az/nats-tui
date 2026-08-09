package operator

import (
	"context"
	"encoding/json"
	"sort"
)

type Capabilities struct {
	SystemAccount  bool `json:"system_account"`
	OperatorMode   bool `json:"operator_mode"`
	ClaimsRead     bool `json:"claims_read"`
	ClaimsWrite    bool `json:"claims_write"`
	LocalAuthority bool `json:"local_authority"`
}
type Limits struct {
	Connections   int64 `json:"connections"`
	LeafNodes     int64 `json:"leaf_nodes"`
	Subscriptions int64 `json:"subscriptions"`
	Payload       int64 `json:"payload"`
	Data          int64 `json:"data"`
	Exports       int64 `json:"exports"`
	Imports       int64 `json:"imports"`
	Streams       int64 `json:"streams"`
	Consumers     int64 `json:"consumers"`
	Memory        int64 `json:"memory"`
	Storage       int64 `json:"storage"`
}
type Sharing struct {
	Name    string `json:"name,omitempty"`
	Subject string `json:"subject"`
	Type    string `json:"type"`
	Account string `json:"account,omitempty"`
	Token   string `json:"token,omitempty"`
	Public  bool   `json:"public"`
}
type SigningKey struct {
	PublicKey      string   `json:"public_key"`
	Scoped         bool     `json:"scoped"`
	Role           string   `json:"role,omitempty"`
	Description    string   `json:"description,omitempty"`
	PublishAllow   []string `json:"publish_allow,omitempty"`
	PublishDeny    []string `json:"publish_deny,omitempty"`
	SubscribeAllow []string `json:"subscribe_allow,omitempty"`
	SubscribeDeny  []string `json:"subscribe_deny,omitempty"`
}
type Account struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Description       string          `json:"description,omitempty"`
	Issuer            string          `json:"issuer,omitempty"`
	System            bool            `json:"system"`
	Managed           bool            `json:"managed"`
	Expired           bool            `json:"expired"`
	JetStream         bool            `json:"jetstream"`
	Connections       int             `json:"connections"`
	LeafNodes         int             `json:"leaf_nodes"`
	Subscriptions     int             `json:"subscriptions"`
	Streams           int             `json:"streams"`
	Consumers         int             `json:"consumers"`
	SlowConsumers     int64           `json:"slow_consumers"`
	SentBytes         uint64          `json:"sent_bytes"`
	ReceivedBytes     uint64          `json:"received_bytes"`
	Messages          uint64          `json:"messages"`
	JetStreamMemory   uint64          `json:"jetstream_memory"`
	JetStreamStorage  uint64          `json:"jetstream_storage"`
	Limits            Limits          `json:"limits"`
	SigningKeys       []string        `json:"signing_keys,omitempty"`
	SigningKeyDetails []SigningKey    `json:"signing_key_details,omitempty"`
	Imports           []Sharing       `json:"imports,omitempty"`
	Exports           []Sharing       `json:"exports,omitempty"`
	ActivationTokens  []string        `json:"activation_tokens,omitempty"`
	JWT               string          `json:"jwt,omitempty"`
	DecodedClaim      json.RawMessage `json:"decoded_claim,omitempty"`
}
type User struct {
	PublicKey         string   `json:"public_key"`
	Name              string   `json:"name"`
	Account           string   `json:"account"`
	Issuer            string   `json:"issuer,omitempty"`
	Revoked           bool     `json:"revoked"`
	PublishAllow      []string `json:"publish_allow,omitempty"`
	PublishDeny       []string `json:"publish_deny,omitempty"`
	SubscribeAllow    []string `json:"subscribe_allow,omitempty"`
	SubscribeDeny     []string `json:"subscribe_deny,omitempty"`
	ResponseMax       int      `json:"response_max"`
	ResponseTTL       int64    `json:"response_ttl"`
	SubscriptionLimit int64    `json:"subscription_limit"`
	JWT               string   `json:"jwt,omitempty"`
}
type Catalog struct {
	capabilities Capabilities
	accounts     []Account
	users        []User
}

func NewCatalog(capabilities Capabilities, accounts []Account, users ...[]User) Catalog {
	merged := map[string]Account{}
	for _, account := range accounts {
		current, ok := merged[account.ID]
		if !ok {
			current = cloneAccount(account)
		} else {
			current.Managed = current.Managed || account.Managed
			current.Connections += account.Connections
			current.LeafNodes += account.LeafNodes
			current.Subscriptions += account.Subscriptions
			current.Streams += account.Streams
			current.Consumers += account.Consumers
			current.SlowConsumers += account.SlowConsumers
			current.SentBytes += account.SentBytes
			current.ReceivedBytes += account.ReceivedBytes
			current.Messages += account.Messages
			current.JetStreamMemory += account.JetStreamMemory
			current.JetStreamStorage += account.JetStreamStorage
			if current.JWT == "" && account.JWT != "" {
				mergeAccountDetail(&current, account)
			}
		}
		merged[account.ID] = current
	}
	owned := make([]Account, 0, len(merged))
	for _, account := range merged {
		owned = append(owned, account)
	}
	sort.Slice(owned, func(i, j int) bool { return owned[i].Name < owned[j].Name })
	var managed []User
	if len(users) > 0 {
		managed = make([]User, len(users[0]))
		for i, user := range users[0] {
			managed[i] = cloneUser(user)
		}
		sort.Slice(managed, func(i, j int) bool { return managed[i].Name < managed[j].Name })
	}
	return Catalog{capabilities: capabilities, accounts: owned, users: managed}
}
func (c Catalog) Capabilities() Capabilities { return c.capabilities }
func (c Catalog) Accounts() []Account {
	result := make([]Account, len(c.accounts))
	for i, v := range c.accounts {
		result[i] = cloneAccount(v)
	}
	return result
}
func (c Catalog) Users() []User {
	result := make([]User, len(c.users))
	for i, user := range c.users {
		result[i] = cloneUser(user)
	}
	return result
}

func MergeCatalog(network, managed Catalog) Catalog {
	left, right := network.Capabilities(), managed.Capabilities()
	capabilities := Capabilities{
		SystemAccount:  left.SystemAccount || right.SystemAccount,
		OperatorMode:   left.OperatorMode || right.OperatorMode,
		ClaimsRead:     left.ClaimsRead || right.ClaimsRead,
		ClaimsWrite:    left.ClaimsWrite || right.ClaimsWrite,
		LocalAuthority: left.LocalAuthority || right.LocalAuthority,
	}
	accounts := append(network.Accounts(), managed.Accounts()...)
	users := append(network.Users(), managed.Users()...)
	return NewCatalog(capabilities, accounts, users)
}
func cloneAccount(a Account) Account {
	a.SigningKeys = append([]string(nil), a.SigningKeys...)
	a.SigningKeyDetails = cloneSigningKeys(a.SigningKeyDetails)
	a.Imports = append([]Sharing(nil), a.Imports...)
	a.Exports = append([]Sharing(nil), a.Exports...)
	a.ActivationTokens = append([]string(nil), a.ActivationTokens...)
	a.DecodedClaim = append(json.RawMessage(nil), a.DecodedClaim...)
	return a
}
func cloneSigningKeys(values []SigningKey) []SigningKey {
	result := make([]SigningKey, len(values))
	for i, value := range values {
		value.PublishAllow = append([]string(nil), value.PublishAllow...)
		value.PublishDeny = append([]string(nil), value.PublishDeny...)
		value.SubscribeAllow = append([]string(nil), value.SubscribeAllow...)
		value.SubscribeDeny = append([]string(nil), value.SubscribeDeny...)
		result[i] = value
	}
	return result
}
func cloneUser(user User) User {
	user.PublishAllow = append([]string(nil), user.PublishAllow...)
	user.PublishDeny = append([]string(nil), user.PublishDeny...)
	user.SubscribeAllow = append([]string(nil), user.SubscribeAllow...)
	user.SubscribeDeny = append([]string(nil), user.SubscribeDeny...)
	return user
}
func mergeAccountDetail(target *Account, source Account) {
	target.Name = source.Name
	target.Description = source.Description
	target.Issuer = source.Issuer
	target.System = source.System
	target.Managed = source.Managed
	target.Expired = source.Expired
	target.JetStream = source.JetStream
	target.Limits = source.Limits
	target.SigningKeys = append([]string(nil), source.SigningKeys...)
	target.SigningKeyDetails = cloneSigningKeys(source.SigningKeyDetails)
	target.Imports = append([]Sharing(nil), source.Imports...)
	target.Exports = append([]Sharing(nil), source.Exports...)
	target.ActivationTokens = append([]string(nil), source.ActivationTokens...)
	target.JWT = source.JWT
	target.DecodedClaim = append(json.RawMessage(nil), source.DecodedClaim...)
}

type Source interface {
	OperatorCatalog(context.Context) (Catalog, error)
}
type ListHandler struct{ source Source }

func NewListHandler(source Source) ListHandler {
	if source == nil {
		panic("nil operator catalog source")
	}
	return ListHandler{source}
}
func (h ListHandler) Handle(ctx context.Context) (Catalog, error) {
	return h.source.OperatorCatalog(ctx)
}
