package authority

import (
	"errors"
	"strings"
	"time"
)

type AccountLimits struct {
	Connections, LeafNodes, Subscriptions, Payload, Data, Exports, Imports int64
	Streams, Consumers, Memory, Storage                                    int64
}

type AccountDefinition struct {
	name        string
	description string
	limits      AccountLimits
}

func NewAccountDefinition(name string, limits AccountLimits) (AccountDefinition, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return AccountDefinition{}, errors.New("account name is required")
	}
	for _, value := range []int64{limits.Connections, limits.LeafNodes, limits.Subscriptions, limits.Payload, limits.Data, limits.Exports, limits.Imports, limits.Streams, limits.Consumers, limits.Memory, limits.Storage} {
		if value < -1 {
			return AccountDefinition{}, errors.New("account limits must be -1 (unlimited), zero, or positive")
		}
	}
	return AccountDefinition{name: name, limits: limits}, nil
}

func (d AccountDefinition) WithDescription(description string) AccountDefinition {
	d.description = strings.TrimSpace(description)
	return d
}
func (d AccountDefinition) Name() string          { return d.name }
func (d AccountDefinition) Description() string   { return d.description }
func (d AccountDefinition) Limits() AccountLimits { return d.limits }
func (d AccountDefinition) IsZero() bool          { return d.name == "" }

type Permissions struct {
	PublishAllow, PublishDeny, SubscribeAllow, SubscribeDeny []string
	ResponseMax                                              int
	ResponseTTL                                              time.Duration
	SubscriptionLimit                                        int64
}

type UserDefinition struct {
	name        string
	account     string
	permissions Permissions
}

func NewUserDefinition(name, account string, permissions Permissions) (UserDefinition, error) {
	name, account = strings.TrimSpace(name), strings.TrimSpace(account)
	if name == "" || account == "" {
		return UserDefinition{}, errors.New("user name and account public key are required")
	}
	if permissions.ResponseMax < 0 || permissions.ResponseTTL < 0 || permissions.SubscriptionLimit < -1 {
		return UserDefinition{}, errors.New("user limits cannot be negative")
	}
	for _, subjects := range [][]string{permissions.PublishAllow, permissions.PublishDeny, permissions.SubscribeAllow, permissions.SubscribeDeny} {
		for _, subject := range subjects {
			if !validSubject(subject) {
				return UserDefinition{}, errors.New("permissions contain an invalid subject")
			}
		}
	}
	return UserDefinition{name: name, account: account, permissions: clonePermissions(permissions)}, nil
}

func (d UserDefinition) Name() string             { return d.name }
func (d UserDefinition) Account() string          { return d.account }
func (d UserDefinition) Permissions() Permissions { return clonePermissions(d.permissions) }
func (d UserDefinition) IsZero() bool             { return d.name == "" }

func validSubject(subject string) bool {
	subject = strings.TrimSpace(subject)
	return subject != "" && !strings.ContainsAny(subject, " \t\r\n") && !strings.Contains(subject, "..")
}

func clonePermissions(value Permissions) Permissions {
	value.PublishAllow = append([]string(nil), value.PublishAllow...)
	value.PublishDeny = append([]string(nil), value.PublishDeny...)
	value.SubscribeAllow = append([]string(nil), value.SubscribeAllow...)
	value.SubscribeDeny = append([]string(nil), value.SubscribeDeny...)
	return value
}
