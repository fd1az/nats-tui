package connections

import (
	"context"
	"testing"
	"time"

	domain "github.com/clcsolutions/nats-tui/internal/domain/connection"
)

type profileMemory struct{ values map[string]domain.Profile }

func (m *profileMemory) Save(_ context.Context, p domain.Profile) error {
	if m.values == nil {
		m.values = map[string]domain.Profile{}
	}
	m.values[p.Name()] = p
	return nil
}
func (m *profileMemory) Find(_ context.Context, n string) (domain.Profile, error) {
	p, ok := m.values[n]
	if !ok {
		return domain.Profile{}, ErrProfileNotFound
	}
	return p, nil
}
func (m *profileMemory) List(context.Context) ([]domain.Profile, error) {
	result := make([]domain.Profile, 0, len(m.values))
	for _, p := range m.values {
		result = append(result, p)
	}
	return result, nil
}
func (m *profileMemory) Delete(_ context.Context, n string) error {
	if _, ok := m.values[n]; !ok {
		return ErrProfileNotFound
	}
	delete(m.values, n)
	return nil
}

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

type contextSourceStub struct{ values []ProfileInput }

func (s contextSourceStub) Profiles(context.Context, string) ([]ProfileInput, error) {
	return append([]ProfileInput(nil), s.values...), nil
}

func TestProfileServiceKeepsSecretsOutOfListAndTracksRecent(t *testing.T) {
	repository := &profileMemory{}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	service := NewProfileService(repository, fixedClock{now})
	if err := service.Save(t.Context(), ProfileInput{Name: "prod", Servers: []string{"nats://prod:4222"}, Username: "ops", Password: "secret"}); err != nil {
		t.Fatal(err)
	}
	if err := service.MarkUsed(t.Context(), "prod"); err != nil {
		t.Fatal(err)
	}
	items, err := service.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Auth != domain.AuthUserPass || items[0].Username != "ops" || !items[0].LastUsedAt.Equal(now) {
		t.Fatalf("items=%+v", items)
	}
}

func TestImportNATSContextsSkipsExistingUnlessReplaceRequested(t *testing.T) {
	repository := &profileMemory{}
	existing, _ := domain.NewProfile("prod", []string{"nats://old:4222"}, "", domain.NoAuthentication())
	_ = repository.Save(t.Context(), existing)
	source := contextSourceStub{values: []ProfileInput{{Name: "prod", Servers: []string{"nats://new:4222"}}, {Name: "dev", Servers: []string{"nats://dev:4222"}}}}
	service := NewProfileImportService(repository, fixedClock{}, source)
	result, err := service.ImportNATSContexts(t.Context(), "contexts", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Imported) != 1 || result.Imported[0] != "dev" || len(result.Skipped) != 1 {
		t.Fatalf("result=%+v", result)
	}
	prod, _ := repository.Find(t.Context(), "prod")
	if prod.Server() != "nats://old:4222" {
		t.Fatalf("prod=%s", prod.Server())
	}
	if _, err := service.ImportNATSContexts(t.Context(), "contexts", true); err != nil {
		t.Fatal(err)
	}
	prod, _ = repository.Find(t.Context(), "prod")
	if prod.Server() != "nats://new:4222" {
		t.Fatalf("prod=%s", prod.Server())
	}
}
