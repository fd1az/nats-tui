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
	ErrServerCredentials  = errors.New("server URL must not contain credentials; use an authentication field")
	ErrInvalidCredentials = errors.New("authentication configuration is incomplete")
)

type AuthKind string

const (
	AuthNone        AuthKind = "none"
	AuthToken       AuthKind = "token"
	AuthCredentials AuthKind = "credentials"
	AuthUserPass    AuthKind = "user-password"
	AuthNKey        AuthKind = "nkey"
	AuthJWT         AuthKind = "jwt"
)

type Authentication struct {
	kind        AuthKind
	token       string
	credentials string
	username    string
	password    string
	nkeySeed    string
	userJWT     string
	userSeed    string
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

func NKeyAuthentication(seedFile string) (Authentication, error) {
	seedFile = strings.TrimSpace(seedFile)
	if seedFile == "" {
		return Authentication{}, ErrInvalidCredentials
	}
	return Authentication{kind: AuthNKey, nkeySeed: seedFile}, nil
}

func JWTAuthentication(jwtFile, seedFile string) (Authentication, error) {
	jwtFile, seedFile = strings.TrimSpace(jwtFile), strings.TrimSpace(seedFile)
	if jwtFile == "" || seedFile == "" {
		return Authentication{}, ErrInvalidCredentials
	}
	return Authentication{kind: AuthJWT, userJWT: jwtFile, userSeed: seedFile}, nil
}

func (a Authentication) Kind() AuthKind      { return a.kind }
func (a Authentication) Token() string       { return a.token }
func (a Authentication) Credentials() string { return a.credentials }
func (a Authentication) Username() string    { return a.username }
func (a Authentication) Password() string    { return a.password }
func (a Authentication) NKeySeed() string    { return a.nkeySeed }
func (a Authentication) UserJWT() string     { return a.userJWT }
func (a Authentication) UserSeed() string    { return a.userSeed }

type TLS struct {
	caFile, certificateFile, keyFile, serverName string
	insecure                                     bool
}

func NewTLS(caFile, certificateFile, keyFile, serverName string, insecure bool) (TLS, error) {
	caFile, certificateFile, keyFile, serverName = strings.TrimSpace(caFile), strings.TrimSpace(certificateFile), strings.TrimSpace(keyFile), strings.TrimSpace(serverName)
	if (certificateFile == "") != (keyFile == "") {
		return TLS{}, errors.New("TLS certificate and key must be configured together")
	}
	return TLS{caFile: caFile, certificateFile: certificateFile, keyFile: keyFile, serverName: serverName, insecure: insecure}, nil
}
func (t TLS) CAFile() string          { return t.caFile }
func (t TLS) CertificateFile() string { return t.certificateFile }
func (t TLS) KeyFile() string         { return t.keyFile }
func (t TLS) ServerName() string      { return t.serverName }
func (t TLS) Insecure() bool          { return t.insecure }
func (t TLS) Enabled() bool {
	return t.caFile != "" || t.certificateFile != "" || t.serverName != "" || t.insecure
}

// Profile is the aggregate root for a reusable NATS connection configuration.
type Profile struct {
	name       string
	servers    []string
	monitorURL string
	auth       Authentication
	tls        TLS
	lastUsedAt time.Time
}

func NewProfile(name string, servers []string, monitorURL string, auth Authentication, tlsConfig ...TLS) (Profile, error) {
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
	var tls TLS
	if len(tlsConfig) > 0 {
		tls = tlsConfig[0]
	}
	return Profile{name: name, servers: normalized, monitorURL: strings.TrimSpace(monitorURL), auth: auth, tls: tls}, nil
}

// NewDirectProfile preserves the zero-configuration URL workflow.
func NewDirectProfile(name, server string) (Profile, error) {
	if strings.TrimSpace(name) == "" {
		name = profileName(server)
	}
	return NewProfile(name, []string{server}, "", NoAuthentication())
}

func RestoreProfile(name string, servers []string, monitorURL string, auth Authentication, lastUsedAt time.Time, tlsConfig ...TLS) (Profile, error) {
	profile, err := NewProfile(name, servers, monitorURL, auth, tlsConfig...)
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
func (p Profile) TLS() TLS                       { return p.tls }
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
	case AuthNKey:
		return a.nkeySeed != ""
	case AuthJWT:
		return a.userJWT != "" && a.userSeed != ""
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
				return nil, ErrInvalidServer
			}
			if parsed.User != nil {
				return nil, ErrServerCredentials
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
