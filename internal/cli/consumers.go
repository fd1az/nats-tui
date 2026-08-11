package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/clcsolutions/nats-tui/internal/app/consumers"
	"github.com/clcsolutions/nats-tui/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type consumerFlags struct {
	description, deliver, deliverGroup, deliverPolicy, startTime string
	ackPolicy, replayPolicy, sample                              string
	filters, backoff, metadata                                   []string
	startSequence, rateLimit                                     uint64
	ackWait, maxRequestExpires, inactiveThreshold, heartbeat     time.Duration
	maxDeliver, maxAckPending, maxWaiting, maxRequestBatch       int
	maxRequestBytes, replicas                                    int
	durable, headersOnly, memoryStorage, flowControl             bool
}

func newConsumerCommand(values *viper.Viper, service ConsumerManager) *cobra.Command {
	command := &cobra.Command{Use: "consumer", Aliases: []string{"consumers"}, Short: "Inspect and manage JetStream consumers", Args: cobra.NoArgs}
	command.AddCommand(newConsumerListCommand(values, service), newConsumerCreateCommand(values, service), newConsumerEditCommand(values, service), newConsumerDeleteCommand(values, service))
	return command
}

func newConsumerListCommand(values *viper.Viper, service ConsumerManager) *cobra.Command {
	var output string
	command := &cobra.Command{Use: "list", Short: "List consumers across all streams", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if service == nil {
			return errors.New("consumer manager is not configured")
		}
		items, err := service.ListConsumerCatalog(cmd.Context(), config.Load(values))
		if err != nil {
			return err
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].Stream < items[j].Stream || items[i].Stream == items[j].Stream && items[i].Name < items[j].Name
		})
		switch output {
		case "json":
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(items)
		case "table":
			for _, item := range items {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%d\t%s\n", item.Stream, item.Name, item.Status, item.Lag(), consumerMode(item)); err != nil {
					return err
				}
			}
			return nil
		default:
			return fmt.Errorf("unsupported output %q (use table or json)", output)
		}
	}}
	command.Flags().StringVarP(&output, "output", "o", "table", "table or json")
	return command
}

func consumerMode(item consumers.Summary) string {
	mode := "pull/ephemeral"
	if item.Push {
		mode = "push/ephemeral"
	}
	if item.Durable {
		mode = strings.Replace(mode, "ephemeral", "durable", 1)
	}
	return mode
}

func newConsumerCreateCommand(values *viper.Viper, service ConsumerManager) *cobra.Command {
	flags := defaultConsumerFlags()
	command := &cobra.Command{Use: "create STREAM NAME", Short: "Create a fully configured consumer", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return errors.New("consumer manager is not configured")
		}
		definition, err := flags.definition(args[0], args[1])
		if err != nil {
			return err
		}
		if err := service.OperateConsumerConfig(cmd.Context(), config.Load(values), consumers.ConfigCommand{Action: consumers.ConfigCreate, Definition: definition}); err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "created consumer %s on stream %s\n", args[1], args[0])
		return err
	}}
	bindConsumerFlags(command.Flags(), &flags)
	return command
}

func newConsumerEditCommand(values *viper.Viper, service ConsumerManager) *cobra.Command {
	flags := defaultConsumerFlags()
	command := &cobra.Command{Use: "edit STREAM NAME", Short: "Edit consumer fields while preserving omitted configuration", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return errors.New("consumer manager is not configured")
		}
		items, err := service.ListConsumerCatalog(cmd.Context(), config.Load(values))
		if err != nil {
			return err
		}
		var definition consumers.Definition
		found := false
		for _, item := range items {
			if item.Stream == args[0] && item.Name == args[1] {
				definition, found = item.Definition(), true
				break
			}
		}
		if !found {
			return fmt.Errorf("consumer %q on stream %q not found", args[1], args[0])
		}
		if err := flags.applyChanged(cmd.Flags(), &definition); err != nil {
			return err
		}
		if err := definition.Validate(); err != nil {
			return err
		}
		if err := service.OperateConsumerConfig(cmd.Context(), config.Load(values), consumers.ConfigCommand{Action: consumers.ConfigUpdate, Definition: definition}); err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "updated consumer %s on stream %s\n", args[1], args[0])
		return err
	}}
	bindConsumerFlags(command.Flags(), &flags)
	return command
}

func newConsumerDeleteCommand(values *viper.Viper, service ConsumerManager) *cobra.Command {
	var yes bool
	command := &cobra.Command{Use: "delete STREAM NAME", Aliases: []string{"rm"}, Short: "Delete a consumer and its delivery state", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return errors.New("deleting a consumer requires --yes")
		}
		if service == nil {
			return errors.New("consumer manager is not configured")
		}
		ref, err := consumers.NewRef(args[0], args[1])
		if err != nil {
			return err
		}
		if err := service.OperateConsumer(cmd.Context(), config.Load(values), consumers.Command{Consumer: ref, Action: consumers.ActionDelete}); err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "deleted consumer %s from stream %s\n", args[1], args[0])
		return err
	}}
	command.Flags().BoolVar(&yes, "yes", false, "confirm deletion")
	return command
}

func defaultConsumerFlags() consumerFlags {
	return consumerFlags{durable: true, deliverPolicy: string(consumers.DeliverAll), ackPolicy: string(consumers.AckExplicit), replayPolicy: string(consumers.ReplayInstant), maxDeliver: -1, maxAckPending: 1000, maxWaiting: 512}
}

func bindConsumerFlags(flags *pflag.FlagSet, values *consumerFlags) {
	flags.StringVar(&values.description, "description", "", "consumer description")
	flags.BoolVar(&values.durable, "durable", values.durable, "persist the consumer independently of client interest")
	flags.StringVar(&values.deliver, "deliver", "", "push delivery subject; omit for pull")
	flags.StringVar(&values.deliverGroup, "deliver-group", "", "push queue group")
	flags.StringVar(&values.deliverPolicy, "deliver-policy", values.deliverPolicy, "all, last, new, by-start-sequence, by-start-time, or last-per-subject")
	flags.Uint64Var(&values.startSequence, "start-sequence", 0, "first stream sequence for by-start-sequence")
	flags.StringVar(&values.startTime, "start-time", "", "RFC3339 timestamp for by-start-time")
	flags.StringVar(&values.ackPolicy, "ack-policy", values.ackPolicy, "none, all, or explicit")
	flags.DurationVar(&values.ackWait, "ack-wait", 0, "acknowledgement timeout")
	flags.IntVar(&values.maxDeliver, "max-deliver", values.maxDeliver, "maximum delivery attempts; -1 is unlimited")
	flags.StringSliceVar(&values.backoff, "backoff", nil, "redelivery intervals, for example 1s,5s,30s")
	flags.StringSliceVar(&values.filters, "filter", nil, "filter subject; repeat or comma-separate for multiple")
	flags.StringVar(&values.replayPolicy, "replay-policy", values.replayPolicy, "instant or original")
	flags.Uint64Var(&values.rateLimit, "rate-limit", 0, "maximum push delivery rate in bits per second")
	flags.StringVar(&values.sample, "sample", "", "ack sample frequency from 0 to 100 percent")
	flags.IntVar(&values.maxAckPending, "max-ack-pending", values.maxAckPending, "maximum outstanding acknowledgements; -1 is unlimited")
	flags.IntVar(&values.maxWaiting, "max-waiting", values.maxWaiting, "maximum waiting pull requests")
	flags.IntVar(&values.maxRequestBatch, "max-request-batch", 0, "maximum pull batch size")
	flags.DurationVar(&values.maxRequestExpires, "max-request-expires", 0, "maximum pull request expiration")
	flags.IntVar(&values.maxRequestBytes, "max-request-bytes", 0, "maximum bytes per pull request")
	flags.DurationVar(&values.inactiveThreshold, "inactive-threshold", 0, "delete after this inactive duration")
	flags.IntVar(&values.replicas, "replicas", 0, "consumer state replicas; 0 inherits the stream")
	flags.BoolVar(&values.memoryStorage, "memory-storage", false, "keep consumer state in memory")
	flags.BoolVar(&values.headersOnly, "headers-only", false, "deliver headers without payload")
	flags.BoolVar(&values.flowControl, "flow-control", false, "enable push flow control")
	flags.DurationVar(&values.heartbeat, "heartbeat", 0, "push idle heartbeat interval")
	flags.StringSliceVar(&values.metadata, "metadata", nil, "metadata key=value; repeat for multiple")
}

func (f consumerFlags) definition(stream, name string) (consumers.Definition, error) {
	startTime, err := parseConsumerStartTime(f.startTime)
	if err != nil {
		return consumers.Definition{}, err
	}
	backoff, err := parseDurations(f.backoff)
	if err != nil {
		return consumers.Definition{}, err
	}
	metadata, err := parsePairs(f.metadata)
	if err != nil {
		return consumers.Definition{}, fmt.Errorf("metadata: %w", err)
	}
	definition := consumers.Definition{
		Stream: stream, Name: name, Description: f.description, Durable: f.durable, DeliverSubject: f.deliver, DeliverGroup: f.deliverGroup,
		DeliverPolicy: consumers.DeliverPolicy(f.deliverPolicy), StartSequence: f.startSequence, StartTime: startTime,
		AckPolicy: consumers.AckPolicy(f.ackPolicy), AckWait: f.ackWait, MaxDeliver: f.maxDeliver, Backoff: backoff,
		FilterSubjects: cleanStrings(f.filters), ReplayPolicy: consumers.ReplayPolicy(f.replayPolicy), RateLimit: f.rateLimit, SampleFrequency: f.sample,
		MaxAckPending: f.maxAckPending, MaxWaiting: f.maxWaiting, MaxRequestBatch: f.maxRequestBatch, MaxRequestExpires: f.maxRequestExpires,
		MaxRequestMaxBytes: f.maxRequestBytes, InactiveThreshold: f.inactiveThreshold, Replicas: f.replicas, MemoryStorage: f.memoryStorage,
		HeadersOnly: f.headersOnly, FlowControl: f.flowControl, IdleHeartbeat: f.heartbeat, Metadata: metadata,
	}
	return definition, definition.Validate()
}

func (f consumerFlags) applyChanged(flags *pflag.FlagSet, definition *consumers.Definition) error {
	var err error
	flags.Visit(func(flag *pflag.Flag) {
		switch flag.Name {
		case "description":
			definition.Description = f.description
		case "durable":
			definition.Durable = f.durable
		case "deliver":
			definition.DeliverSubject = f.deliver
		case "deliver-group":
			definition.DeliverGroup = f.deliverGroup
		case "deliver-policy":
			definition.DeliverPolicy = consumers.DeliverPolicy(f.deliverPolicy)
		case "start-sequence":
			definition.StartSequence = f.startSequence
		case "ack-policy":
			definition.AckPolicy = consumers.AckPolicy(f.ackPolicy)
		case "ack-wait":
			definition.AckWait = f.ackWait
		case "max-deliver":
			definition.MaxDeliver = f.maxDeliver
		case "filter":
			definition.FilterSubjects = cleanStrings(f.filters)
		case "replay-policy":
			definition.ReplayPolicy = consumers.ReplayPolicy(f.replayPolicy)
		case "rate-limit":
			definition.RateLimit = f.rateLimit
		case "sample":
			definition.SampleFrequency = f.sample
		case "max-ack-pending":
			definition.MaxAckPending = f.maxAckPending
		case "max-waiting":
			definition.MaxWaiting = f.maxWaiting
		case "max-request-batch":
			definition.MaxRequestBatch = f.maxRequestBatch
		case "max-request-expires":
			definition.MaxRequestExpires = f.maxRequestExpires
		case "max-request-bytes":
			definition.MaxRequestMaxBytes = f.maxRequestBytes
		case "inactive-threshold":
			definition.InactiveThreshold = f.inactiveThreshold
		case "replicas":
			definition.Replicas = f.replicas
		case "memory-storage":
			definition.MemoryStorage = f.memoryStorage
		case "headers-only":
			definition.HeadersOnly = f.headersOnly
		case "flow-control":
			definition.FlowControl = f.flowControl
		case "heartbeat":
			definition.IdleHeartbeat = f.heartbeat
		}
	})
	if flags.Changed("start-time") {
		definition.StartTime, err = parseConsumerStartTime(f.startTime)
		if err != nil {
			return err
		}
	}
	if flags.Changed("backoff") {
		definition.Backoff, err = parseDurations(f.backoff)
		if err != nil {
			return err
		}
	}
	if flags.Changed("metadata") {
		definition.Metadata, err = parsePairs(f.metadata)
		if err != nil {
			return fmt.Errorf("metadata: %w", err)
		}
	}
	return nil
}

func parseConsumerStartTime(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("start time must be RFC3339: %w", err)
	}
	return &parsed, nil
}

func parseDurations(values []string) ([]time.Duration, error) {
	result := make([]time.Duration, 0, len(values))
	for _, raw := range values {
		for _, value := range strings.Split(raw, ",") {
			if strings.TrimSpace(value) == "" {
				continue
			}
			parsed, err := time.ParseDuration(strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("invalid duration %q: %w", value, err)
			}
			result = append(result, parsed)
		}
	}
	return result, nil
}

func cleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, raw := range values {
		for _, value := range strings.Split(raw, ",") {
			if value = strings.TrimSpace(value); value != "" {
				result = append(result, value)
			}
		}
	}
	return result
}

func parsePairs(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make(map[string]string, len(values))
	for _, value := range values {
		key, item, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("expected key=value, got %q", value)
		}
		result[strings.TrimSpace(key)] = item
	}
	return result, nil
}
