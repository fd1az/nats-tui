package connection

import (
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"
)

var (
	ErrNameRequired       = errors.New("connection profile name is required")
	ErrServerRequired     = errors.New("at least one NATS server URL is required")
	ErrInvalidServer      = errors.New("invalid NATS server URL")
	ErrInvalidCredentials = errors.New("authentication configuration is incomplete")
)

type AuthKind string

const (
	AuthNone        AuthKind = "none"
	AuthToken       AuthKind = "token"
	AuthCredentials AuthKind = "credentials"
	AuthUserPass    AuthKind = "user-password"
)

type Authentication struct {
	kind        AuthKind
	token       string
	credentials string
	username    string
	password    string
}

func NoAuthentication() Authentication { return Authentication{kind: AuthNone} }

func TokenAuthentication(token string) (Authentication, error) {
	if strings.TrimSpace(token) == "" {
		return Authentication{}, ErrInvalidCredentials
	}
	return Authentication{kind: AuthToken, token: token}, nil
}

func CredentialsAuthentication(path string) (Authentication, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Authentication{}, ErrInvalidCredentials
	}
	return Authentication{kind: AuthCredentials, credentials: path}, nil
}

func UserPasswordAuthentication(username, password string) (Authentication, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return Authentication{}, ErrInvalidCredentials
	}
	return Authentication{kind: AuthUserPass, username: username, password: password}, nil
}

func (a Authentication) Kind() AuthKind      { return a.kind }
func (a Authentication) Token() string       { return a.token }
func (a Authentication) Credentials() string { return a.credentials }
func (a Authentication) Username() string    { return a.username }
func (a Authentication) Password() string    { return a.password }

// Profile is the aggregate root for a reusable NATS connection configuration.
type Profile struct {
	name       string
	servers    []string
	monitorURL string
	auth       Authentication
	lastUsedAt time.Time
}

func NewProfile(name string, servers []string, monitorURL string, auth Authentication) (Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Profile{}, ErrNameRequired
	}
	normalized, err := normalizeServers(servers)
	if err != nil {
		return Profile{}, err
	}
	if auth.kind == "" {
		auth = NoAuthentication()
	}
	if !auth.valid() {
		return Profile{}, ErrInvalidCredentials
	}
	return Profile{name: name, servers: normalized, monitorURL: strings.TrimSpace(monitorURL), auth: auth}, nil
}

// NewDirectProfile preserves the zero-configuration URL workflow.
func NewDirectProfile(name, server string) (Profile, error) {
	if strings.TrimSpace(name) == "" {
		name = profileName(server)
	}
	return NewProfile(name, []string{server}, "", NoAuthentication())
}

func RestoreProfile(name string, servers []string, monitorURL string, auth Authentication, lastUsedAt time.Time) (Profile, error) {
	profile, err := NewProfile(name, servers, monitorURL, auth)
	if err != nil {
		return Profile{}, err
	}
	profile.lastUsedAt = lastUsedAt
	return profile, nil
}

func (p Profile) Name() string                   { return p.name }
func (p Profile) Server() string                 { return strings.Join(p.servers, ",") }
func (p Profile) Servers() []string              { return append([]string(nil), p.servers...) }
func (p Profile) MonitorURL() string             { return p.monitorURL }
func (p Profile) Authentication() Authentication { return p.auth }
func (p Profile) LastUsedAt() time.Time          { return p.lastUsedAt }
func (p *Profile) MarkUsed(at time.Time)         { p.lastUsedAt = at }

func (a Authentication) valid() bool {
	switch a.kind {
	case AuthNone:
		return true
	case AuthToken:
		return a.token != ""
	case AuthCredentials:
		return a.credentials != ""
	case AuthUserPass:
		return a.username != "" && a.password != ""
	default:
		return false
	}
}

func normalizeServers(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, raw := range values {
		for _, candidate := range strings.Split(raw, ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			parsed, err := url.Parse(candidate)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "nats" && parsed.Scheme != "tls" && parsed.Scheme != "ws" && parsed.Scheme != "wss") {
				return nil, errors.Join(ErrInvalidServer, errors.New(candidate))
			}
			if _, exists := seen[candidate]; exists {
				continue
			}
			seen[candidate] = struct{}{}
			result = append(result, candidate)
		}
	}
	if len(result) == 0 {
		return nil, ErrServerRequired
	}
	return result, nil
}

func SortByRecent(profiles []Profile) {
	sort.SliceStable(profiles, func(i, j int) bool {
		if profiles[i].lastUsedAt.Equal(profiles[j].lastUsedAt) {
			return profiles[i].name < profiles[j].name
		}
		return profiles[i].lastUsedAt.After(profiles[j].lastUsedAt)
	})
}

func profileName(server string) string {
	parsed, err := url.Parse(strings.TrimSpace(strings.Split(server, ",")[0]))
	if err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return server
}
