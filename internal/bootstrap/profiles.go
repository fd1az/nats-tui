package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	app "github.com/clcsolutions/nats-tui/internal/app/connections"
	"github.com/clcsolutions/nats-tui/internal/config"
	domain "github.com/clcsolutions/nats-tui/internal/domain/connection"
)

func (r Runner) SaveConnectionProfile(ctx context.Context, input app.ProfileInput) error {
	return r.profileService().Save(ctx, input)
}
func (r Runner) ListConnectionProfiles(ctx context.Context) ([]app.ProfileSummary, error) {
	return r.profileService().List(ctx)
}
func (r Runner) DeleteConnectionProfile(ctx context.Context, name string) error {
	return r.profileService().Delete(ctx, name)
}
func (r Runner) ImportNATSContexts(ctx context.Context, directory string, replace bool) (app.ImportResult, error) {
	if directory == "" {
		configDirectory, err := os.UserConfigDir()
		if err != nil {
			return app.ImportResult{}, fmt.Errorf("find NATS context directory: %w", err)
		}
		directory = filepath.Join(configDirectory, "nats", "context")
	}
	return r.profileService().ImportNATSContexts(ctx, directory, replace)
}
func (r Runner) ResolveConnectionProfile(ctx context.Context, name string, base config.Settings) (config.Settings, error) {
	profile, err := r.profileService().Find(ctx, name)
	if err != nil {
		return config.Settings{}, err
	}
	resolved := settingsFromProfile(profile)
	resolved.Refresh, resolved.AuthorityDir, resolved.OperatorSeed = base.Refresh, base.AuthorityDir, base.OperatorSeed
	return resolved, nil
}
func (r Runner) TestConnectionProfile(ctx context.Context, name string) (app.ProfileTestResult, error) {
	profile, err := r.profileService().Find(ctx, name)
	if err != nil {
		return app.ProfileTestResult{}, err
	}
	settings := settingsFromProfile(profile)
	started := r.clock.Now()
	monitor, err := r.connector()(settings)
	if err != nil {
		return app.ProfileTestResult{}, fmt.Errorf("profile %q transport/authentication check failed: %w", name, err)
	}
	defer monitor.Close()
	snapshot, err := monitor.Snapshot(ctx)
	if err != nil {
		return app.ProfileTestResult{}, fmt.Errorf("profile %q connected but JetStream/account check failed: %w", name, err)
	}
	if err := r.profileService().MarkUsed(ctx, name); err != nil {
		return app.ProfileTestResult{}, err
	}
	return app.ProfileTestResult{Profile: name, ServerName: snapshot.ServerName, Cluster: snapshot.Cluster, Version: snapshot.Version, Latency: r.clock.Now().Sub(started), Streams: snapshot.Streams, Consumers: snapshot.Consumers}, nil
}

func settingsFromProfile(profile domain.Profile) config.Settings {
	auth := profile.Authentication()
	tlsConfig := profile.TLS()
	return config.Settings{Profile: profile.Name(), Server: profile.Server(), MonitorURL: profile.MonitorURL(), Credentials: auth.Credentials(), Token: auth.Token(), Username: auth.Username(), Password: auth.Password(), NKeySeed: auth.NKeySeed(), UserJWT: auth.UserJWT(), UserSeed: auth.UserSeed(), TLSCA: tlsConfig.CAFile(), TLSCert: tlsConfig.CertificateFile(), TLSKey: tlsConfig.KeyFile(), TLSServerName: tlsConfig.ServerName(), TLSInsecure: tlsConfig.Insecure(), Refresh: 2 * time.Second}
}
