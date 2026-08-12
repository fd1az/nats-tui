package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	app "github.com/fd1az/nats-tui/internal/app/commandbar"
	"github.com/fd1az/nats-tui/internal/config"
	domain "github.com/fd1az/nats-tui/internal/domain/commandbar"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type CommandManager interface {
	ListCustomCommands(context.Context, config.Settings) ([]app.Entry, error)
	SaveCustomCommand(context.Context, config.Settings, domain.DefinitionInput) error
	DeleteCustomCommand(context.Context, config.Settings, string) error
	ListCommandHistory(context.Context, config.Settings, int) ([]app.HistoryView, error)
	PreviewCustomCommand(context.Context, config.Settings, string, map[string]string) (app.PreviewView, error)
}

func newCommandsCommand(values *viper.Viper, service CommandManager) *cobra.Command {
	var output string
	command := &cobra.Command{Use: "commands", Aliases: []string{"cmd"}, Short: "Manage command-bar automation", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if service == nil {
			return errors.New("command manager is not configured")
		}
		entries, err := service.ListCustomCommands(cmd.Context(), config.Load(values))
		if err != nil {
			return err
		}
		if output == "json" {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(entries)
		}
		if output != "table" {
			return fmt.Errorf("unsupported output %q (use table or json)", output)
		}
		for _, entry := range entries {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", entry.ID, entry.Title, entry.Action, entry.Risk); err != nil {
				return err
			}
		}
		return nil
	}}
	command.Flags().StringVarP(&output, "output", "o", "table", "table or json")
	command.AddCommand(newCommandAdd(values, service), newCommandDelete(values, service), newCommandPreview(values, service), newCommandHistory(values, service))
	return command
}
func newCommandAdd(values *viper.Viper, service CommandManager) *cobra.Command {
	var title, description, group, action, risk string
	var parameters, required, arguments, equivalent []string
	command := &cobra.Command{Use: "add ID", Short: "Save a typed custom command", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return errors.New("command manager is not configured")
		}
		requiredSet := map[string]bool{}
		for _, name := range required {
			requiredSet[name] = true
		}
		params := make([]domain.ParameterInput, 0, len(parameters))
		declared := make(map[string]bool, len(parameters))
		for _, raw := range parameters {
			name, defaultValue, _ := strings.Cut(raw, "=")
			declared[name] = true
			params = append(params, domain.ParameterInput{Name: name, Default: defaultValue, Required: requiredSet[name]})
		}
		for name := range requiredSet {
			if !declared[name] {
				return fmt.Errorf("required parameter %q was not declared with --parameter", name)
			}
		}
		mapped, err := parseAssignments(arguments)
		if err != nil {
			return fmt.Errorf("arguments: %w", err)
		}
		return service.SaveCustomCommand(cmd.Context(), config.Load(values), domain.DefinitionInput{ID: args[0], Title: title, Description: description, Group: group, Action: action, Risk: domain.Risk(risk), Parameters: params, Arguments: mapped, Equivalent: equivalent, Custom: true})
	}}
	flags := command.Flags()
	flags.StringVar(&title, "title", "", "display title")
	flags.StringVar(&description, "description", "", "short operator help")
	flags.StringVar(&group, "group", "Custom", "result group")
	flags.StringVar(&action, "action", "", "typed action such as navigate.messages")
	flags.StringVar(&risk, "risk", string(domain.RiskRead), "read, mutation, or destructive")
	flags.StringSliceVar(&parameters, "parameter", nil, "parameter name or name=default (repeatable)")
	flags.StringSliceVar(&required, "required", nil, "required parameter name (repeatable)")
	flags.StringSliceVar(&arguments, "argument", nil, "action argument key=template (repeatable)")
	flags.StringSliceVar(&equivalent, "equivalent-part", nil, "one literal nats CLI argv part (repeatable)")
	_ = command.MarkFlagRequired("title")
	_ = command.MarkFlagRequired("action")
	return command
}
func newCommandDelete(values *viper.Viper, service CommandManager) *cobra.Command {
	var yes bool
	command := &cobra.Command{Use: "delete ID", Short: "Delete a custom command", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return errors.New("deleting a custom command requires --yes")
		}
		if service == nil {
			return errors.New("command manager is not configured")
		}
		return service.DeleteCustomCommand(cmd.Context(), config.Load(values), args[0])
	}}
	command.Flags().BoolVar(&yes, "yes", false, "confirm deletion")
	return command
}
func newCommandPreview(values *viper.Viper, service CommandManager) *cobra.Command {
	var raw []string
	var output string
	command := &cobra.Command{Use: "preview ID", Short: "Resolve parameters without executing", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return errors.New("command manager is not configured")
		}
		mapped, err := parseAssignments(raw)
		if err != nil {
			return err
		}
		preview, err := service.PreviewCustomCommand(cmd.Context(), config.Load(values), args[0], mapped)
		if err != nil {
			return err
		}
		if output == "json" {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(preview)
		}
		if output != "text" {
			return fmt.Errorf("unsupported output %q (use text or json)", output)
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\nAction: %s\nRisk: %s\nEquivalent: %s\n", preview.Title, preview.Action, preview.Risk, preview.Command)
		return err
	}}
	command.Flags().StringSliceVar(&raw, "value", nil, "parameter value key=value (repeatable)")
	command.Flags().StringVarP(&output, "output", "o", "text", "text or json")
	return command
}
func newCommandHistory(values *viper.Viper, service CommandManager) *cobra.Command {
	var limit int
	var jsonOutput bool
	command := &cobra.Command{Use: "history", Short: "Show persistent command history", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if service == nil {
			return errors.New("command manager is not configured")
		}
		history, err := service.ListCommandHistory(cmd.Context(), config.Load(values), limit)
		if err != nil {
			return err
		}
		if jsonOutput {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(history)
		}
		for _, entry := range history {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\n", entry.StartedAt.Format(time.RFC3339), entry.Status, entry.Duration, entry.CommandID, entry.Title); err != nil {
				return err
			}
		}
		return nil
	}}
	command.Flags().IntVarP(&limit, "limit", "n", 50, "maximum entries")
	command.Flags().BoolVar(&jsonOutput, "json", false, "print JSON")
	return command
}
func parseAssignments(values []string) (map[string]string, error) {
	result := make(map[string]string, len(values))
	for _, value := range values {
		name, item, ok := strings.Cut(value, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("expected key=value, got %q", value)
		}
		result[name] = item
	}
	return result, nil
}
