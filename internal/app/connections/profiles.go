package connections

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	domain "github.com/clcsolutions/nats-tui/internal/domain/connection"
)

var ErrProfileNotFound = errors.New("connection profile not found")

type Clock interface{ Now() time.Time }

type ProfileRepository interface {
	Save(context.Context, domain.Profile) error
	Find(context.Context, string) (domain.Profile, error)
	List(context.Context) ([]domain.Profile, error)
	Delete(context.Context, string) error
}

type ProfileSummary struct {
	Name       string
	Servers    []string
	MonitorURL string
	Auth       domain.AuthKind
	Username   string
	LastUsedAt time.Time
}

type ProfileTestResult struct {
	Profile, ServerName, Cluster, Version string
	Latency                               time.Duration
	Streams, Consumers                    int
}

type ProfileInput struct {
	Name, MonitorURL, Token, Credentials, Username, Password string
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
	}
	if err != nil {
		return domain.Profile{}, err
	}
	return domain.NewProfile(i.Name, i.Servers, i.MonitorURL, auth)
}

type ProfileService struct {
	repository ProfileRepository
	clock      Clock
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
		result = append(result, ProfileSummary{Name: profile.Name(), Servers: profile.Servers(), MonitorURL: profile.MonitorURL(), Auth: auth.Kind(), Username: auth.Username(), LastUsedAt: profile.LastUsedAt()})
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
