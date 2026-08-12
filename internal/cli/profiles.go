package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	app "github.com/fd1az/nats-tui/internal/app/connections"
	"github.com/spf13/cobra"
)

func newProfileCommand(service ProfileManager) *cobra.Command {
	command := &cobra.Command{Use: "profile", Short: "Manage reusable NATS connection profiles", Args: cobra.NoArgs}
	command.AddCommand(newProfileSaveCommand(service), newProfileListCommand(service), newProfileDeleteCommand(service), newProfileTestCommand(service), newProfileExportCommand(service), newProfileImportCommand(service))
	return command
}

func newProfileSaveCommand(service ProfileManager) *cobra.Command {
	var servers []string
	var monitorURL, token, credentials, username, password, nkeySeed, userJWT, userSeed string
	var tlsCA, tlsCert, tlsKey, tlsServerName string
	var tlsInsecure bool
	command := &cobra.Command{Use: "save NAME", Short: "Create or replace a connection profile", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return errors.New("profile manager is unavailable")
		}
		if err := service.SaveConnectionProfile(cmd.Context(), app.ProfileInput{Name: args[0], Servers: servers, MonitorURL: monitorURL, Token: token, Credentials: credentials, Username: username, Password: password, NKeySeed: nkeySeed, UserJWT: userJWT, UserSeed: userSeed, TLSCA: tlsCA, TLSCert: tlsCert, TLSKey: tlsKey, TLSServerName: tlsServerName, TLSInsecure: tlsInsecure}); err != nil {
			return err
		}
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "saved profile %s\n", args[0])
		return err
	}}
	flags := command.Flags()
	flags.StringSliceVar(&servers, "server-url", nil, "NATS server URL; repeat or comma-separate for failover")
	flags.StringVar(&monitorURL, "monitor-url", "", "optional NATS monitoring URL")
	flags.StringVar(&token, "auth-token", "", "authentication token")
	flags.StringVar(&credentials, "credentials-file", "", "NATS credentials file")
	flags.StringVar(&username, "auth-user", "", "authentication username")
	flags.StringVar(&password, "auth-password", "", "authentication password")
	flags.StringVar(&nkeySeed, "nkey-seed", "", "NKey seed file")
	flags.StringVar(&userJWT, "user-jwt", "", "user JWT file")
	flags.StringVar(&userSeed, "user-seed", "", "user NKey seed file")
	flags.StringVar(&tlsCA, "tls-ca", "", "TLS CA certificate file")
	flags.StringVar(&tlsCert, "tls-cert", "", "TLS client certificate file")
	flags.StringVar(&tlsKey, "tls-key", "", "TLS client private key file")
	flags.StringVar(&tlsServerName, "tls-server-name", "", "TLS server name override")
	flags.BoolVar(&tlsInsecure, "tls-insecure", false, "skip TLS verification (unsafe)")
	_ = command.MarkFlagRequired("server-url")
	command.MarkFlagsMutuallyExclusive("auth-token", "credentials-file", "auth-user", "nkey-seed", "user-jwt")
	return command
}

func newProfileImportCommand(service ProfileManager) *cobra.Command {
	var directory string
	var replace bool
	command := &cobra.Command{Use: "import-nats", Short: "Import profiles from NATS CLI JSON contexts", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if service == nil {
			return errors.New("profile manager is unavailable")
		}
		result, err := service.ImportNATSContexts(cmd.Context(), directory, replace)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "imported %d profile(s), skipped %d existing: %s\n", len(result.Imported), len(result.Skipped), strings.Join(result.Imported, ", "))
		return err
	}}
	command.Flags().StringVar(&directory, "directory", "", "NATS context directory (default: user config nats/context)")
	command.Flags().BoolVar(&replace, "replace", false, "replace existing profiles with matching names")
	return command
}

func newProfileListCommand(service ProfileManager) *cobra.Command {
	var output string
	command := &cobra.Command{Use: "list", Short: "List profiles, most recently used first", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if service == nil {
			return errors.New("profile manager is unavailable")
		}
		items, err := service.ListConnectionProfiles(cmd.Context())
		if err != nil {
			return err
		}
		return writeProfiles(cmd, items, output)
	}}
	command.Flags().StringVarP(&output, "output", "o", "table", "output: table, json, or plain")
	return command
}

func newProfileDeleteCommand(service ProfileManager) *cobra.Command {
	var yes bool
	command := &cobra.Command{Use: "delete NAME", Short: "Delete a saved profile", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return errors.New("profile manager is unavailable")
		}
		if !yes {
			return errors.New("profile deletion requires --yes")
		}
		return service.DeleteConnectionProfile(cmd.Context(), args[0])
	}}
	command.Flags().BoolVar(&yes, "yes", false, "confirm deletion")
	return command
}

func newProfileTestCommand(service ProfileManager) *cobra.Command {
	return &cobra.Command{Use: "test NAME", Short: "Test transport, authentication, and JetStream access", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return errors.New("profile manager is unavailable")
		}
		result, err := service.TestConnectionProfile(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "✓ %s connected to %s  NATS %s  latency %s  streams %d  consumers %d\n", result.Profile, result.ServerName, result.Version, result.Latency.Round(1_000_000), result.Streams, result.Consumers)
		return err
	}}
}

func newProfileExportCommand(service ProfileManager) *cobra.Command {
	return &cobra.Command{Use: "export", Short: "Export sanitized profiles without secrets", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if service == nil {
			return errors.New("profile manager is unavailable")
		}
		items, err := service.ListConnectionProfiles(cmd.Context())
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(profileOutputs(items))
	}}
}

func writeProfiles(cmd *cobra.Command, items []app.ProfileSummary, output string) error {
	switch strings.ToLower(output) {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(profileOutputs(items))
	case "plain":
		for _, item := range items {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), item.Name); err != nil {
				return err
			}
		}
		return nil
	case "table":
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "NAME\tSERVERS\tAUTH\tLAST USED"); err != nil {
			return err
		}
		for _, item := range items {
			last := "never"
			if !item.LastUsedAt.IsZero() {
				last = item.LastUsedAt.Format("2006-01-02 15:04:05")
			}
			security := string(item.Auth)
			if item.TLS {
				security += "+tls"
			}
			if item.InsecureTLS {
				security += "!"
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", item.Name, strings.Join(item.Servers, ","), security, last); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported output %q", output)
	}
}

type profileOutput struct {
	Name        string   `json:"name"`
	Servers     []string `json:"servers"`
	MonitorURL  string   `json:"monitor_url,omitempty"`
	Auth        string   `json:"auth"`
	Username    string   `json:"username,omitempty"`
	TLS         bool     `json:"tls"`
	TLSServer   string   `json:"tls_server_name,omitempty"`
	InsecureTLS bool     `json:"tls_insecure,omitempty"`
	LastUsedAt  string   `json:"last_used_at,omitempty"`
}

func profileOutputs(items []app.ProfileSummary) []profileOutput {
	result := make([]profileOutput, 0, len(items))
	for _, item := range items {
		last := ""
		if !item.LastUsedAt.IsZero() {
			last = item.LastUsedAt.Format(time.RFC3339)
		}
		result = append(result, profileOutput{Name: item.Name, Servers: append([]string(nil), item.Servers...), MonitorURL: item.MonitorURL, Auth: string(item.Auth), Username: item.Username, TLS: item.TLS, TLSServer: item.TLSServer, InsecureTLS: item.InsecureTLS, LastUsedAt: last})
	}
	return result
}
