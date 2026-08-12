package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	app "github.com/fd1az/nats-tui/internal/app/connections"
	"github.com/fd1az/nats-tui/internal/app/subjects"
	"github.com/fd1az/nats-tui/internal/config"
)

type profileManagerStub struct {
	saved app.ProfileInput
	items []app.ProfileSummary
}

func (s *profileManagerStub) SaveConnectionProfile(_ context.Context, input app.ProfileInput) error {
	s.saved = input
	return nil
}
func (s *profileManagerStub) ListConnectionProfiles(context.Context) ([]app.ProfileSummary, error) {
	return s.items, nil
}
func (s *profileManagerStub) DeleteConnectionProfile(context.Context, string) error { return nil }
func (s *profileManagerStub) TestConnectionProfile(context.Context, string) (app.ProfileTestResult, error) {
	return app.ProfileTestResult{Profile: "prod", ServerName: "nats-a", Version: "2.14.4", Latency: 5 * time.Millisecond}, nil
}
func (s *profileManagerStub) ResolveConnectionProfile(_ context.Context, name string, base config.Settings) (config.Settings, error) {
	base.Profile, base.Server, base.Username, base.Password = name, "nats://prod:4222", "ops", "secret"
	return base, nil
}
func (s *profileManagerStub) ImportNATSContexts(context.Context, string, bool) (app.ImportResult, error) {
	return app.ImportResult{Imported: []string{"prod"}}, nil
}

func TestProfileSaveAndSanitizedList(t *testing.T) {
	service := &profileManagerStub{items: []app.ProfileSummary{{Name: "prod", Servers: []string{"nats://a:4222", "nats://b:4222"}, Auth: "user-password", Username: "ops"}}}
	root := NewRoot(Dependencies{Profiles: service})
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetArgs([]string{"profile", "save", "prod", "--server-url", "nats://a:4222,nats://b:4222", "--auth-user", "ops", "--auth-password", "secret"})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if service.saved.Password != "secret" || len(service.saved.Servers) != 2 {
		t.Fatalf("saved=%+v", service.saved)
	}
	output.Reset()
	root = NewRoot(Dependencies{Profiles: service})
	root.SetOut(output)
	root.SetArgs([]string{"profile", "export"})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "secret") || !strings.Contains(output.String(), "prod") {
		t.Fatalf("output=%s", output)
	}
}

func TestExplicitProfileResolvesBeforeOperationalCommand(t *testing.T) {
	service := &profileManagerStub{}
	root := NewRoot(Dependencies{Profiles: service, Subjects: settingsSubjectExplorer{}})
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetArgs([]string{"--profile", "prod", "subject", "list", "--output", "plain"})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestProfileImportCommandReportsImportedContexts(t *testing.T) {
	service := &profileManagerStub{}
	root := NewRoot(Dependencies{Profiles: service})
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetArgs([]string{"profile", "import-nats", "--directory", "/contexts"})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "imported 1") || !strings.Contains(output.String(), "prod") {
		t.Fatalf("output=%s", output)
	}
}

type settingsSubjectExplorer struct{}

func (settingsSubjectExplorer) ExploreSubjects(_ context.Context, settings config.Settings) ([]subjects.Node, error) {
	if settings.Server != "nats://prod:4222" || settings.Username != "ops" {
		return nil, errors.New("profile was not resolved")
	}
	return nil, nil
}
