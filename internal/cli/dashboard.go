package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/clcsolutions/nats-tui/internal/app/dashboard"
	"github.com/clcsolutions/nats-tui/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type DashboardManager interface {
	ListDashboardPanels(context.Context, config.Settings) ([]dashboard.Panel, error)
	SaveDashboardPanel(context.Context, config.Settings, string, string, dashboard.Metric) (dashboard.Panel, error)
	DeleteDashboardPanel(context.Context, config.Settings, string) error
}

func newDashboardCommand(values *viper.Viper, service DashboardManager) *cobra.Command {
	command := &cobra.Command{Use: "dashboard", Short: "Configure the real-time dashboard", Args: cobra.NoArgs}
	command.AddCommand(newDashboardPanelCommand(values, service), &cobra.Command{Use: "metrics", Short: "List metrics available to custom panels", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		for _, metric := range dashboard.Metrics() {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), metric); err != nil {
				return err
			}
		}
		return nil
	}})
	return command
}
func newDashboardPanelCommand(values *viper.Viper, service DashboardManager) *cobra.Command {
	var output string
	command := &cobra.Command{Use: "panel", Aliases: []string{"panels"}, Short: "List profile-scoped custom panels", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if service == nil {
			return errors.New("dashboard manager is not configured")
		}
		panels, err := service.ListDashboardPanels(cmd.Context(), config.Load(values))
		if err != nil {
			return err
		}
		if output == "json" {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(panels)
		}
		if output != "table" {
			return fmt.Errorf("unsupported output %q (use table or json)", output)
		}
		for _, panel := range panels {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", panel.ID, panel.Title, panel.Metric); err != nil {
				return err
			}
		}
		return nil
	}}
	command.Flags().StringVarP(&output, "output", "o", "table", "table or json")
	var title string
	add := &cobra.Command{Use: "add ID METRIC", Short: "Add or replace a custom dashboard panel", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return errors.New("dashboard manager is not configured")
		}
		panel, err := service.SaveDashboardPanel(cmd.Context(), config.Load(values), args[0], title, dashboard.Metric(strings.ToLower(args[1])))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), panel.ID)
		return err
	}}
	add.Flags().StringVar(&title, "title", "", "panel display title")
	var yes bool
	remove := &cobra.Command{Use: "delete ID", Short: "Delete a custom dashboard panel", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return errors.New("deleting a dashboard panel requires --yes")
		}
		if service == nil {
			return errors.New("dashboard manager is not configured")
		}
		return service.DeleteDashboardPanel(cmd.Context(), config.Load(values), args[0])
	}}
	remove.Flags().BoolVar(&yes, "yes", false, "confirm panel deletion")
	command.AddCommand(add, remove)
	return command
}
