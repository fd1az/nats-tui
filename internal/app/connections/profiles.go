package connections

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	domain "github.com/fd1az/nats-tui/internal/domain/connection"
)

var ErrProfileNotFound = errors.New("connection profile not found")

type Clock interface{ Now() time.Time }

type ProfileRepository interface {
	Save(context.Context, domain.Profile) error
	Find(context.Context, string) (domain.Profile, error)
	List(context.Context) ([]domain.Profile, error)
	Delete(context.Context, string) error
}

type ContextSource interface {
	Profiles(context.Context, string) ([]ProfileInput, error)
}

type ImportResult struct{ Imported, Skipped []string }

type ProfileSummary struct {
	Name        string
	Servers     []string
	MonitorURL  string
	Auth        domain.AuthKind
	Username    string
	TLS         bool
	TLSServer   string
	InsecureTLS bool
	LastUsedAt  time.Time
}

type ProfileTestResult struct {
	Profile, ServerName, Cluster, Version string
	Latency                               time.Duration
	Streams, Consumers                    int
}

type ProfileInput struct {
	Name, MonitorURL, Token, Credentials, Username, Password string
	NKeySeed, UserJWT, UserSeed                              string
	TLSCA, TLSCert, TLSKey, TLSServerName                    string
	TLSInsecure                                              bool
	Servers                                                  []string
}

func (i ProfileInput) Profile() (domain.Profile, error) {
	configured := 0
	if i.Token != "" {
		configured++
	}
	if i.Credentials != "" {
		configured++
	}
	if i.Username != "" || i.Password != "" {
		configured++
	}
	if i.NKeySeed != "" {
		configured++
	}
	if i.UserJWT != "" || i.UserSeed != "" {
		configured++
	}
	if configured > 1 {
		return domain.Profile{}, domain.ErrInvalidCredentials
	}
	auth := domain.NoAuthentication()
	var err error
	switch {
	case i.Token != "":
		auth, err = domain.TokenAuthentication(i.Token)
	case i.Credentials != "":
		auth, err = domain.CredentialsAuthentication(i.Credentials)
	case i.Username != "" || i.Password != "":
		auth, err = domain.UserPasswordAuthentication(i.Username, i.Password)
	case i.NKeySeed != "":
		auth, err = domain.NKeyAuthentication(i.NKeySeed)
	case i.UserJWT != "" || i.UserSeed != "":
		auth, err = domain.JWTAuthentication(i.UserJWT, i.UserSeed)
	}
	if err != nil {
		return domain.Profile{}, err
	}
	tlsConfig, err := domain.NewTLS(i.TLSCA, i.TLSCert, i.TLSKey, i.TLSServerName, i.TLSInsecure)
	if err != nil {
		return domain.Profile{}, err
	}
	return domain.NewProfile(i.Name, i.Servers, i.MonitorURL, auth, tlsConfig)
}

type ProfileService struct {
	repository ProfileRepository
	clock      Clock
	contexts   ContextSource
}

func NewProfileImportService(repository ProfileRepository, clock Clock, contexts ContextSource) ProfileService {
	service := NewProfileService(repository, clock)
	if contexts == nil {
		panic("nil NATS context source")
	}
	service.contexts = contexts
	return service
}

func (s ProfileService) ImportNATSContexts(ctx context.Context, directory string, replace bool) (ImportResult, error) {
	if s.contexts == nil {
		return ImportResult{}, errors.New("NATS context import is unavailable")
	}
	inputs, err := s.contexts.Profiles(ctx, directory)
	if err != nil {
		return ImportResult{}, err
	}
	result := ImportResult{}
	for _, input := range inputs {
		if !replace {
			_, findErr := s.repository.Find(ctx, input.Name)
			switch {
			case findErr == nil:
				result.Skipped = append(result.Skipped, input.Name)
				continue
			case !errors.Is(findErr, ErrProfileNotFound):
				return result, findErr
			}
		}
		if err := s.Save(ctx, input); err != nil {
			return result, errors.Join(errors.New("import NATS context "+input.Name), err)
		}
		result.Imported = append(result.Imported, input.Name)
	}
	sort.Strings(result.Imported)
	sort.Strings(result.Skipped)
	return result, nil
}

func NewProfileService(repository ProfileRepository, clock Clock) ProfileService {
	if repository == nil || clock == nil {
		panic("connection profile dependencies cannot be nil")
	}
	return ProfileService{repository: repository, clock: clock}
}

func (s ProfileService) Save(ctx context.Context, input ProfileInput) error {
	profile, err := input.Profile()
	if err != nil {
		return err
	}
	return s.repository.Save(ctx, profile)
}

func (s ProfileService) List(ctx context.Context) ([]ProfileSummary, error) {
	profiles, err := s.repository.List(ctx)
	if err != nil {
		return nil, err
	}
	domain.SortByRecent(profiles)
	result := make([]ProfileSummary, 0, len(profiles))
	for _, profile := range profiles {
		auth := profile.Authentication()
		tlsConfig := profile.TLS()
		result = append(result, ProfileSummary{Name: profile.Name(), Servers: profile.Servers(), MonitorURL: profile.MonitorURL(), Auth: auth.Kind(), Username: auth.Username(), TLS: tlsConfig.Enabled(), TLSServer: tlsConfig.ServerName(), InsecureTLS: tlsConfig.Insecure(), LastUsedAt: profile.LastUsedAt()})
	}
	return result, nil
}

func (s ProfileService) Find(ctx context.Context, name string) (domain.Profile, error) {
	return s.repository.Find(ctx, strings.TrimSpace(name))
}

func (s ProfileService) Delete(ctx context.Context, name string) error {
	return s.repository.Delete(ctx, strings.TrimSpace(name))
}

func (s ProfileService) MarkUsed(ctx context.Context, name string) error {
	profile, err := s.repository.Find(ctx, name)
	if err != nil {
		return err
	}
	profile.MarkUsed(s.clock.Now())
	return s.repository.Save(ctx, profile)
}

func SortSummaries(items []ProfileSummary) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].LastUsedAt.Equal(items[j].LastUsedAt) {
			return items[i].Name < items[j].Name
		}
		return items[i].LastUsedAt.After(items[j].LastUsedAt)
	})
}
