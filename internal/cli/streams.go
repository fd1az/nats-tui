package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/clcsolutions/nats-tui/internal/app/streams"
	"github.com/clcsolutions/nats-tui/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type streamFlags struct {
	description, storage, retention, discard                         string
	subjects, sources, sourceConfigs, placementTags, metadata        []string
	maxMessages, maxBytes, maxPerSubject                             int64
	maxMessageSize                                                   int32
	maxConsumers, replicas                                           int
	maxAge, duplicateWindow                                          time.Duration
	compression, allowDirect, mirrorDirect, allowRollup              bool
	denyDelete, denyPurge, sealed, noAck, allowMessageTTL            bool
	discardNewPerSubject, republishHeadersOnly                       bool
	placementCluster, transformSource, transformDestination          string
	republishSource, republishDestination, mirror                    string
	mirrorFilter, mirrorStartTime, mirrorExternalAPI, mirrorDelivery string
	mirrorTransformSource, mirrorTransformDestination                string
	mirrorStartSequence                                              uint64
}

func newStreamCommand(values *viper.Viper, service StreamManager) *cobra.Command {
	command := &cobra.Command{Use: "stream", Aliases: []string{"streams"}, Short: "Inspect and manage JetStream streams", Args: cobra.NoArgs}
	command.AddCommand(newStreamListCommand(values, service), newStreamCreateCommand(values, service), newStreamEditCommand(values, service), newStreamDeleteCommand(values, service), newStreamPurgeCommand(values, service), newStreamDeleteMessageCommand(values, service), newStreamExportCommand(values, service))
	return command
}

func newStreamListCommand(values *viper.Viper, service StreamManager) *cobra.Command {
	var output string
	command := &cobra.Command{Use: "list", Short: "List streams", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if service == nil {
			return errors.New("stream manager is not configured")
		}
		items, err := service.ListStreamCatalog(cmd.Context(), config.Load(values))
		if err != nil {
			return err
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
		if output == "json" {
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(items)
		}
		if output != "table" {
			return fmt.Errorf("unsupported output %q (use table or json)", output)
		}
		for _, item := range items {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%d\t%d\t%d\t%s\n", item.Name, item.Messages, item.Bytes, item.Replicas, streamTopology(item)); err != nil {
				return err
			}
		}
		return nil
	}}
	command.Flags().StringVarP(&output, "output", "o", "table", "table or json")
	return command
}

func newStreamCreateCommand(values *viper.Viper, service StreamManager) *cobra.Command {
	flags := defaultStreamFlags()
	command := &cobra.Command{Use: "create NAME", Short: "Create a stream with advanced configuration", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return errors.New("stream manager is not configured")
		}
		definition, err := flags.definition(args[0])
		if err != nil {
			return err
		}
		if err := service.OperateStreamConfig(cmd.Context(), config.Load(values), streams.Command{Action: streams.ActionCreate, Name: args[0], Definition: definition}); err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "created stream %s\n", args[0])
		return err
	}}
	bindStreamFlags(command.Flags(), &flags)
	return command
}

func newStreamEditCommand(values *viper.Viper, service StreamManager) *cobra.Command {
	flags := defaultStreamFlags()
	var yes bool
	command := &cobra.Command{Use: "edit NAME", Short: "Edit stream fields while preserving omitted configuration", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return errors.New("stream manager is not configured")
		}
		items, err := service.ListStreamCatalog(cmd.Context(), config.Load(values))
		if err != nil {
			return err
		}
		var definition streams.Definition
		found := false
		for _, item := range items {
			if item.Name == args[0] {
				definition, found = item.Definition(), true
				break
			}
		}
		if !found {
			return fmt.Errorf("stream %q not found", args[0])
		}
		if err := flags.applyChanged(cmd.Flags(), &definition); err != nil {
			return err
		}
		if found && itemsDefinitionSealed(items, args[0]) && cmd.Flags().Changed("sealed") && !definition.Sealed {
			return errors.New("sealed streams cannot be unsealed")
		}
		if definition.Sealed && cmd.Flags().Changed("sealed") && !yes {
			return errors.New("sealing is irreversible; pass --yes to confirm")
		}
		if err := definition.Validate(); err != nil {
			return err
		}
		if err := service.OperateStreamConfig(cmd.Context(), config.Load(values), streams.Command{Action: streams.ActionUpdate, Name: args[0], Definition: definition}); err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "updated stream %s\n", args[0])
		return err
	}}
	bindStreamFlags(command.Flags(), &flags)
	command.Flags().BoolVar(&yes, "yes", false, "confirm irreversible sealing")
	return command
}

func itemsDefinitionSealed(items []streams.Summary, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return item.Sealed
		}
	}
	return false
}

func newStreamDeleteCommand(values *viper.Viper, service StreamManager) *cobra.Command {
	var yes bool
	command := &cobra.Command{Use: "delete NAME", Short: "Delete a stream and all stored messages", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return errors.New("deleting a stream requires --yes")
		}
		if service == nil {
			return errors.New("stream manager is not configured")
		}
		if err := service.OperateStreamConfig(cmd.Context(), config.Load(values), streams.Command{Action: streams.ActionDelete, Name: args[0]}); err != nil {
			return err
		}
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "deleted stream %s\n", args[0])
		return err
	}}
	command.Flags().BoolVar(&yes, "yes", false, "confirm permanent deletion")
	return command
}

func newStreamPurgeCommand(values *viper.Viper, service StreamManager) *cobra.Command {
	var subject string
	var sequence, keep uint64
	var yes bool
	command := &cobra.Command{Use: "purge NAME", Short: "Purge all or selected stored messages", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return errors.New("purging messages requires --yes")
		}
		if service == nil {
			return errors.New("stream manager is not configured")
		}
		input := streams.PurgeCommand{Stream: args[0], Subject: subject, Sequence: sequence, Keep: keep}
		if err := input.Validate(); err != nil {
			return err
		}
		if err := service.PurgeStreamMessages(cmd.Context(), config.Load(values), input); err != nil {
			return err
		}
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "purged stream %s\n", args[0])
		return err
	}}
	command.Flags().StringVar(&subject, "subject", "", "limit purge to a subject filter")
	command.Flags().Uint64Var(&sequence, "sequence", 0, "remove messages below this stream sequence")
	command.Flags().Uint64Var(&keep, "keep", 0, "keep the newest N matching messages")
	command.Flags().BoolVar(&yes, "yes", false, "confirm permanent message removal")
	command.MarkFlagsMutuallyExclusive("sequence", "keep")
	return command
}

func newStreamDeleteMessageCommand(values *viper.Viper, service StreamManager) *cobra.Command {
	var secure, yes bool
	command := &cobra.Command{Use: "delete-message NAME SEQUENCE", Short: "Delete one stored message", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return errors.New("deleting a stored message requires --yes")
		}
		if service == nil {
			return errors.New("stream manager is not configured")
		}
		sequence, err := strconv.ParseUint(args[1], 10, 64)
		if err != nil || sequence == 0 {
			return errors.New("sequence must be a positive integer")
		}
		input := streams.MessageDeleteCommand{Stream: args[0], Sequence: sequence, Secure: secure}
		if err := service.DeleteStoredStreamMessage(cmd.Context(), config.Load(values), input); err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "deleted message %d from stream %s\n", sequence, args[0])
		return err
	}}
	command.Flags().BoolVar(&secure, "secure", false, "overwrite message data before deletion")
	command.Flags().BoolVar(&yes, "yes", false, "confirm permanent deletion")
	return command
}

func newStreamExportCommand(values *viper.Viper, service StreamManager) *cobra.Command {
	return &cobra.Command{Use: "export NAME", Short: "Export stream configuration as JSON", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return errors.New("stream manager is not configured")
		}
		items, err := service.ListStreamCatalog(cmd.Context(), config.Load(values))
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.Name == args[0] {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(item.Definition())
			}
		}
		return fmt.Errorf("stream %q not found", args[0])
	}}
}

func defaultStreamFlags() streamFlags {
	return streamFlags{storage: "file", retention: "limits", discard: "old", maxMessages: -1, maxBytes: -1, maxPerSubject: -1, maxMessageSize: -1, maxConsumers: -1, replicas: 1}
}

func bindStreamFlags(flags *pflag.FlagSet, value *streamFlags) {
	flags.StringVar(&value.description, "description", "", "human-readable description")
	flags.StringSliceVar(&value.subjects, "subject", nil, "captured subject; repeat or comma-separate")
	flags.StringVar(&value.storage, "storage", "file", "file or memory")
	flags.StringVar(&value.retention, "retention", "limits", "limits, interest, or workqueue")
	flags.Int64Var(&value.maxMessages, "max-messages", -1, "maximum stored messages (-1 unlimited)")
	flags.Int64Var(&value.maxBytes, "max-bytes", -1, "maximum stored bytes (-1 unlimited)")
	flags.Int64Var(&value.maxPerSubject, "max-messages-per-subject", -1, "per-subject message limit (-1 unlimited)")
	flags.Int32Var(&value.maxMessageSize, "max-message-size", -1, "maximum message bytes (-1 unlimited)")
	flags.IntVar(&value.maxConsumers, "max-consumers", -1, "maximum consumers (-1 unlimited)")
	flags.DurationVar(&value.maxAge, "max-age", 0, "maximum message age, for example 24h")
	flags.DurationVar(&value.duplicateWindow, "duplicate-window", 0, "duplicate detection window")
	flags.IntVar(&value.replicas, "replicas", 1, "replicas from 1 to 5")
	flags.StringVar(&value.discard, "discard", "old", "old or new")
	flags.BoolVar(&value.discardNewPerSubject, "discard-new-per-subject", false, "apply discard-new independently per subject")
	flags.BoolVar(&value.compression, "compression", false, "enable S2 file compression")
	flags.BoolVar(&value.allowDirect, "allow-direct", false, "enable direct message access")
	flags.BoolVar(&value.mirrorDirect, "mirror-direct", false, "enable direct access through mirrors")
	flags.BoolVar(&value.allowRollup, "allow-rollup", false, "allow Nats-Rollup headers")
	flags.BoolVar(&value.denyDelete, "deny-delete", false, "deny individual message deletion")
	flags.BoolVar(&value.denyPurge, "deny-purge", false, "deny stream purge")
	flags.BoolVar(&value.sealed, "sealed", false, "irreversibly seal the stream")
	flags.BoolVar(&value.noAck, "no-ack", false, "disable publish acknowledgements")
	flags.BoolVar(&value.allowMessageTTL, "allow-message-ttl", false, "allow header-driven per-message TTL")
	flags.StringVar(&value.placementCluster, "placement-cluster", "", "target cluster")
	flags.StringSliceVar(&value.placementTags, "placement-tag", nil, "placement tag; repeat as needed")
	flags.StringSliceVar(&value.metadata, "metadata", nil, "metadata key=value; repeat as needed")
	flags.StringVar(&value.transformSource, "transform-source", "", "subject transform source")
	flags.StringVar(&value.transformDestination, "transform-destination", "", "subject transform destination")
	flags.StringVar(&value.republishSource, "republish-source", "", "republish source filter")
	flags.StringVar(&value.republishDestination, "republish-destination", "", "republish destination")
	flags.BoolVar(&value.republishHeadersOnly, "republish-headers-only", false, "republish headers without payload")
	flags.StringVar(&value.mirror, "mirror", "", "mirror another stream")
	flags.StringSliceVar(&value.sources, "source", nil, "source stream; repeat as needed")
	flags.StringSliceVar(&value.sourceConfigs, "source-config", nil, "source JSON with name, filters, transforms, start position, and external prefixes")
	flags.Uint64Var(&value.mirrorStartSequence, "mirror-start-sequence", 0, "mirror starting sequence")
	flags.StringVar(&value.mirrorStartTime, "mirror-start-time", "", "mirror start time in RFC3339")
	flags.StringVar(&value.mirrorFilter, "mirror-filter", "", "mirror subject filter")
	flags.StringVar(&value.mirrorExternalAPI, "mirror-external-api", "", "external mirror API prefix")
	flags.StringVar(&value.mirrorDelivery, "mirror-delivery-prefix", "", "external mirror delivery prefix")
	flags.StringVar(&value.mirrorTransformSource, "mirror-transform-source", "", "mirror subject transform source")
	flags.StringVar(&value.mirrorTransformDestination, "mirror-transform-destination", "", "mirror subject transform destination")
}

func (f streamFlags) definition(name string) (streams.Definition, error) {
	definition := streams.Definition{Name: name}
	if err := f.applyAll(&definition); err != nil {
		return streams.Definition{}, err
	}
	if err := definition.Validate(); err != nil {
		return streams.Definition{}, err
	}
	return definition, nil
}

func (f streamFlags) applyAll(d *streams.Definition) error {
	d.Description, d.Subjects = f.description, append([]string(nil), f.subjects...)
	storage, retention, discard, err := parseStreamEnums(f.storage, f.retention, f.discard)
	if err != nil {
		return err
	}
	d.Storage, d.Retention, d.Discard = storage, retention, discard
	d.MaxMessages, d.MaxBytes, d.MaxMessagesPerSubject, d.MaxMessageSize, d.MaxConsumers = f.maxMessages, f.maxBytes, f.maxPerSubject, f.maxMessageSize, f.maxConsumers
	d.MaxAge, d.DuplicateWindow, d.Replicas = f.maxAge, f.duplicateWindow, f.replicas
	d.DiscardNewPerSubject, d.Compression, d.AllowDirect, d.MirrorDirect, d.AllowRollup = f.discardNewPerSubject, f.compression, f.allowDirect, f.mirrorDirect, f.allowRollup
	d.DenyDelete, d.DenyPurge, d.Sealed, d.NoAck, d.AllowMessageTTL = f.denyDelete, f.denyPurge, f.sealed, f.noAck, f.allowMessageTTL
	d.PlacementCluster, d.PlacementTags = f.placementCluster, append([]string(nil), f.placementTags...)
	metadata, err := parseAssignments(f.metadata)
	if err != nil {
		return err
	}
	d.Metadata = metadata
	d.SubjectTransform = optionalTransform(f.transformSource, f.transformDestination)
	if f.republishSource != "" || f.republishDestination != "" {
		d.Republish = &streams.Republish{Source: f.republishSource, Destination: f.republishDestination, HeadersOnly: f.republishHeadersOnly}
	}
	mirror, err := f.mirrorSource()
	if err != nil {
		return err
	}
	d.Mirror = mirror
	for _, source := range f.sources {
		d.Sources = append(d.Sources, streams.Source{Name: strings.TrimSpace(source)})
	}
	for _, raw := range f.sourceConfigs {
		source, err := parseStreamSourceJSON(raw)
		if err != nil {
			return err
		}
		d.Sources = append(d.Sources, source)
	}
	return nil
}

func (f streamFlags) applyChanged(flags *pflag.FlagSet, d *streams.Definition) error {
	all := *d
	if err := f.applyAll(&all); err != nil {
		return err
	}
	set := func(name string, apply func()) {
		if flags.Changed(name) {
			apply()
		}
	}
	set("description", func() { d.Description = all.Description })
	set("subject", func() { d.Subjects = all.Subjects })
	set("storage", func() { d.Storage = all.Storage })
	set("retention", func() { d.Retention = all.Retention })
	set("discard", func() { d.Discard = all.Discard })
	set("max-messages", func() { d.MaxMessages = all.MaxMessages })
	set("max-bytes", func() { d.MaxBytes = all.MaxBytes })
	set("max-messages-per-subject", func() { d.MaxMessagesPerSubject = all.MaxMessagesPerSubject })
	set("max-message-size", func() { d.MaxMessageSize = all.MaxMessageSize })
	set("max-consumers", func() { d.MaxConsumers = all.MaxConsumers })
	set("max-age", func() { d.MaxAge = all.MaxAge })
	set("duplicate-window", func() { d.DuplicateWindow = all.DuplicateWindow })
	set("replicas", func() { d.Replicas = all.Replicas })
	set("discard-new-per-subject", func() { d.DiscardNewPerSubject = all.DiscardNewPerSubject })
	set("compression", func() { d.Compression = all.Compression })
	set("allow-direct", func() { d.AllowDirect = all.AllowDirect })
	set("mirror-direct", func() { d.MirrorDirect = all.MirrorDirect })
	set("allow-rollup", func() { d.AllowRollup = all.AllowRollup })
	set("deny-delete", func() { d.DenyDelete = all.DenyDelete })
	set("deny-purge", func() { d.DenyPurge = all.DenyPurge })
	set("sealed", func() { d.Sealed = all.Sealed })
	set("no-ack", func() { d.NoAck = all.NoAck })
	set("allow-message-ttl", func() { d.AllowMessageTTL = all.AllowMessageTTL })
	set("placement-cluster", func() { d.PlacementCluster = all.PlacementCluster })
	set("placement-tag", func() { d.PlacementTags = all.PlacementTags })
	set("metadata", func() { d.Metadata = all.Metadata })
	if flags.Changed("transform-source") || flags.Changed("transform-destination") {
		transform := &streams.SubjectTransform{}
		if d.SubjectTransform != nil {
			*transform = *d.SubjectTransform
		}
		if flags.Changed("transform-source") {
			transform.Source = f.transformSource
		}
		if flags.Changed("transform-destination") {
			transform.Destination = f.transformDestination
		}
		if transform.Source == "" && transform.Destination == "" {
			d.SubjectTransform = nil
		} else {
			d.SubjectTransform = transform
		}
	}
	if flags.Changed("republish-source") || flags.Changed("republish-destination") || flags.Changed("republish-headers-only") {
		republish := &streams.Republish{}
		if d.Republish != nil {
			*republish = *d.Republish
		}
		if flags.Changed("republish-source") {
			republish.Source = f.republishSource
		}
		if flags.Changed("republish-destination") {
			republish.Destination = f.republishDestination
		}
		if flags.Changed("republish-headers-only") {
			republish.HeadersOnly = f.republishHeadersOnly
		}
		if republish.Source == "" && republish.Destination == "" {
			d.Republish = nil
		} else {
			d.Republish = republish
		}
	}
	if flags.Changed("mirror") || flags.Changed("mirror-start-sequence") || flags.Changed("mirror-start-time") || flags.Changed("mirror-filter") || flags.Changed("mirror-external-api") || flags.Changed("mirror-delivery-prefix") || flags.Changed("mirror-transform-source") || flags.Changed("mirror-transform-destination") {
		mirror := &streams.Source{}
		if d.Mirror != nil {
			*mirror = *d.Mirror
			mirror.SubjectTransforms = append([]streams.SubjectTransform(nil), d.Mirror.SubjectTransforms...)
		}
		if flags.Changed("mirror") {
			mirror.Name = strings.TrimSpace(f.mirror)
		}
		if flags.Changed("mirror-start-sequence") {
			mirror.StartSequence, mirror.StartTime = f.mirrorStartSequence, nil
		}
		if flags.Changed("mirror-start-time") {
			if f.mirrorStartTime == "" {
				mirror.StartTime = nil
			} else {
				started, err := time.Parse(time.RFC3339, f.mirrorStartTime)
				if err != nil {
					return fmt.Errorf("mirror start time must be RFC3339: %w", err)
				}
				mirror.StartTime = &started
			}
			mirror.StartSequence = 0
		}
		if flags.Changed("mirror-filter") {
			mirror.FilterSubject = f.mirrorFilter
		}
		if flags.Changed("mirror-external-api") || flags.Changed("mirror-delivery-prefix") {
			if mirror.External == nil {
				mirror.External = &streams.External{}
			}
			if flags.Changed("mirror-external-api") {
				mirror.External.APIPrefix = f.mirrorExternalAPI
			}
			if flags.Changed("mirror-delivery-prefix") {
				mirror.External.DeliverPrefix = f.mirrorDelivery
			}
		}
		if flags.Changed("mirror-transform-source") || flags.Changed("mirror-transform-destination") {
			transform := streams.SubjectTransform{}
			if len(mirror.SubjectTransforms) > 0 {
				transform = mirror.SubjectTransforms[0]
			}
			if flags.Changed("mirror-transform-source") {
				transform.Source = f.mirrorTransformSource
			}
			if flags.Changed("mirror-transform-destination") {
				transform.Destination = f.mirrorTransformDestination
			}
			mirror.SubjectTransforms = []streams.SubjectTransform{transform}
		}
		if mirror.Name == "" {
			d.Mirror = nil
		} else {
			d.Mirror = mirror
		}
	}
	if flags.Changed("source") || flags.Changed("source-config") {
		d.Sources = all.Sources
	}
	return nil
}

func parseStreamEnums(storageValue, retentionValue, discardValue string) (streams.Storage, streams.Retention, streams.Discard, error) {
	storage := streams.Storage(strings.ToLower(storageValue))
	if storage != streams.StorageFile && storage != streams.StorageMemory {
		return "", "", "", fmt.Errorf("unsupported storage %q", storageValue)
	}
	retention := streams.Retention(strings.ToLower(retentionValue))
	if retention != streams.RetentionLimits && retention != streams.RetentionInterest && retention != streams.RetentionWorkQueue {
		return "", "", "", fmt.Errorf("unsupported retention %q", retentionValue)
	}
	discard := streams.Discard(strings.ToLower(discardValue))
	if discard != streams.DiscardOld && discard != streams.DiscardNew {
		return "", "", "", fmt.Errorf("unsupported discard policy %q", discardValue)
	}
	return storage, retention, discard, nil
}

func (f streamFlags) mirrorSource() (*streams.Source, error) {
	if strings.TrimSpace(f.mirror) == "" {
		return nil, nil
	}
	result := &streams.Source{Name: f.mirror, StartSequence: f.mirrorStartSequence, FilterSubject: f.mirrorFilter}
	if f.mirrorStartTime != "" {
		started, err := time.Parse(time.RFC3339, f.mirrorStartTime)
		if err != nil {
			return nil, fmt.Errorf("mirror start time must be RFC3339: %w", err)
		}
		result.StartTime = &started
	}
	if f.mirrorExternalAPI != "" || f.mirrorDelivery != "" {
		result.External = &streams.External{APIPrefix: f.mirrorExternalAPI, DeliverPrefix: f.mirrorDelivery}
	}
	if f.mirrorTransformSource != "" || f.mirrorTransformDestination != "" {
		result.SubjectTransforms = []streams.SubjectTransform{{Source: f.mirrorTransformSource, Destination: f.mirrorTransformDestination}}
	}
	return result, nil
}

type streamSourceJSON struct {
	Name          string `json:"name"`
	StartSequence uint64 `json:"start_sequence,omitempty"`
	StartTime     string `json:"start_time,omitempty"`
	FilterSubject string `json:"filter_subject,omitempty"`
	ExternalAPI   string `json:"external_api,omitempty"`
	DeliverPrefix string `json:"deliver_prefix,omitempty"`
	Transforms    []struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	} `json:"transforms,omitempty"`
}

func parseStreamSourceJSON(raw string) (streams.Source, error) {
	var input streamSourceJSON
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return streams.Source{}, fmt.Errorf("decode source config: %w", err)
	}
	result := streams.Source{Name: input.Name, StartSequence: input.StartSequence, FilterSubject: input.FilterSubject}
	if input.StartTime != "" {
		started, err := time.Parse(time.RFC3339, input.StartTime)
		if err != nil {
			return streams.Source{}, fmt.Errorf("source %q start time must be RFC3339: %w", input.Name, err)
		}
		result.StartTime = &started
	}
	if input.ExternalAPI != "" || input.DeliverPrefix != "" {
		result.External = &streams.External{APIPrefix: input.ExternalAPI, DeliverPrefix: input.DeliverPrefix}
	}
	for _, transform := range input.Transforms {
		result.SubjectTransforms = append(result.SubjectTransforms, streams.SubjectTransform{Source: transform.Source, Destination: transform.Destination})
	}
	if err := result.Validate(); err != nil {
		return streams.Source{}, err
	}
	return result, nil
}

func optionalTransform(source, destination string) *streams.SubjectTransform {
	if source == "" && destination == "" {
		return nil
	}
	return &streams.SubjectTransform{Source: source, Destination: destination}
}

func streamTopology(item streams.Summary) string {
	if item.Mirror != nil {
		return "mirror:" + item.Mirror.Name
	}
	if len(item.Sources) > 0 {
		names := make([]string, 0, len(item.Sources))
		for _, source := range item.Sources {
			names = append(names, source.Name)
		}
		return "sources:" + strings.Join(names, ",")
	}
	return "standalone"
}
