package localstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"time"

	app "github.com/fd1az/nats-tui/internal/app/connections"
	domain "github.com/fd1az/nats-tui/internal/domain/connection"
)

type ProfileRepository struct {
	mu   sync.Mutex
	file StateFile
}

func NewProfileRepository(file StateFile) *ProfileRepository {
	if file == nil {
		panic("nil profile state file")
	}
	return &ProfileRepository{file: file}
}

type profileState struct {
	Profiles []profileDTO `json:"profiles,omitempty"`
}
type profileDTO struct {
	Name          string    `json:"name"`
	Servers       []string  `json:"servers"`
	MonitorURL    string    `json:"monitor_url,omitempty"`
	Auth          string    `json:"auth"`
	Token         string    `json:"token,omitempty"`
	Credentials   string    `json:"credentials,omitempty"`
	Username      string    `json:"username,omitempty"`
	Password      string    `json:"password,omitempty"`
	NKeySeed      string    `json:"nkey_seed,omitempty"`
	UserJWT       string    `json:"user_jwt,omitempty"`
	UserSeed      string    `json:"user_seed,omitempty"`
	TLSCA         string    `json:"tls_ca,omitempty"`
	TLSCert       string    `json:"tls_cert,omitempty"`
	TLSKey        string    `json:"tls_key,omitempty"`
	TLSServerName string    `json:"tls_server_name,omitempty"`
	TLSInsecure   bool      `json:"tls_insecure,omitempty"`
	LastUsedAt    time.Time `json:"last_used_at,omitempty"`
}

func (r *ProfileRepository) Save(_ context.Context, profile domain.Profile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return err
	}
	dto := profileToDTO(profile)
	for i := range state.Profiles {
		if state.Profiles[i].Name == profile.Name() {
			state.Profiles[i] = dto
			return r.save(state)
		}
	}
	state.Profiles = append(state.Profiles, dto)
	return r.save(state)
}

func (r *ProfileRepository) Find(_ context.Context, name string) (domain.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return domain.Profile{}, err
	}
	for _, dto := range state.Profiles {
		if dto.Name == name {
			return dto.profile()
		}
	}
	return domain.Profile{}, app.ErrProfileNotFound
}

func (r *ProfileRepository) List(context.Context) ([]domain.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return nil, err
	}
	result := make([]domain.Profile, 0, len(state.Profiles))
	for _, dto := range state.Profiles {
		profile, mapErr := dto.profile()
		if mapErr != nil {
			return nil, fmt.Errorf("restore profile %q: %w", dto.Name, mapErr)
		}
		result = append(result, profile)
	}
	return result, nil
}

func (r *ProfileRepository) Delete(_ context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := r.load()
	if err != nil {
		return err
	}
	for i := range state.Profiles {
		if state.Profiles[i].Name == name {
			state.Profiles = append(state.Profiles[:i], state.Profiles[i+1:]...)
			return r.save(state)
		}
	}
	return app.ErrProfileNotFound
}

func (r *ProfileRepository) load() (profileState, error) {
	data, err := r.file.Read()
	if errors.Is(err, fs.ErrNotExist) {
		return profileState{}, nil
	}
	if err != nil {
		return profileState{}, fmt.Errorf("read profiles: %w", err)
	}
	var state profileState
	if err := json.Unmarshal(data, &state); err != nil {
		return profileState{}, fmt.Errorf("decode profiles: %w", err)
	}
	return state, nil
}
func (r *ProfileRepository) save(state profileState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := r.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write profiles: %w", err)
	}
	return nil
}
func profileToDTO(profile domain.Profile) profileDTO {
	auth := profile.Authentication()
	tlsConfig := profile.TLS()
	return profileDTO{Name: profile.Name(), Servers: profile.Servers(), MonitorURL: profile.MonitorURL(), Auth: string(auth.Kind()), Token: auth.Token(), Credentials: auth.Credentials(), Username: auth.Username(), Password: auth.Password(), NKeySeed: auth.NKeySeed(), UserJWT: auth.UserJWT(), UserSeed: auth.UserSeed(), TLSCA: tlsConfig.CAFile(), TLSCert: tlsConfig.CertificateFile(), TLSKey: tlsConfig.KeyFile(), TLSServerName: tlsConfig.ServerName(), TLSInsecure: tlsConfig.Insecure(), LastUsedAt: profile.LastUsedAt()}
}
func (d profileDTO) profile() (domain.Profile, error) {
	var auth domain.Authentication
	var err error
	switch domain.AuthKind(d.Auth) {
	case "", domain.AuthNone:
		auth = domain.NoAuthentication()
	case domain.AuthToken:
		auth, err = domain.TokenAuthentication(d.Token)
	case domain.AuthCredentials:
		auth, err = domain.CredentialsAuthentication(d.Credentials)
	case domain.AuthUserPass:
		auth, err = domain.UserPasswordAuthentication(d.Username, d.Password)
	case domain.AuthNKey:
		auth, err = domain.NKeyAuthentication(d.NKeySeed)
	case domain.AuthJWT:
		auth, err = domain.JWTAuthentication(d.UserJWT, d.UserSeed)
	default:
		err = domain.ErrInvalidCredentials
	}
	if err != nil {
		return domain.Profile{}, err
	}
	tlsConfig, err := domain.NewTLS(d.TLSCA, d.TLSCert, d.TLSKey, d.TLSServerName, d.TLSInsecure)
	if err != nil {
		return domain.Profile{}, err
	}
	return domain.RestoreProfile(d.Name, d.Servers, d.MonitorURL, auth, d.LastUsedAt, tlsConfig)
}
