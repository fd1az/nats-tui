package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	app "github.com/clcsolutions/nats-tui/internal/app/connections"
	"github.com/spf13/cobra"
)

func newProfileCommand(service ProfileManager) *cobra.Command {
	command := &cobra.Command{Use: "profile", Short: "Manage reusable NATS connection profiles", Args: cobra.NoArgs}
	command.AddCommand(newProfileSaveCommand(service), newProfileListCommand(service), newProfileDeleteCommand(service), newProfileTestCommand(service), newProfileExportCommand(service))
	return command
}

func newProfileSaveCommand(service ProfileManager) *cobra.Command {
	var servers []string
	var monitorURL, token, credentials, username, password string
	command := &cobra.Command{Use: "save NAME", Short: "Create or replace a connection profile", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return errors.New("profile manager is unavailable")
		}
		if err := service.SaveConnectionProfile(cmd.Context(), app.ProfileInput{Name: args[0], Servers: servers, MonitorURL: monitorURL, Token: token, Credentials: credentials, Username: username, Password: password}); err != nil {
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
	_ = command.MarkFlagRequired("server-url")
	command.MarkFlagsMutuallyExclusive("auth-token", "credentials-file", "auth-user")
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
		return encoder.Encode(items)
	}}
}

func writeProfiles(cmd *cobra.Command, items []app.ProfileSummary, output string) error {
	switch strings.ToLower(output) {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(items)
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
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", item.Name, strings.Join(item.Servers, ","), item.Auth, last); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported output %q", output)
	}
}
