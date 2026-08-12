package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	app "github.com/fd1az/nats-tui/internal/app/alerts"
	"github.com/fd1az/nats-tui/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type AlertManager interface {
	ListAlerts(context.Context, config.Settings, app.Query) ([]app.Record, error)
	WatchAlerts(context.Context, config.Settings) (<-chan app.Record, <-chan error, error)
	WatchAlertsWithNotifications(context.Context, config.Settings, string, []string) (<-chan app.Record, <-chan error, error)
	AcknowledgeAlert(context.Context, config.Settings, string) error
	AcknowledgeAllAlerts(context.Context, config.Settings) (int, error)
	ScanAlerts(context.Context, config.Settings) ([]app.IngestResult, error)
	ListAlertRules(context.Context, config.Settings) ([]app.RuleView, error)
	CreateAlertRule(context.Context, config.Settings, app.CreateRule) error
	SetAlertRuleEnabled(context.Context, config.Settings, string, bool) error
	DeleteAlertRule(context.Context, config.Settings, string) error
}

func newAlertsCommand(values *viper.Viper, service AlertManager) *cobra.Command {
	command := &cobra.Command{Use: "alerts", Short: "Inspect advisories and local health alerts", Args: cobra.NoArgs}
	command.AddCommand(newAlertListCommand(values, service), newAlertWatchCommand(values, service), newAlertAckCommand(values, service), newAlertScanCommand(values, service), newAlertRulesCommand(values, service))
	return command
}

func requireAlerts(service AlertManager) error {
	if service == nil {
		return errors.New("alert service is not configured")
	}
	return nil
}

func newAlertListCommand(values *viper.Viper, service AlertManager) *cobra.Command {
	var query app.Query
	var output string
	command := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List the persistent alert inbox", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if err := requireAlerts(service); err != nil {
			return err
		}
		records, err := service.ListAlerts(cmd.Context(), config.Load(values), query)
		if err != nil {
			return err
		}
		return writeAlerts(cmd.OutOrStdout(), records, output)
	}}
	flags := command.Flags()
	flags.StringVarP(&query.Search, "search", "s", "", "search title, resource, subject, or payload")
	flags.StringVar(&query.Severity, "severity", "", "filter by info, warning, or critical")
	flags.StringVar(&query.Category, "category", "", "filter by audit, stream, consumer, server, connection, or jetstream")
	flags.BoolVar(&query.Unacknowledged, "unacked", false, "show only unacknowledged alerts")
	flags.IntVarP(&query.Limit, "limit", "n", 100, "maximum alerts to print (0 means all)")
	flags.StringVarP(&output, "output", "o", "table", "output format: table, json, or ndjson")
	return command
}

func newAlertWatchCommand(values *viper.Viper, service AlertManager) *cobra.Command {
	var output string
	var webhook, program string
	var arguments []string
	command := &cobra.Command{Use: "watch", Short: "Watch JetStream advisories as they arrive", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if err := requireAlerts(service); err != nil {
			return err
		}
		var records <-chan app.Record
		var failures <-chan error
		var err error
		if webhook != "" || program != "" {
			command := []string(nil)
			if program != "" {
				command = append([]string{program}, arguments...)
			}
			records, failures, err = service.WatchAlertsWithNotifications(cmd.Context(), config.Load(values), webhook, command)
		} else {
			records, failures, err = service.WatchAlerts(cmd.Context(), config.Load(values))
		}
		if err != nil {
			return err
		}
		for records != nil || failures != nil {
			select {
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			case record, ok := <-records:
				if !ok {
					records = nil
					continue
				}
				if err := writeAlerts(cmd.OutOrStdout(), []app.Record{record}, output); err != nil {
					return err
				}
			case failure, ok := <-failures:
				if !ok {
					failures = nil
					continue
				}
				if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", failure); err != nil {
					return err
				}
			}
		}
		return nil
	}}
	command.Flags().StringVarP(&output, "output", "o", "ndjson", "output format: table, json, or ndjson")
	command.Flags().StringVar(&webhook, "webhook", "", "POST each stored alert as JSON to this URL")
	command.Flags().StringVar(&program, "notify-program", "", "run this executable for each stored alert (JSON is sent on stdin)")
	command.Flags().StringSliceVar(&arguments, "notify-arg", nil, "argument passed directly to --notify-program (repeatable)")
	return command
}

func newAlertAckCommand(values *viper.Viper, service AlertManager) *cobra.Command {
	var all bool
	command := &cobra.Command{Use: "ack [ALERT_ID]", Short: "Acknowledge one alert or the complete inbox", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAlerts(service); err != nil {
			return err
		}
		if all {
			if len(args) != 0 {
				return errors.New("ALERT_ID and --all are mutually exclusive")
			}
			count, err := service.AcknowledgeAllAlerts(cmd.Context(), config.Load(values))
			if err == nil {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Acknowledged %d alerts.\n", count)
			}
			return err
		}
		if len(args) != 1 {
			return errors.New("provide ALERT_ID or --all")
		}
		return service.AcknowledgeAlert(cmd.Context(), config.Load(values), args[0])
	}}
	command.Flags().BoolVar(&all, "all", false, "acknowledge every alert in the inbox")
	return command
}

func newAlertScanCommand(values *viper.Viper, service AlertManager) *cobra.Command {
	return &cobra.Command{Use: "scan", Short: "Evaluate server and connection health rules now", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if err := requireAlerts(service); err != nil {
			return err
		}
		results, err := service.ScanAlerts(cmd.Context(), config.Load(values))
		if err != nil {
			return err
		}
		stored := 0
		for _, result := range results {
			if result.Stored {
				stored++
			}
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Health scan complete: %d new alerts.\n", stored)
		return err
	}}
}

func newAlertRulesCommand(values *viper.Viper, service AlertManager) *cobra.Command {
	command := &cobra.Command{Use: "rules", Short: "Manage local severity and cooldown rules", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if err := requireAlerts(service); err != nil {
			return err
		}
		rules, err := service.ListAlertRules(cmd.Context(), config.Load(values))
		if err != nil {
			return err
		}
		for _, rule := range rules {
			matcher := rule.Pattern
			if matcher == "" {
				matcher = fmt.Sprintf("%s >= %g", rule.Metric, rule.Threshold)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\t%v\n", rule.ID, rule.Name, matcher, strings.ToUpper(rule.Severity), rule.Cooldown, rule.Enabled)
			if err != nil {
				return err
			}
		}
		return nil
	}}
	command.AddCommand(newAlertRuleAddCommand(values, service))
	for _, enabled := range []bool{true, false} {
		value := enabled
		verb := "disable"
		if value {
			verb = "enable"
		}
		command.AddCommand(&cobra.Command{Use: verb + " RULE_ID", Short: strings.ToUpper(verb[:1]) + verb[1:] + " an alert rule", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireAlerts(service); err != nil {
				return err
			}
			return service.SetAlertRuleEnabled(cmd.Context(), config.Load(values), args[0], value)
		}})
	}
	var yes bool
	deleteCommand := &cobra.Command{Use: "delete RULE_ID", Short: "Delete a local alert rule", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return errors.New("deleting an alert rule requires --yes")
		}
		if err := requireAlerts(service); err != nil {
			return err
		}
		return service.DeleteAlertRule(cmd.Context(), config.Load(values), args[0])
	}}
	deleteCommand.Flags().BoolVar(&yes, "yes", false, "confirm rule deletion")
	command.AddCommand(deleteCommand)
	return command
}

func newAlertRuleAddCommand(values *viper.Viper, service AlertManager) *cobra.Command {
	var pattern, metric, severity string
	var threshold float64
	var cooldown time.Duration
	command := &cobra.Command{Use: "add NAME", Short: "Create an event matcher or health threshold", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAlerts(service); err != nil {
			return err
		}
		if (pattern == "") == (metric == "") {
			return errors.New("set exactly one of --match or --metric")
		}
		return service.CreateAlertRule(cmd.Context(), config.Load(values), app.CreateRule{Name: args[0], Pattern: pattern, Metric: metric, Threshold: threshold, Severity: severity, Cooldown: cooldown, Enabled: true})
	}}
	flags := command.Flags()
	flags.StringVar(&pattern, "match", "", "NATS event subject pattern (supports * and terminal >)")
	flags.StringVar(&metric, "metric", "", "health metric: slow-consumers, pending-bytes, server-cpu, server-memory, or link-down")
	flags.Float64Var(&threshold, "threshold", 1, "health threshold that triggers the rule")
	flags.StringVar(&severity, "severity", "warning", "resulting severity: info, warning, or critical")
	flags.DurationVar(&cooldown, "cooldown", time.Minute, "minimum interval between matching alerts")
	return command
}

func writeAlerts(output io.Writer, records []app.Record, format string) error {
	switch strings.ToLower(format) {
	case "json":
		return json.NewEncoder(output).Encode(records)
	case "ndjson":
		encoder := json.NewEncoder(output)
		for _, record := range records {
			if err := encoder.Encode(record); err != nil {
				return err
			}
		}
		return nil
	case "table", "":
		for _, record := range records {
			state := "NEW"
			if record.Acknowledged {
				state = "ACK"
			}
			resource := strings.Trim(strings.Join([]string{record.ResourcePrimary, record.ResourceSecondary}, "/"), "/")
			if resource == "" {
				resource = "-"
			}
			if _, err := fmt.Fprintf(output, "%s\t%s\t%s\t%s\t%s\t%s\n", record.OccurredAt.Format(time.RFC3339), strings.ToUpper(record.Severity), state, record.Category, resource, record.Title); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported alert output %q (use table, json, or ndjson)", format)
	}
}
